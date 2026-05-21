//go:build integration

package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Exonical/stig-manager-react/api/internal/auth"
)

// TestAuditMiddlewareRecordsMutations boots the full server stack
// against a real Postgres, fires a POST that the audit middleware
// should capture, then reads /api/op/audit-log back to confirm the
// row is present with the right shape.
func TestAuditMiddlewareRecordsMutations(t *testing.T) {
	pool := newIntegrationPool(t)
	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(t.Context(), auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	handler := newTestServer(t, withAuth(prov), withPool(pool))

	// Drive a mutating call through the stack. The Collections
	// handler refuses anonymous writes with 403, but the audit
	// middleware records the attempt regardless of outcome — which
	// is exactly what we want forensically.
	body := []byte(`{"name":"audit-it","metadata":{"password":"hunter2"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/collections",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// The audit row is written from a goroutine; poll the read
	// endpoint until it shows up.
	tok := fx.token(t, "stig-manager:op:read")
	deadline := time.Now().Add(5 * time.Second)
	var rows []map[string]any
	for time.Now().Before(deadline) {
		read := httptest.NewRecorder()
		readReq := httptest.NewRequest(http.MethodGet,
			"/api/op/audit-log?path=collections&limit=10", nil)
		readReq.Header.Set("Authorization", "Bearer "+tok)
		handler.ServeHTTP(read, readReq)
		if read.Code != http.StatusOK {
			t.Fatalf("audit-log: status %d body=%s", read.Code, read.Body.String())
		}
		if err := json.NewDecoder(read.Body).Decode(&rows); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(rows) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(rows) == 0 {
		t.Fatalf("no audit row recorded for POST /api/collections")
	}

	first := rows[0]
	if first["method"] != "POST" {
		t.Errorf("method: got %v want POST", first["method"])
	}
	if first["path"] != "/api/collections" {
		t.Errorf("path: got %v", first["path"])
	}
	if first["status"] == nil {
		t.Errorf("status missing")
	}
	payload, _ := first["payload"].(map[string]any)
	if payload == nil {
		t.Fatalf("payload missing on audited row: %v", first)
	}
	if payload["name"] != "audit-it" {
		t.Errorf("payload.name: got %v", payload["name"])
	}
	meta, _ := payload["metadata"].(map[string]any)
	if meta == nil || meta["password"] != "***" {
		t.Errorf("expected nested password redacted: %v", payload)
	}
}

// TestAuditLogRequiresScope checks the read endpoint enforces the
// stig-manager:op:read scope.
func TestAuditLogRequiresScope(t *testing.T) {
	pool := newIntegrationPool(t)
	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(t.Context(), auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	handler := newTestServer(t, withAuth(prov), withPool(pool))

	// No token at all -> 401.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/op/audit-log", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-token: got %d want 401, body=%s", rec.Code, rec.Body.String())
	}

	// Wrong scope -> 403.
	wrong := fx.token(t, "stig-manager:stig:read")
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/op/audit-log", nil)
	req.Header.Set("Authorization", "Bearer "+wrong)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("wrong-scope: got %d want 403", rec.Code)
	}
}

// TestAuditMethodFilter exercises the ?method= filter end-to-end.
func TestAuditMethodFilter(t *testing.T) {
	pool := newIntegrationPool(t)
	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(t.Context(), auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	handler := newTestServer(t, withAuth(prov), withPool(pool))

	// Fire one POST + one PATCH. Use anonymous calls — auth result
	// doesn't matter, the middleware records the attempt.
	post := httptest.NewRequest(http.MethodPost, "/api/collections",
		bytes.NewReader([]byte(`{"name":"a"}`)))
	post.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(httptest.NewRecorder(), post)

	patch := httptest.NewRequest(http.MethodPatch, "/api/collections/1",
		bytes.NewReader([]byte(`{"name":"b"}`)))
	patch.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(httptest.NewRecorder(), patch)

	tok := fx.token(t, "stig-manager:op:read")
	if !waitForMethod(t, handler, tok, "POST", "PATCH") {
		t.Fatalf("expected both POST and PATCH rows recorded")
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/op/audit-log?method=PATCH&limit=100", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("filter: status %d", rec.Code)
	}
	var got []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("method filter returned nothing")
	}
	for _, row := range got {
		if row["method"] != "PATCH" {
			t.Fatalf("filter leaked %s row: %v", row["method"], row)
		}
	}
}

// waitForMethod polls /api/op/audit-log until rows containing each of
// methods have been recorded, or the timeout expires.
func waitForMethod(t *testing.T, handler http.Handler, tok string, methods ...string) bool {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/op/audit-log?limit=100", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("audit-log: %d", rec.Code)
		}
		var rows []map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&rows); err != nil {
			t.Fatalf("decode: %v", err)
		}
		seen := map[string]bool{}
		for _, r := range rows {
			if m, ok := r["method"].(string); ok {
				seen[strings.ToUpper(m)] = true
			}
		}
		all := true
		for _, m := range methods {
			if !seen[strings.ToUpper(m)] {
				all = false
				break
			}
		}
		if all {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

var _ = context.Background
