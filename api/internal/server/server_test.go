package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Exonical/stig-manager-react/api/internal/config"
	"github.com/Exonical/stig-manager-react/api/internal/server"
)

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	cfg := &config.Config{
		HTTPAddr:       ":0",
		AllowedOrigins: []string{"*"},
	}
	srv := server.New(server.Options{
		Version:   "1.2.3",
		Commit:    "abc123",
		BuildDate: "2026-05-19",
		Config:    cfg,
	})
	return srv.Router()
}

func TestHealth(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)

	newTestServer(t).ServeHTTP(rec, req)

	if got := rec.Result().StatusCode; got != http.StatusOK {
		t.Fatalf("status: got %d want %d", got, http.StatusOK)
	}
}

func TestAppInfo(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/op/appinfo", nil)

	newTestServer(t).ServeHTTP(rec, req)

	if got := rec.Result().StatusCode; got != http.StatusOK {
		t.Fatalf("status: got %d want %d (body=%s)", got, http.StatusOK, rec.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["version"] != "1.2.3" {
		t.Fatalf("version: got %v want 1.2.3", body["version"])
	}
}

func TestConfiguration(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/op/configuration", nil)

	newTestServer(t).ServeHTTP(rec, req)

	if got := rec.Result().StatusCode; got != http.StatusOK {
		t.Fatalf("status: got %d want %d (body=%s)", got, http.StatusOK, rec.Body.String())
	}
}

// TestUnimplementedReturns501 confirms that every operation we have not yet
// overridden returns a 501 Not Implemented from api.Unimplemented (the
// generator-supplied default) rather than a 404 or a panic.
//
// /collections has only optional query parameters, so it bypasses the
// generated 400 validation layer and exercises Unimplemented directly.
func TestUnimplementedReturns501(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/collections", nil)

	newTestServer(t).ServeHTTP(rec, req)

	if got := rec.Result().StatusCode; got != http.StatusNotImplemented {
		t.Fatalf("status: got %d want %d (body=%s)", got, http.StatusNotImplemented, rec.Body.String())
	}
}
