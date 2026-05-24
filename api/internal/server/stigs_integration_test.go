//go:build integration

package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/Exonical/stig-manager-react/api/internal/auth"
)

// readFixture returns the contents of the shared XCCDF parser fixture
// the integration tests use to round-trip an import.
func readFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../xccdf/testdata/sample.xccdf.xml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return data
}

// uploadXccdf constructs a multipart/form-data request body with the
// XCCDF fixture as the importFile part.
func uploadXccdf(t *testing.T, payload []byte) (string, []byte) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("importFile", "TEST_OS_STIG.xml")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := io.Copy(part, bytes.NewReader(payload)); err != nil {
		t.Fatalf("copy: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close mw: %v", err)
	}
	return mw.FormDataContentType(), buf.Bytes()
}

// TestSTIGsHTTPFlow exercises GET/POST /stigs and the rule/cci lookups
// through the HTTP surface against a real Postgres testcontainer.
func TestSTIGsHTTPFlow(t *testing.T) {
	pool := newIntegrationPool(t)

	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(context.Background(), auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	handler := newTestServer(t, withAuth(prov), withPool(pool))

	xccdfBody := readFixture(t)

	// List on an empty library returns 200 + [].
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/stigs", nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:stig:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list empty: got %d (body=%s)", rec.Code, rec.Body.String())
	}
	var empty []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &empty); err != nil {
		t.Fatalf("list empty json: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("list empty: got %d", len(empty))
	}

	// POST /stigs with a :read-only token → 403.
	ct, body := uploadXccdf(t, xccdfBody)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/stigs", bytes.NewReader(body))
	req.Header.Set("Content-Type", ct)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:stig:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("import wrong scope: got %d (body=%s)", rec.Code, rec.Body.String())
	}

	// POST /stigs with stig-manager:stig → 200 + RevisionPost.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/stigs", bytes.NewReader(body))
	req.Header.Set("Content-Type", ct)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:stig"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("import: got %d (body=%s)", rec.Code, rec.Body.String())
	}
	var revResp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &revResp); err != nil {
		t.Fatalf("import json: %v", err)
	}
	if revResp["benchmarkId"] != "TEST_OS_STIG" || revResp["revisionStr"] != "V2R3" {
		t.Fatalf("import response: %+v", revResp)
	}

	// Re-import without clobber → 400 + ErrDuplicateName surface.
	ct2, body2 := uploadXccdf(t, xccdfBody)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/stigs", bytes.NewReader(body2))
	req.Header.Set("Content-Type", ct2)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:stig"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("re-import: got %d (body=%s)", rec.Code, rec.Body.String())
	}

	// Re-import with ?clobber=true → 200.
	ct3, body3 := uploadXccdf(t, xccdfBody)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/stigs?clobber=true", bytes.NewReader(body3))
	req.Header.Set("Content-Type", ct3)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:stig"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("clobber re-import: got %d (body=%s)", rec.Code, rec.Body.String())
	}

	// GET /stigs (populated) returns the imported entry.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/stigs?title=Operating", nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:stig:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list populated: got %d (body=%s)", rec.Code, rec.Body.String())
	}
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("list json: %v", err)
	}
	if len(list) != 1 || list[0]["benchmarkId"] != "TEST_OS_STIG" {
		t.Fatalf("list populated: %+v", list)
	}

	// GET /stigs/{benchmarkId} → 200 with revisionStrs included.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/stigs/TEST_OS_STIG", nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:stig:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get by id: got %d (body=%s)", rec.Code, rec.Body.String())
	}
	var stigBody map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &stigBody); err != nil {
		t.Fatalf("get json: %v", err)
	}
	if stigBody["lastRevisionStr"] != "V2R3" {
		t.Fatalf("get: %+v", stigBody)
	}

	// GET /stigs/{missing} → 404.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/stigs/UNKNOWN", nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:stig:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get missing: got %d", rec.Code)
	}

	// GET /stigs/rules/{ruleId} → 200 + projection.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/stigs/rules/SV-100001r1_rule", nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:stig:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get rule: got %d (body=%s)", rec.Code, rec.Body.String())
	}
	var ruleBody map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &ruleBody); err != nil {
		t.Fatalf("rule json: %v", err)
	}
	if ruleBody["severity"] != "medium" {
		t.Fatalf("rule severity: %v", ruleBody["severity"])
	}

	// GET /stigs/{benchmarkId}/revisions/{revisionStr}/rules → 200 +
	// array of rules. Default (no projection) returns the basics only.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/stigs/TEST_OS_STIG/revisions/V2R3/rules", nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:stig:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list rules: got %d (body=%s)", rec.Code, rec.Body.String())
	}
	var rulesArr []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &rulesArr); err != nil {
		t.Fatalf("rules json: %v", err)
	}
	if len(rulesArr) == 0 {
		t.Fatalf("expected at least one rule, got 0")
	}
	first := rulesArr[0]
	if first["ruleId"] == nil || first["severity"] == nil {
		t.Fatalf("rule row missing ruleId/severity: %+v", first)
	}
	// Default projection must NOT include heavy fields.
	if _, has := first["check"]; has {
		t.Fatalf("default projection unexpectedly includes check: %+v", first)
	}
	if _, has := first["detail"]; has {
		t.Fatalf("default projection unexpectedly includes detail: %+v", first)
	}

	// With ?projection=check&projection=detail&projection=ccis the
	// heavy fields show up.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet,
		"/api/stigs/TEST_OS_STIG/revisions/V2R3/rules?projection=check&projection=detail&projection=ccis",
		nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:stig:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list rules proj: got %d (body=%s)", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &rulesArr); err != nil {
		t.Fatalf("rules proj json: %v", err)
	}
	gotCheck := false
	for _, row := range rulesArr {
		if _, has := row["check"]; has {
			gotCheck = true
			break
		}
	}
	if !gotCheck {
		t.Fatalf("expected at least one rule with check: %+v", rulesArr)
	}

	// "latest" resolves to the most-recent imported revision.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/stigs/TEST_OS_STIG/revisions/latest/rules", nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:stig:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list rules latest: got %d (body=%s)", rec.Code, rec.Body.String())
	}

	// GET /stigs/{bid}/revisions/{rev}/rules/{ruleId} → 200.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet,
		"/api/stigs/TEST_OS_STIG/revisions/V2R3/rules/SV-100001r1_rule?projection=check&projection=fix",
		nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:stig:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get rule by revision: got %d (body=%s)", rec.Code, rec.Body.String())
	}
	var ruleByRevBody map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &ruleByRevBody); err != nil {
		t.Fatalf("rule by revision json: %v", err)
	}
	if ruleByRevBody["ruleId"] != "SV-100001r1_rule" {
		t.Fatalf("rule by revision ruleId: %v", ruleByRevBody["ruleId"])
	}

	// Unknown revision → 404.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/stigs/TEST_OS_STIG/revisions/V99R99/rules", nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:stig:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown revision: got %d", rec.Code)
	}

	// GET /stigs/ccis/{cci} → 200 + cci with stigs[]. The OpenAPI spec
	// accepts six digits with no prefix; the handler normalises.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/stigs/ccis/000366", nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:stig:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get cci: got %d (body=%s)", rec.Code, rec.Body.String())
	}
	var cciBody map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cciBody); err != nil {
		t.Fatalf("cci json: %v", err)
	}
	if cciBody["cci"] != "CCI-000366" {
		t.Fatalf("cci.cci: %v", cciBody["cci"])
	}
	if stigs, _ := cciBody["stigs"].([]any); len(stigs) != 1 {
		t.Fatalf("cci.stigs: %+v", cciBody["stigs"])
	}

	// GET /stigs/ccis/{missing} → 404.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/stigs/ccis/999999", nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:stig:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get cci missing: got %d", rec.Code)
	}

	// POST /stigs with garbage payload → 400.
	rec = httptest.NewRecorder()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("importFile", "junk.xml")
	_, _ = io.Copy(part, strings.NewReader("not really xml"))
	_ = mw.Close()
	req = httptest.NewRequest(http.MethodPost, "/api/stigs", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:stig"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("garbage import: got %d (body=%s)", rec.Code, rec.Body.String())
	}
}
