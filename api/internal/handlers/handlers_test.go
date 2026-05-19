package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Exonical/stig-manager-react/api/internal/handlers"
)

func TestHealth(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)

	handlers.Health(rec, req)

	if got := rec.Result().StatusCode; got != http.StatusOK {
		t.Fatalf("status: got %d want %d", got, http.StatusOK)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status field: got %q want %q", body["status"], "ok")
	}
}
