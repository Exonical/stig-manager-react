//go:build integration

package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Exonical/stig-manager-react/api/internal/auth"
	"github.com/Exonical/stig-manager-react/api/internal/store"
	"github.com/Exonical/stig-manager-react/api/internal/xccdf"
)

// seedBatchScenario builds a small but realistic universe for the
// batch-review handler:
//
//   - one collection that the JWT subject owns,
//   - two assets in that collection,
//   - the TEST_OS_STIG benchmark imported (two rules:
//     SV-100001r1_rule + SV-100002r1_rule),
//   - both assets configured with the benchmark assigned.
//
// Returns the IDs the test needs to drive the endpoint.
func seedBatchScenario(t *testing.T, pool *pgxpool.Pool, subject, username string) (userID, collID int64, assetA, assetB int64, benchID string, ruleA, ruleB string) {
	t.Helper()
	ctx := context.Background()

	userID, collID = seedCollectionWithOwner(t, pool, subject, username)

	// Import the shared XCCDF fixture so the rule_ids actually exist.
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
		CollectionID: collID, Name: "batch-host-1-" + username, Noncomputing: false,
	})
	if err != nil {
		t.Fatalf("create asset 1: %v", err)
	}
	a2, err := assets.Create(ctx, store.AssetCreate{
		CollectionID: collID, Name: "batch-host-2-" + username, Noncomputing: false,
	})
	if err != nil {
		t.Fatalf("create asset 2: %v", err)
	}
	stigs := []string{benchID}
	if _, err := assets.Update(ctx, a1.AssetID, store.AssetUpdate{BenchmarkIDs: &stigs}); err != nil {
		t.Fatalf("assign stig to asset 1: %v", err)
	}
	if _, err := assets.Update(ctx, a2.AssetID, store.AssetUpdate{BenchmarkIDs: &stigs}); err != nil {
		t.Fatalf("assign stig to asset 2: %v", err)
	}

	return userID, collID, a1.AssetID, a2.AssetID, benchID,
		"SV-100001r1_rule", "SV-100002r1_rule"
}

// TestReviewBatchMerge exercises the happy path: a merge against two
// assets × two rules (i.e. four pairs) where no reviews exist yet.
// The handler should report 4 inserts, 0 updates, 0 failures and the
// rows should be readable afterwards via the per-asset GET endpoint.
func TestReviewBatchMerge(t *testing.T) {
	pool := newIntegrationPool(t)
	_, collID, assetA, assetB, benchID, ruleA, ruleB := seedBatchScenario(t, pool, "user-1", "alice")
	_ = benchID
	_ = ruleA
	_ = ruleB

	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(context.Background(), auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	handler := newTestServer(t, withAuth(prov), withPool(pool))

	body, _ := json.Marshal(map[string]any{
		"action": "merge",
		"assets": map[string]any{"assetIds": []string{
			strconv.FormatInt(assetA, 10), strconv.FormatInt(assetB, 10),
		}},
		"rules": map[string]any{"benchmarkIds": []string{benchID}},
		"source": map[string]any{
			"review": map[string]any{
				"result":  "pass",
				"detail":  "batch detail",
				"comment": "batch comment",
				"status":  "saved",
			},
		},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		"/api/collections/"+strconv.FormatInt(collID, 10)+"/reviews",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("merge: got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["inserted"] != float64(4) {
		t.Fatalf("inserted: want 4 got %v (resp=%v)", resp["inserted"], resp)
	}
	if resp["updated"] != float64(0) {
		t.Fatalf("updated: want 0 got %v", resp["updated"])
	}
	if resp["failedValidation"] != float64(0) {
		t.Fatalf("failedValidation: want 0 got %v", resp["failedValidation"])
	}

	// A second merge with a different result and status should now hit
	// updates instead of inserts.
	body, _ = json.Marshal(map[string]any{
		"action": "merge",
		"assets": map[string]any{"assetIds": []string{strconv.FormatInt(assetA, 10)}},
		"rules":  map[string]any{"ruleIds": []string{ruleA, ruleB}},
		"source": map[string]any{
			"review": map[string]any{
				"result":  "fail",
				"detail":  "now failing",
				"comment": "batch retry",
				"status":  "saved",
			},
		},
	})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost,
		"/api/collections/"+strconv.FormatInt(collID, 10)+"/reviews",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("re-merge: got %d body=%s", rec.Code, rec.Body.String())
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["inserted"] != float64(0) {
		t.Fatalf("re-merge inserted: want 0 got %v", resp["inserted"])
	}
	if resp["updated"] != float64(2) {
		t.Fatalf("re-merge updated: want 2 got %v", resp["updated"])
	}

	// Round-trip: GET /collections/{cid}/reviews/{assetA} should now
	// reflect the failing result.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet,
		"/api/collections/"+strconv.FormatInt(collID, 10)+
			"/reviews/"+strconv.FormatInt(assetA, 10), nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list-after-merge: got %d body=%s", rec.Code, rec.Body.String())
	}
	var list []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 2 {
		t.Fatalf("list-after-merge: want 2 rows got %d (%v)", len(list), list)
	}
	for _, row := range list {
		if row["result"] != "fail" {
			t.Fatalf("list-after-merge: expected result=fail got %v", row)
		}
	}
}

// TestReviewBatchInsertOnly proves the `insert` action skips pairs
// that already have a review row rather than turning them into
// updates.
func TestReviewBatchInsertOnly(t *testing.T) {
	pool := newIntegrationPool(t)
	userID, collID, assetA, assetB, benchID, ruleA, _ := seedBatchScenario(t, pool, "user-1", "alice")

	// Pre-seed a review on (assetA, ruleA) by hand via the store so
	// the batch sees it as existing.
	if _, err := store.NewReviewRepo(pool).Put(context.Background(), assetA, ruleA, userID, store.ReviewWrite{
		Result: "pass", Detail: "pre-existing", Comment: "preseed",
	}); err != nil {
		t.Fatalf("preseed review: %v", err)
	}

	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(context.Background(), auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	handler := newTestServer(t, withAuth(prov), withPool(pool))

	body, _ := json.Marshal(map[string]any{
		"action": "insert",
		"assets": map[string]any{"assetIds": []string{
			strconv.FormatInt(assetA, 10), strconv.FormatInt(assetB, 10),
		}},
		"rules": map[string]any{"benchmarkIds": []string{benchID}},
		"source": map[string]any{
			"review": map[string]any{
				"result":  "notapplicable",
				"detail":  "n/a",
				"comment": "n/a",
				"status":  "saved",
			},
		},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		"/api/collections/"+strconv.FormatInt(collID, 10)+"/reviews",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("insert: got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	// 4 pairs minus 1 pre-existing = 3 inserts; updated should stay 0.
	if resp["inserted"] != float64(3) {
		t.Fatalf("insert: want 3 inserts got %v (resp=%v)", resp["inserted"], resp)
	}
	if resp["updated"] != float64(0) {
		t.Fatalf("insert: want 0 updates got %v", resp["updated"])
	}

	// The pre-existing review should still report the original detail.
	pre, err := store.NewReviewRepo(pool).Get(context.Background(), assetA, ruleA)
	if err != nil {
		t.Fatalf("get preseeded: %v", err)
	}
	if pre.Detail != "pre-existing" {
		t.Fatalf("insert clobbered preseed: %+v", pre)
	}
}

// TestReviewBatchUpdateWithFilter exercises the `update` action with
// an `updateFilters` clause that only targets reviews currently in
// `result=pass`. The other rows must be left untouched.
func TestReviewBatchUpdateWithFilter(t *testing.T) {
	pool := newIntegrationPool(t)
	userID, collID, assetA, assetB, benchID, ruleA, ruleB := seedBatchScenario(t, pool, "user-1", "alice")
	_ = benchID

	rr := store.NewReviewRepo(pool)
	ctx := context.Background()
	mustPut := func(assetID int64, ruleID, result string) {
		if _, err := rr.Put(ctx, assetID, ruleID, userID, store.ReviewWrite{
			Result: result, Detail: "seed", Comment: "seed",
		}); err != nil {
			t.Fatalf("seed review (%d,%s): %v", assetID, ruleID, err)
		}
	}
	mustPut(assetA, ruleA, "pass")
	mustPut(assetA, ruleB, "fail")
	mustPut(assetB, ruleA, "pass")
	mustPut(assetB, ruleB, "fail")

	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(context.Background(), auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	handler := newTestServer(t, withAuth(prov), withPool(pool))

	body, _ := json.Marshal(map[string]any{
		"action": "update",
		"assets": map[string]any{"assetIds": []string{
			strconv.FormatInt(assetA, 10), strconv.FormatInt(assetB, 10),
		}},
		"rules": map[string]any{"ruleIds": []string{ruleA, ruleB}},
		"source": map[string]any{
			"review": map[string]any{"comment": "post-filter"},
		},
		"updateFilters": []map[string]any{
			{"field": "result", "value": "pass", "condition": "equals"},
		},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		"/api/collections/"+strconv.FormatInt(collID, 10)+"/reviews",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update-filter: got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["updated"] != float64(2) {
		t.Fatalf("update-filter: want 2 updated (both pass rows), got %v (resp=%v)", resp["updated"], resp)
	}
	if resp["inserted"] != float64(0) {
		t.Fatalf("update-filter: want 0 inserted got %v", resp["inserted"])
	}

	check := func(assetID int64, ruleID, wantComment string) {
		r, err := rr.Get(ctx, assetID, ruleID)
		if err != nil {
			t.Fatalf("get (%d,%s): %v", assetID, ruleID, err)
		}
		if r.Comment != wantComment {
			t.Fatalf("(%d,%s) comment: want %q got %q", assetID, ruleID, wantComment, r.Comment)
		}
	}
	check(assetA, ruleA, "post-filter") // was pass → updated
	check(assetB, ruleA, "post-filter") // was pass → updated
	check(assetA, ruleB, "seed")        // was fail → untouched
	check(assetB, ruleB, "seed")        // was fail → untouched
}

// TestReviewBatchDryRun verifies dryRun=true returns the
// ReviewBatchResponseDryRun shape and does NOT mutate the database.
func TestReviewBatchDryRun(t *testing.T) {
	pool := newIntegrationPool(t)
	_, collID, assetA, assetB, benchID, ruleA, ruleB := seedBatchScenario(t, pool, "user-1", "alice")
	_ = benchID
	_ = ruleA
	_ = ruleB

	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(context.Background(), auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	handler := newTestServer(t, withAuth(prov), withPool(pool))

	body, _ := json.Marshal(map[string]any{
		"action": "merge",
		"dryRun": true,
		"assets": map[string]any{"assetIds": []string{
			strconv.FormatInt(assetA, 10), strconv.FormatInt(assetB, 10),
		}},
		"rules": map[string]any{"benchmarkIds": []string{benchID}},
		"source": map[string]any{
			"review": map[string]any{
				"result":  "pass",
				"detail":  "dry",
				"comment": "dry",
			},
		},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		"/api/collections/"+strconv.FormatInt(collID, 10)+"/reviews",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("dryrun: got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["willInsert"] != float64(4) {
		t.Fatalf("willInsert: want 4 got %v (resp=%v)", resp["willInsert"], resp)
	}
	if resp["willUpdate"] != float64(0) {
		t.Fatalf("willUpdate: want 0 got %v", resp["willUpdate"])
	}
	if resp["willFailValidation"] != float64(0) {
		t.Fatalf("willFailValidation: want 0 got %v", resp["willFailValidation"])
	}

	// Confirm no actual reviews were written.
	rows, err := store.NewReviewRepo(pool).List(context.Background(), store.ListReviewsOptions{CollectionID: collID})
	if err != nil {
		t.Fatalf("list after dryrun: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("dryrun wrote %d rows", len(rows))
	}
}

// TestReviewBatchInsertValidation: merge into a pair where there's no
// review row yet but source.review omits required fields (detail/
// comment). The pair should land in validationErrors[] rather than
// being silently dropped.
func TestReviewBatchInsertValidation(t *testing.T) {
	pool := newIntegrationPool(t)
	_, collID, assetA, _, benchID, _, _ := seedBatchScenario(t, pool, "user-1", "alice")

	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(context.Background(), auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	handler := newTestServer(t, withAuth(prov), withPool(pool))

	body, _ := json.Marshal(map[string]any{
		"action": "merge",
		"assets": map[string]any{"assetIds": []string{strconv.FormatInt(assetA, 10)}},
		"rules":  map[string]any{"benchmarkIds": []string{benchID}},
		"source": map[string]any{
			"review": map[string]any{"result": "pass"}, // missing detail+comment
		},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		"/api/collections/"+strconv.FormatInt(collID, 10)+"/reviews",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("validation: got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["failedValidation"] != float64(2) {
		t.Fatalf("failedValidation: want 2 got %v (resp=%v)", resp["failedValidation"], resp)
	}
	verr, _ := resp["validationErrors"].([]any)
	if len(verr) != 2 {
		t.Fatalf("validationErrors: want 2 got %d (%v)", len(verr), verr)
	}
}
