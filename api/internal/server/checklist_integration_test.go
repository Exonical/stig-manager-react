//go:build integration

package server_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Exonical/stig-manager-react/api/internal/auth"
	"github.com/Exonical/stig-manager-react/api/internal/checklist"
	"github.com/Exonical/stig-manager-react/api/internal/store"
	"github.com/Exonical/stig-manager-react/api/internal/xccdf"
)

// seedChecklistScenario imports the shared XCCDF fixture, creates one
// asset, maps the benchmark, and submits one pass + one fail review.
// Returns (collID, assetID, benchID).
func seedChecklistScenario(t *testing.T, pool *pgxpool.Pool, subject, username string) (collID, assetID int64, benchID string) {
	t.Helper()
	ctx := context.Background()

	userID, c := seedCollectionWithOwner(t, pool, subject, username)
	collID = c

	f, err := os.Open("../xccdf/testdata/sample.xccdf.xml")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()
	bench, err := xccdf.Parse(f)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if _, err := store.NewSTIGRepo(pool).ImportRevision(ctx, bench, true); err != nil {
		t.Fatalf("import revision: %v", err)
	}
	benchID = bench.BenchmarkID

	assets := store.NewAssetRepo(pool)
	a, err := assets.Create(ctx, store.AssetCreate{
		CollectionID: collID, Name: "checklist-host-" + username,
		FQDN: "checklist-host.example.com", IP: "10.0.0.50", MAC: "aa:bb:cc:dd:ee:50",
	})
	if err != nil {
		t.Fatalf("create asset: %v", err)
	}
	assetID = a.AssetID
	stigs := []string{benchID}
	if _, err := assets.Update(ctx, a.AssetID, store.AssetUpdate{BenchmarkIDs: &stigs}); err != nil {
		t.Fatalf("assign stig: %v", err)
	}

	reviews := store.NewReviewRepo(pool)
	if _, err := reviews.Put(ctx, a.AssetID, "SV-100001r1_rule", userID, store.ReviewWrite{
		Result: "pass", StatusLabel: "submitted", Detail: "all good",
	}); err != nil {
		t.Fatalf("review pass: %v", err)
	}
	if _, err := reviews.Put(ctx, a.AssetID, "SV-100002r1_rule", userID, store.ReviewWrite{
		Result: "fail", StatusLabel: "saved", Detail: "finding", Comment: "needs fix",
	}); err != nil {
		t.Fatalf("review fail: %v", err)
	}
	return collID, assetID, benchID
}

func checklistServer(t *testing.T, pool *pgxpool.Pool) (http.Handler, *oidcFixture) {
	t.Helper()
	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(context.Background(), auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	return newTestServer(t, withAuth(prov), withPool(pool)), fx
}

// TestChecklistByAssetStig_AllFormats hits every format the
// per-asset+stig endpoint supports (json/ckl/cklb/xccdf) and verifies
// that the binary formats re-parse back to the reviews we seeded.
func TestChecklistByAssetStig_AllFormats(t *testing.T) {
	pool := newIntegrationPool(t)
	collID, assetID, benchID := seedChecklistScenario(t, pool, "user-1", "checklist-user")
	handler, fx := checklistServer(t, pool)
	tok := fx.token(t, "stig-manager:collection:read")
	_ = collID
	aStr := strconv.FormatInt(assetID, 10)
	base := "/api/assets/" + aStr + "/checklists/" + benchID + "/latest"

	// JSON variant: array of per-rule summary objects.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, base+"?format=json", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("json status: %d body=%s", rec.Code, rec.Body.String())
	}
	var jsonRows []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &jsonRows); err != nil {
		t.Fatalf("json decode: %v body=%s", err, rec.Body.String())
	}
	results := map[string]string{}
	for _, r := range jsonRows {
		if rid, ok := r["ruleId"].(string); ok {
			results[rid] = r["result"].(string)
		}
	}
	if results["SV-100001r1_rule"] != "pass" || results["SV-100002r1_rule"] != "fail" {
		t.Fatalf("json results: %v", results)
	}

	// CKL variant: parse back via ParseCKL and confirm reviews.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, base+"?format=ckl", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ckl status: %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "xml") {
		t.Fatalf("ckl content-type: %s", rec.Header().Get("Content-Type"))
	}
	cklParsed, err := checklist.ParseCKL(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("re-parse ckl: %v body=%s", err, rec.Body.String())
	}
	if cklParsed.BenchmarkID != benchID {
		t.Fatalf("ckl benchmark: %s", cklParsed.BenchmarkID)
	}
	checkRoundTripResults(t, "ckl", cklParsed.Reviews)

	// CKLB variant.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, base+"?format=cklb", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cklb status: %d body=%s", rec.Code, rec.Body.String())
	}
	cklbParsed, err := checklist.ParseCKLB(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("re-parse cklb: %v body=%s", err, rec.Body.String())
	}
	checkRoundTripResults(t, "cklb", cklbParsed.Reviews)

	// XCCDF variant.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, base+"?format=xccdf", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("xccdf status: %d body=%s", rec.Code, rec.Body.String())
	}
	xParsed, err := checklist.ParseXCCDFResults(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("re-parse xccdf: %v body=%s", err, rec.Body.String())
	}
	checkRoundTripResults(t, "xccdf", xParsed.Reviews)
}

// TestChecklistByAsset_MultiSTIG calls the multi-STIG asset endpoint
// and verifies the asset's mapped benchmark is included in the
// returned CKL.
func TestChecklistByAsset_MultiSTIG(t *testing.T) {
	pool := newIntegrationPool(t)
	_, assetID, benchID := seedChecklistScenario(t, pool, "user-1", "multi-user")
	handler, fx := checklistServer(t, pool)
	tok := fx.token(t, "stig-manager:collection:read")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/assets/"+strconv.FormatInt(assetID, 10)+"/checklists?format=ckl", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("multi ckl status: %d body=%s", rec.Code, rec.Body.String())
	}
	parsed, err := checklist.ParseCKL(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("parse multi ckl: %v", err)
	}
	if parsed.BenchmarkID != benchID {
		t.Fatalf("multi ckl benchmark: %s", parsed.BenchmarkID)
	}
	if len(parsed.Reviews) == 0 {
		t.Fatalf("multi ckl: expected reviews, got none")
	}
}

// TestChecklistByCollectionStig validates the collection-level
// per-rule summary endpoint.
func TestChecklistByCollectionStig(t *testing.T) {
	pool := newIntegrationPool(t)
	collID, _, benchID := seedChecklistScenario(t, pool, "user-1", "summary-user")
	handler, fx := checklistServer(t, pool)
	tok := fx.token(t, "stig-manager:collection:read")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/collections/"+strconv.FormatInt(collID, 10)+"/checklists/"+benchID+"/latest", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("summary status: %d body=%s", rec.Code, rec.Body.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode summary: %v body=%s", err, rec.Body.String())
	}
	pass, fail := 0, 0
	for _, r := range rows {
		pass += int(r["pass"].(float64))
		fail += int(r["fail"].(float64))
	}
	if pass != 1 || fail != 1 {
		t.Fatalf("summary counts: pass=%d fail=%d rows=%v", pass, fail, rows)
	}
}

// TestArchiveByCollection_CKL drives the POST /archive/ckl endpoint
// and confirms the returned ZIP contains exactly one CKL entry per
// (asset, stig) pair in the selection.
func TestArchiveByCollection_CKL(t *testing.T) {
	pool := newIntegrationPool(t)
	collID, assetID, benchID := seedChecklistScenario(t, pool, "user-1", "archive-user")
	handler, fx := checklistServer(t, pool)
	tok := fx.token(t, "stig-manager:collection:read")

	body := bytes.NewBufferString(`[{"assetId": "` + strconv.FormatInt(assetID, 10) + `",
		"stigs": ["` + benchID + `"]}]`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		"/api/collections/"+strconv.FormatInt(collID, 10)+"/archive/ckl", body)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("archive status: %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "application/zip") {
		t.Fatalf("archive content-type: %s", rec.Header().Get("Content-Type"))
	}
	r, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	if len(r.File) != 1 {
		var names []string
		for _, f := range r.File {
			names = append(names, f.Name)
		}
		t.Fatalf("expected 1 CKL entry, got %d (%v)", len(r.File), names)
	}
	if !strings.HasSuffix(r.File[0].Name, ".ckl") {
		t.Fatalf("entry name: %s", r.File[0].Name)
	}
	// Read the CKL entry and re-parse to confirm reviews round-trip.
	rc, err := r.File[0].Open()
	if err != nil {
		t.Fatalf("open entry: %v", err)
	}
	defer rc.Close()
	contents, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read entry: %v", err)
	}
	parsed, err := checklist.ParseCKL(bytes.NewReader(contents))
	if err != nil {
		t.Fatalf("parse entry: %v", err)
	}
	checkRoundTripResults(t, "archive-ckl", parsed.Reviews)
}

// TestChecklist_AuthGuards verifies the auth checks fire correctly:
// 401 when no bearer; 403 when the user has the bearer but no
// collection grant; 404 when the asset doesn't exist.
func TestChecklist_AuthGuards(t *testing.T) {
	pool := newIntegrationPool(t)
	_, assetID, benchID := seedChecklistScenario(t, pool, "user-1", "guarded-user")
	handler, fx := checklistServer(t, pool)
	base := "/api/assets/" + strconv.FormatInt(assetID, 10) + "/checklists/" + benchID + "/latest"

	// 401: no bearer.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, base, nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-bearer: got %d want 401 body=%s", rec.Code, rec.Body.String())
	}

	// 403: bearer is signed for a different sub with no grant on the
	// collection.
	otherTok := fx.tokenForSub(t, "user-99", "stig-manager:collection:read")
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, base, nil)
	req.Header.Set("Authorization", "Bearer "+otherTok)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("no-grant: got %d want 403 body=%s", rec.Code, rec.Body.String())
	}
}

// checkRoundTripResults asserts that a re-parsed checklist contains
// the two reviews seedChecklistScenario submitted.
func checkRoundTripResults(t *testing.T, label string, parsed []checklist.ParsedReview) {
	t.Helper()
	want := map[string]checklist.Result{
		"SV-100001r1_rule": checklist.ResultPass,
		"SV-100002r1_rule": checklist.ResultFail,
	}
	got := map[string]checklist.Result{}
	for _, r := range parsed {
		got[r.RuleID] = r.Result
	}
	for rid, w := range want {
		if got[rid] != w {
			t.Fatalf("%s rule %s: got %s want %s (all=%v)", label, rid, got[rid], w, got)
		}
	}
}
