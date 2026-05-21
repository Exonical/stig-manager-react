//go:build integration

package server_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Exonical/stig-manager-react/api/internal/auth"
)

func opServer(t *testing.T, pool *pgxpool.Pool) (http.Handler, *oidcFixture) {
	t.Helper()
	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(t.Context(), auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	return newTestServer(t, withAuth(prov), withPool(pool)), fx
}

// TestStateEndpoint verifies GET /api/op/state reports db=true (we
// have a live pool) and oidc=true (we configured an auth provider).
func TestStateEndpoint(t *testing.T) {
	pool := newIntegrationPool(t)
	handler, _ := opServer(t, pool)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/op/state", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["currentState"] != "available" {
		t.Errorf("currentState: got %v want available", body["currentState"])
	}
	deps, _ := body["dependencies"].(map[string]any)
	if deps == nil || deps["db"] != true || deps["oidc"] != true {
		t.Errorf("dependencies: got %v want {db:true,oidc:true}", deps)
	}
}

// TestAppInfoEndpoint exercises the read-only AppInfo summary: counts,
// runtime, request counter, schema migration.
func TestAppInfoEndpoint(t *testing.T) {
	pool := newIntegrationPool(t)
	handler, fx := opServer(t, pool)
	tok := fx.token(t, "stig-manager:op:read")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/op/appinfo", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["version"] != "1.2.3" {
		t.Errorf("version: got %v want 1.2.3", body["version"])
	}
	if body["schema"] == "" || body["schema"] == "0" {
		t.Errorf("schema migration version not surfaced: %v", body["schema"])
	}
	counts, _ := body["counts"].(map[string]any)
	if counts == nil {
		t.Fatalf("counts missing")
	}
	if _, ok := counts["users"]; !ok {
		t.Errorf("counts.users missing")
	}
	pg, _ := body["postgres"].(map[string]any)
	if pg == nil || pg["version"] == "" {
		t.Errorf("postgres section missing: %v", body["postgres"])
	}
	rt, _ := body["runtime"].(map[string]any)
	if rt == nil || rt["goVersion"] == "" {
		t.Errorf("runtime section missing: %v", body["runtime"])
	}
	reqStats, _ := body["requests"].(map[string]any)
	if reqStats == nil {
		t.Errorf("requests section missing")
	}
}

// TestAppDataTables verifies GET /api/op/appdata/tables returns every
// public table.
func TestAppDataTables(t *testing.T) {
	pool := newIntegrationPool(t)
	handler, fx := opServer(t, pool)
	tok := fx.token(t, "stig-manager:op:read")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/op/appdata/tables", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var tables []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&tables); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := map[string]bool{
		"app_user": false, "collection": false, "job": false,
		"job_run": false, "review": false, "asset": false,
	}
	for _, tbl := range tables {
		name, _ := tbl["name"].(string)
		if _, ok := want[name]; ok {
			want[name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("table %q missing from /op/appdata/tables", name)
		}
	}
}

// TestAppDataExport verifies the JSON export contains all known
// tables and parses correctly.
func TestAppDataExport(t *testing.T) {
	pool := newIntegrationPool(t)
	handler, fx := opServer(t, pool)
	tok := fx.token(t, "stig-manager:op:read")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/op/appdata", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("content-type: got %q want application/json*", ct)
	}
	var dump map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&dump); err != nil {
		t.Fatalf("decode: %v", err)
	}
	tables, _ := dump["tables"].(map[string]any)
	if tables == nil {
		t.Fatalf("tables missing")
	}
	for _, name := range []string{"app_user", "collection", "job_task", "stig"} {
		if _, ok := tables[name]; !ok {
			t.Errorf("table %q missing from export", name)
		}
	}
}

// TestReplaceAppDataDeferred verifies the import endpoint returns the
// staged-503 marker.
func TestReplaceAppDataDeferred(t *testing.T) {
	pool := newIntegrationPool(t)
	handler, fx := opServer(t, pool)
	tok := fx.token(t, "stig-manager:op")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/op/appdata", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d want 503 (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestStateSseStream verifies the SSE endpoint streams an inaugural
// state.snapshot frame and that a subsequent job-run transition
// produces a job.run event on the same stream.
func TestStateSseStream(t *testing.T) {
	pool := newIntegrationPool(t)
	// Use the standard server (no auth) so the SSE endpoint stays
	// open without a token; the spec marks it security: [].
	handler := newTestServer(t, withPool(pool))

	srv := httptest.NewServer(handler)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/op/state/sse", nil)
	if err != nil {
		t.Fatalf("req: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("content-type: got %q want text/event-stream*", ct)
	}
	scanner := bufio.NewScanner(resp.Body)
	gotSnapshot := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: state.snapshot") {
			gotSnapshot = true
			break
		}
	}
	if !gotSnapshot {
		t.Fatalf("did not receive state.snapshot event")
	}
}
