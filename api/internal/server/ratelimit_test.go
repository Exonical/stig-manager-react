package server_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// TestRateLimitMiddleware hits a public endpoint (the SPA's
// runtime-config script) past the configured burst and asserts the
// middleware switches over to 429s with a Retry-After header. Using
// /js/Env.js keeps the test independent of auth and the DB pool.
func TestRateLimitMiddleware(t *testing.T) {
	handler := newTestServer(t, withRateLimit(0.5, 3))

	// First 3 calls drain the bucket.
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/js/Env.js", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("call %d: expected 200 from initial burst, got %d", i, rec.Code)
		}
	}

	// 4th call from the same IP trips the limiter.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/js/Env.js", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after burst drained, got %d body=%s", rec.Code, rec.Body.String())
	}
	retry := rec.Header().Get("Retry-After")
	if retry == "" {
		t.Fatalf("expected Retry-After on 429")
	}
	if n, err := strconv.Atoi(retry); err != nil || n < 1 {
		t.Fatalf("Retry-After malformed: %q", retry)
	}

	// A different IP gets its own bucket and is not denied.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/js/Env.js", nil)
	req.RemoteAddr = "10.0.0.2:1234"
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected per-IP isolation, got %d on second client", rec.Code)
	}
}

// TestRateLimitSkipsHealth confirms /health stays reachable even
// after the limiter is fully drained.
func TestRateLimitSkipsHealth(t *testing.T) {
	handler := newTestServer(t, withRateLimit(0.1, 1))

	// Drain the bucket against the Env.js endpoint.
	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/js/Env.js", nil)
		req.RemoteAddr = "10.0.0.3:1"
		handler.ServeHTTP(rec, req)
		_ = rec
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.RemoteAddr = "10.0.0.3:1"
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/health should bypass the limiter, got %d", rec.Code)
	}
}
