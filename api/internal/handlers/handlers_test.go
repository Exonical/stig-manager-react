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

func TestAppInfo(t *testing.T) {
	t.Parallel()

	build := handlers.AppInfoBuild{
		Version:   "1.2.3",
		Commit:    "abc123",
		BuildDate: "2026-05-19",
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/op/appinfo", nil)

	handlers.AppInfo(build)(rec, req)

	if got := rec.Result().StatusCode; got != http.StatusOK {
		t.Fatalf("status: got %d want %d", got, http.StatusOK)
	}
	var got handlers.AppInfoBuild
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got != build {
		t.Fatalf("appinfo: got %+v want %+v", got, build)
	}
}

func TestAppDataTables(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/op/appdata/tables", nil)

	handlers.AppDataTables(rec, req)

	if got := rec.Result().StatusCode; got != http.StatusOK {
		t.Fatalf("status: got %d want %d", got, http.StatusOK)
	}
	var body []string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body) != 0 {
		t.Fatalf("tables: got %v want empty", body)
	}
}
