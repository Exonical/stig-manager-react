package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/Exonical/stig-manager-react/api/internal/auth"
)

// TestAllowBurstThenDeny exhausts the bucket, then waits long enough
// for one token to be replenished and confirms the next call passes.
func TestAllowBurstThenDeny(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := New(Config{RequestsPerSecond: 10, Burst: 2, Enabled: true})
	l.now = func() time.Time { return t0 }

	// Two tokens => both pass.
	for i := 0; i < 2; i++ {
		ok, _ := l.Allow("k")
		if !ok {
			t.Fatalf("call %d: expected pass on initial burst", i)
		}
	}
	// Third call drained.
	ok, retry := l.Allow("k")
	if ok {
		t.Fatalf("expected deny after burst exhaustion")
	}
	if retry <= 0 {
		t.Fatalf("expected positive retry hint, got %v", retry)
	}

	// Advance time by 1s; the 10 rps refill returns the bucket.
	l.now = func() time.Time { return t0.Add(time.Second) }
	ok, _ = l.Allow("k")
	if !ok {
		t.Fatalf("expected pass after refill")
	}
}

// TestAllowIndependentKeys confirms the limiter buckets stay
// per-key — exhausting "a" must not affect "b".
func TestAllowIndependentKeys(t *testing.T) {
	t0 := time.Now()
	l := New(Config{RequestsPerSecond: 1, Burst: 1, Enabled: true})
	l.now = func() time.Time { return t0 }

	if ok, _ := l.Allow("a"); !ok {
		t.Fatalf("expected first call on key=a to pass")
	}
	if ok, _ := l.Allow("a"); ok {
		t.Fatalf("expected second call on key=a to deny")
	}
	if ok, _ := l.Allow("b"); !ok {
		t.Fatalf("expected first call on key=b to pass (independent bucket)")
	}
}

// TestSweepDropsStale advances the clock past IdleTTL and confirms
// the previously-seen bucket is reclaimed (a fresh burst is allowed
// again).
func TestSweepDropsStale(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := New(Config{
		RequestsPerSecond: 1, Burst: 1,
		Enabled: true,
		IdleTTL: time.Minute,
	})
	l.now = func() time.Time { return t0 }
	if ok, _ := l.Allow("k"); !ok {
		t.Fatalf("expected initial pass")
	}
	if ok, _ := l.Allow("k"); ok {
		t.Fatalf("expected deny after burst")
	}
	// Bucket should be reaped.
	l.now = func() time.Time { return t0.Add(time.Hour) }
	l.Sweep()
	count := 0
	l.buckets.Range(func(_, _ any) bool {
		count++
		return true
	})
	if count != 0 {
		t.Fatalf("expected sweep to drop stale bucket, %d remain", count)
	}
}

// TestMiddlewareKeysByUser routes two requests with the same subject
// through the middleware and asserts the second is denied — i.e. the
// auth subject is preferred over the IP when keying.
func TestMiddlewareKeysByUser(t *testing.T) {
	l := New(Config{RequestsPerSecond: 1, Burst: 1, Enabled: true})
	calls := 0
	h := l.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	}))

	hit := func(remote, sub string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/whatever", nil)
		req.RemoteAddr = remote + ":1234"
		req = req.WithContext(auth.WithUser(req.Context(), &auth.User{Subject: sub}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	if rec := hit("1.1.1.1", "alice"); rec.Code != http.StatusOK {
		t.Fatalf("first alice: got %d", rec.Code)
	}
	rec := hit("2.2.2.2", "alice")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second alice (different IP, same sub): expected 429 got %d", rec.Code)
	}
	if retry := rec.Header().Get("Retry-After"); retry == "" {
		t.Fatalf("expected Retry-After header on 429")
	} else if n, err := strconv.Atoi(retry); err != nil || n < 1 {
		t.Fatalf("Retry-After malformed: %q", retry)
	}
	if calls != 1 {
		t.Fatalf("expected one downstream call, got %d", calls)
	}
}

// TestMiddlewareSkipPaths confirms the SSE-style skip list bypasses
// the limiter even when the bucket is exhausted.
func TestMiddlewareSkipPaths(t *testing.T) {
	l := New(Config{RequestsPerSecond: 0.1, Burst: 1, Enabled: true})
	calls := 0
	h := l.Middleware("/api/op/state/sse")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
	}))
	// Drain the bucket via a non-skipped path.
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/foo", nil)
		req.RemoteAddr = "9.9.9.9:1"
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
	// SSE path stays accessible.
	req := httptest.NewRequest(http.MethodGet, "/api/op/state/sse", nil)
	req.RemoteAddr = "9.9.9.9:1"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected SSE bypass, got %d", rec.Code)
	}
	if calls < 1 {
		t.Fatalf("expected at least the SSE call to reach the inner handler")
	}
}

// TestMiddlewareDisabled is a sanity check that Enabled=false truly
// bypasses the bucket logic.
func TestMiddlewareDisabled(t *testing.T) {
	l := New(Config{RequestsPerSecond: 0.001, Burst: 1, Enabled: false})
	h := l.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for i := 0; i < 50; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/foo", nil)
		req.RemoteAddr = "8.8.8.8:1"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("call %d: disabled limiter should always pass, got %d", i, rec.Code)
		}
	}
}
