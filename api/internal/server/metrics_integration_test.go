//go:build integration

package server_test

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Exonical/stig-manager-react/api/internal/auth"
	"github.com/Exonical/stig-manager-react/api/internal/store"
	"github.com/Exonical/stig-manager-react/api/internal/xccdf"
)

// seedMetricsScenario imports the shared XCCDF fixture and wires up
// two assets with the benchmark. The first asset gets a "pass" review
// and a "fail" review; the second asset is left untouched so it shows
// up as fully unassessed.
//
//	Asset A (host-a): rule SV-100001r1_rule = pass(submitted), rule SV-100002r1_rule = fail(saved)
//	Asset B (host-b): no reviews
//
// Returns (collectionID, assetA, assetB, benchmarkID, labelID).
func seedMetricsScenario(t *testing.T, pool *pgxpool.Pool, subject, username string) (collID, assetA, assetB int64, benchID, labelID string) {
	t.Helper()
	ctx := context.Background()

	userID, c := seedCollectionWithOwner(t, pool, subject, username)
	collID = c

	f, err := os.Open("../xccdf/testdata/sample.xccdf.xml")
	if err != nil {
		t.Fatalf("open xccdf fixture: %v", err)
	}
	defer f.Close()
	bench, err := xccdf.Parse(f)
	if err != nil {
		t.Fatalf("parse xccdf fixture: %v", err)
	}
	if _, err := store.NewSTIGRepo(pool).ImportRevision(ctx, bench, true); err != nil {
		t.Fatalf("import revision: %v", err)
	}
	benchID = bench.BenchmarkID

	assets := store.NewAssetRepo(pool)
	a1, err := assets.Create(ctx, store.AssetCreate{
		CollectionID: collID, Name: "host-a-" + username, Noncomputing: false,
	})
	if err != nil {
		t.Fatalf("create asset a: %v", err)
	}
	assetA = a1.AssetID
	a2, err := assets.Create(ctx, store.AssetCreate{
		CollectionID: collID, Name: "host-b-" + username, Noncomputing: false,
	})
	if err != nil {
		t.Fatalf("create asset b: %v", err)
	}
	assetB = a2.AssetID
	stigs := []string{benchID}
	if _, err := assets.Update(ctx, a1.AssetID, store.AssetUpdate{BenchmarkIDs: &stigs}); err != nil {
		t.Fatalf("assign stig to asset a: %v", err)
	}
	if _, err := assets.Update(ctx, a2.AssetID, store.AssetUpdate{BenchmarkIDs: &stigs}); err != nil {
		t.Fatalf("assign stig to asset b: %v", err)
	}

	reviews := store.NewReviewRepo(pool)
	if _, err := reviews.Put(ctx, a1.AssetID, "SV-100001r1_rule", userID, store.ReviewWrite{
		Result: "pass", Comment: "ok", Detail: "verified", StatusLabel: "submitted",
	}); err != nil {
		t.Fatalf("review pass: %v", err)
	}
	if _, err := reviews.Put(ctx, a1.AssetID, "SV-100002r1_rule", userID, store.ReviewWrite{
		Result: "fail", Comment: "needs work", Detail: "config drift", StatusLabel: "saved",
	}); err != nil {
		t.Fatalf("review fail: %v", err)
	}

	// One label applied only to asset A so the agg-by-label test has
	// both a labelled and an unlabelled bucket.
	labels := store.NewLabelRepo(pool)
	lbl, err := labels.Create(ctx, store.LabelCreate{
		CollectionID: collID, Name: "ProductionA", Color: "ff0000",
	})
	if err != nil {
		t.Fatalf("create label: %v", err)
	}
	labelID = lbl.LabelID
	if err := labels.SetAssets(ctx, collID, lbl.LabelID, []int64{a1.AssetID}); err != nil {
		t.Fatalf("apply label: %v", err)
	}
	return collID, assetA, assetB, benchID, labelID
}

func metricsServer(t *testing.T, pool *pgxpool.Pool) (handler http.Handler, fx *oidcFixture) {
	t.Helper()
	fx = newOIDCFixture(t)
	prov, err := auth.NewProvider(context.Background(), auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	handler = newTestServer(t, withAuth(prov), withPool(pool))
	return handler, fx
}

func decodeJSONList(t *testing.T, rec *httptest.ResponseRecorder) []map[string]any {
	t.Helper()
	var out []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode list: %v body=%s", err, rec.Body.String())
	}
	return out
}

func decodeJSONObj(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode obj: %v body=%s", err, rec.Body.String())
	}
	return out
}

// TestMetricsSummary_FullSurface drives all five summary endpoints
// end-to-end against the same seeded scenario. We assert the headline
// counts (2 assets × 2 rules = 4 assessments; 2 assessed = 1 pass + 1
// fail; etc.) and the structural shape of each response.
func TestMetricsSummary_FullSurface(t *testing.T) {
	pool := newIntegrationPool(t)
	collID, assetA, _, benchID, labelID := seedMetricsScenario(t, pool, "user-1", "metrics-user")
	handler, fx := metricsServer(t, pool)
	tok := fx.token(t, "stig-manager:collection:read")
	collStr := strconv.FormatInt(collID, 10)

	// ── unagg: one row per (asset, stig) ───────────────────────────
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/collections/"+collStr+"/metrics/summary", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unagg status: %d body=%s", rec.Code, rec.Body.String())
	}
	rows := decodeJSONList(t, rec)
	if len(rows) != 2 {
		t.Fatalf("unagg: expected 2 rows (one per asset), got %d", len(rows))
	}
	// Asset A row should report 2 assessments + 2 assessed.
	var aRow map[string]any
	for _, r := range rows {
		if r["assetId"] == strconv.FormatInt(assetA, 10) {
			aRow = r
		}
	}
	if aRow == nil {
		t.Fatalf("unagg: missing asset A row in %v", rows)
	}
	if aRow["benchmarkId"] != benchID {
		t.Fatalf("unagg row A: benchmarkId got %v want %s", aRow["benchmarkId"], benchID)
	}
	metricsA := aRow["metrics"].(map[string]any)
	if metricsA["assessed"].(float64) != 2 || metricsA["assessments"].(float64) != 2 {
		t.Fatalf("unagg row A metrics: %v", metricsA)
	}
	resultsA := metricsA["results"].(map[string]any)
	if resultsA["pass"].(float64) != 1 || resultsA["fail"].(float64) != 1 {
		t.Fatalf("unagg row A results: %v", resultsA)
	}

	// ── agg by collection: single object ───────────────────────────
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/collections/"+collStr+"/metrics/summary/collection", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("agg collection status: %d body=%s", rec.Code, rec.Body.String())
	}
	obj := decodeJSONObj(t, rec)
	if obj["assets"].(float64) != 2 || obj["stigs"].(float64) != 1 || obj["checklists"].(float64) != 2 {
		t.Fatalf("agg collection counts: %v", obj)
	}
	metricsC := obj["metrics"].(map[string]any)
	// 2 assets × 2 rules = 4 assessments total; 2 reviews exist.
	if metricsC["assessments"].(float64) != 4 || metricsC["assessed"].(float64) != 2 {
		t.Fatalf("agg collection metrics: %v", metricsC)
	}
	// The fixture's two rules have severity medium + high; we failed
	// the high-severity rule, so findings.high == 1 and the others 0.
	findings := metricsC["findings"].(map[string]any)
	if findings["high"].(float64) != 1 || findings["medium"].(float64) != 0 || findings["low"].(float64) != 0 {
		t.Fatalf("agg collection findings: %v", findings)
	}

	// ── agg by asset ───────────────────────────────────────────────
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/collections/"+collStr+"/metrics/summary/asset", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("agg asset status: %d body=%s", rec.Code, rec.Body.String())
	}
	assetRows := decodeJSONList(t, rec)
	if len(assetRows) != 2 {
		t.Fatalf("agg asset: want 2 rows got %d", len(assetRows))
	}
	for _, ar := range assetRows {
		m := ar["metrics"].(map[string]any)
		if m["assessments"].(float64) != 2 {
			t.Fatalf("agg asset row %s: assessments want 2 got %v", ar["assetId"], m["assessments"])
		}
		if ar["assetId"] == strconv.FormatInt(assetA, 10) {
			if m["assessed"].(float64) != 2 {
				t.Fatalf("agg asset row A: assessed want 2 got %v", m["assessed"])
			}
			labels := ar["labels"].([]any)
			if len(labels) != 1 || labels[0].(map[string]any)["labelId"] != labelID {
				t.Fatalf("agg asset row A: labels want [%s] got %v", labelID, labels)
			}
		} else {
			if m["assessed"].(float64) != 0 {
				t.Fatalf("agg asset row B: assessed want 0 got %v", m["assessed"])
			}
		}
	}

	// ── agg by stig ────────────────────────────────────────────────
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/collections/"+collStr+"/metrics/summary/stig", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("agg stig status: %d body=%s", rec.Code, rec.Body.String())
	}
	stigRows := decodeJSONList(t, rec)
	if len(stigRows) != 1 {
		t.Fatalf("agg stig: want 1 row got %d", len(stigRows))
	}
	if stigRows[0]["benchmarkId"] != benchID {
		t.Fatalf("agg stig: benchmarkId %v want %s", stigRows[0]["benchmarkId"], benchID)
	}
	if stigRows[0]["assets"].(float64) != 2 {
		t.Fatalf("agg stig: assets want 2 got %v", stigRows[0]["assets"])
	}

	// ── agg by label (labelled bucket + unlabelled bucket) ─────────
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/collections/"+collStr+"/metrics/summary/label", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("agg label status: %d body=%s", rec.Code, rec.Body.String())
	}
	labelRows := decodeJSONList(t, rec)
	if len(labelRows) != 2 {
		t.Fatalf("agg label: want 2 rows (labelled + unlabelled) got %d (%v)", len(labelRows), labelRows)
	}
	var labelled, unlabelled map[string]any
	for _, lr := range labelRows {
		if lr["labelId"] == nil {
			unlabelled = lr
		} else {
			labelled = lr
		}
	}
	if labelled == nil || unlabelled == nil {
		t.Fatalf("agg label: expected one labelled+one unlabelled got %v", labelRows)
	}
	if labelled["labelId"] != labelID {
		t.Fatalf("agg label: labelled bucket labelId %v want %s", labelled["labelId"], labelID)
	}
	if labelled["assets"].(float64) != 1 || unlabelled["assets"].(float64) != 1 {
		t.Fatalf("agg label assets: labelled=%v unlabelled=%v", labelled["assets"], unlabelled["assets"])
	}
}

// TestMetricsSummary_CSVAndFilters validates the format=csv contract
// and the assetId / labelMatch=null filters.
func TestMetricsSummary_CSVAndFilters(t *testing.T) {
	pool := newIntegrationPool(t)
	collID, assetA, _, benchID, _ := seedMetricsScenario(t, pool, "user-1", "csv-user")
	handler, fx := metricsServer(t, pool)
	tok := fx.token(t, "stig-manager:collection:read")
	collStr := strconv.FormatInt(collID, 10)

	// CSV: agg collection → two rows (header + one data row).
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/collections/"+collStr+"/metrics/summary/collection?format=csv", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("csv collection status: %d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("csv collection content-type: %s", ct)
	}
	csvRows, err := csv.NewReader(strings.NewReader(rec.Body.String())).ReadAll()
	if err != nil {
		t.Fatalf("csv parse: %v", err)
	}
	if len(csvRows) != 2 {
		t.Fatalf("csv collection: want 2 rows (header+data), got %d (%v)", len(csvRows), csvRows)
	}
	if csvRows[0][0] != "collectionId" || csvRows[0][2] != "assets" {
		t.Fatalf("csv collection header: %v", csvRows[0])
	}

	// assetId filter narrows agg-by-asset to one row.
	rec = httptest.NewRecorder()
	url := "/api/collections/" + collStr + "/metrics/summary/asset?assetId=" + strconv.FormatInt(assetA, 10)
	req = httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("filter assetId status: %d body=%s", rec.Code, rec.Body.String())
	}
	rows := decodeJSONList(t, rec)
	if len(rows) != 1 {
		t.Fatalf("filter assetId: want 1 row got %d", len(rows))
	}
	if rows[0]["assetId"] != strconv.FormatInt(assetA, 10) {
		t.Fatalf("filter assetId row: %v", rows[0])
	}

	// labelMatch=null surfaces the unlabelled asset only.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/collections/"+collStr+"/metrics/summary/asset?labelMatch=null", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("labelMatch null status: %d body=%s", rec.Code, rec.Body.String())
	}
	rows = decodeJSONList(t, rec)
	if len(rows) != 1 {
		t.Fatalf("labelMatch null: want 1 row (the unlabelled asset), got %d", len(rows))
	}

	// benchmarkId filter (with a non-matching id) returns 0 rows.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/collections/"+collStr+"/metrics/summary/asset?benchmarkId=NOPE", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("benchmarkId NOPE status: %d body=%s", rec.Code, rec.Body.String())
	}
	rows = decodeJSONList(t, rec)
	if len(rows) != 0 {
		t.Fatalf("benchmarkId NOPE: want 0 rows got %d", len(rows))
	}

	// benchmarkId filter with the real id matches both assets.
	rec = httptest.NewRecorder()
	url = "/api/collections/" + collStr + "/metrics/summary/asset?benchmarkId=" + benchID
	req = httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("benchmarkId real status: %d body=%s", rec.Code, rec.Body.String())
	}
	rows = decodeJSONList(t, rec)
	if len(rows) != 2 {
		t.Fatalf("benchmarkId real: want 2 rows got %d", len(rows))
	}
}

// TestMetricsSummary_AuthChecks verifies missing scope and missing
// grant produce 403 (and missing token produces 401).
func TestMetricsSummary_AuthChecks(t *testing.T) {
	pool := newIntegrationPool(t)
	collID, _, _, _, _ := seedMetricsScenario(t, pool, "user-1", "auth-owner")
	handler, fx := metricsServer(t, pool)
	collStr := strconv.FormatInt(collID, 10)

	// No bearer at all → 401.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/collections/"+collStr+"/metrics/summary/collection", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-token status: %d body=%s", rec.Code, rec.Body.String())
	}

	// Bearer present but wrong scope → 403.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/collections/"+collStr+"/metrics/summary/collection", nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:op:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("wrong-scope status: %d body=%s", rec.Code, rec.Body.String())
	}
}
