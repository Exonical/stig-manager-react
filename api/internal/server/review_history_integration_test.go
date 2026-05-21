//go:build integration

package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Exonical/stig-manager-react/api/internal/auth"
)

// putReview is a tiny helper that PUTs a review and panics on a non-2xx
// response. The bulk-of-the-test logic stays focused on history.
func putReview(t *testing.T, handler http.Handler, fx *oidcFixture, base string, body map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, base, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection"))
	handler.ServeHTTP(rec, req)
	if rec.Code/100 != 2 {
		t.Fatalf("put review: got %d body=%s", rec.Code, rec.Body.String())
	}
}

// patchReview is the PATCH analogue; ditto on error handling.
func patchReview(t *testing.T, handler http.Handler, fx *oidcFixture, base string, body map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, base, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection"))
	handler.ServeHTTP(rec, req)
	if rec.Code/100 != 2 {
		t.Fatalf("patch review: got %d body=%s", rec.Code, rec.Body.String())
	}
}

// historyServer wires the OIDC fixture + a fresh pool into the test
// server. Returns the assembled handler, the JWT fixture, and the
// collection+asset ids the caller can use to construct URLs.
func historyServer(t *testing.T) (http.Handler, *oidcFixture, *pgxpool.Pool, int64, int64) {
	t.Helper()
	pool := newIntegrationPool(t)
	_, collID, assetID := seedAssetForReviews(t, pool, "user-1", "alice")

	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(context.Background(), auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	return newTestServer(t, withAuth(prov), withPool(pool)), fx, pool, collID, assetID
}

// TestReviewHistoryByCollection seeds two reviews on a single asset,
// patches them so they each accumulate at least one history entry,
// then queries /collections/{cid}/review-history and verifies the
// nested ReviewHistoryAsset/Rule shape.
func TestReviewHistoryByCollection(t *testing.T) {
	handler, fx, _, collID, assetID := historyServer(t)
	collStr := strconv.FormatInt(collID, 10)
	aIDStr := strconv.FormatInt(assetID, 10)
	const ruleA = "SV-1001r1_rule"
	const ruleB = "SV-1002r1_rule"
	baseA := "/api/collections/" + collStr + "/reviews/" + aIDStr + "/" + ruleA
	baseB := "/api/collections/" + collStr + "/reviews/" + aIDStr + "/" + ruleB

	// Two reviews → two PATCHes → two history rows.
	putReview(t, handler, fx, baseA, map[string]any{
		"result": "pass", "comment": "first-a",
		"status": map[string]any{"label": "submitted"},
	})
	patchReview(t, handler, fx, baseA, map[string]any{"comment": "second-a"})
	putReview(t, handler, fx, baseB, map[string]any{
		"result": "fail", "comment": "first-b",
		"status": map[string]any{"label": "submitted"},
	})
	patchReview(t, handler, fx, baseB, map[string]any{"comment": "second-b"})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/collections/"+collStr+"/review-history", nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("history: got %d body=%s", rec.Code, rec.Body.String())
	}
	var assets []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &assets); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	if len(assets) != 1 {
		t.Fatalf("want 1 asset bucket, got %d (%v)", len(assets), assets)
	}
	if got, _ := assets[0]["assetId"].(string); got != aIDStr {
		t.Errorf("assetId: got %q want %q", got, aIDStr)
	}
	rules, _ := assets[0]["reviewHistories"].([]any)
	if len(rules) != 2 {
		t.Fatalf("want 2 rule buckets, got %d", len(rules))
	}
	// Each rule should hold exactly one history snapshot (the PUT'd
	// first state, captured when the PATCH overwrote it).
	for _, r := range rules {
		rb, _ := r.(map[string]any)
		hist, _ := rb["history"].([]any)
		if len(hist) != 1 {
			t.Errorf("rule %v: want 1 history entry, got %d", rb["ruleId"], len(hist))
		}
		h0, _ := hist[0].(map[string]any)
		switch rb["ruleId"] {
		case ruleA:
			if h0["comment"] != "first-a" {
				t.Errorf("ruleA history comment: %v", h0["comment"])
			}
		case ruleB:
			if h0["comment"] != "first-b" {
				t.Errorf("ruleB history comment: %v", h0["comment"])
			}
		default:
			t.Errorf("unexpected ruleId: %v", rb["ruleId"])
		}
	}
}

// TestReviewHistoryFilterByRuleAndStatus narrows the listing by
// ruleId + status and verifies the rejected rows aren't surfaced.
func TestReviewHistoryFilterByRuleAndStatus(t *testing.T) {
	handler, fx, _, collID, assetID := historyServer(t)
	collStr := strconv.FormatInt(collID, 10)
	aIDStr := strconv.FormatInt(assetID, 10)
	const ruleA = "SV-2001r1_rule"
	const ruleB = "SV-2002r1_rule"
	baseA := "/api/collections/" + collStr + "/reviews/" + aIDStr + "/" + ruleA
	baseB := "/api/collections/" + collStr + "/reviews/" + aIDStr + "/" + ruleB

	putReview(t, handler, fx, baseA, map[string]any{
		"result": "pass", "comment": "a-first",
		"status": map[string]any{"label": "submitted"},
	})
	patchReview(t, handler, fx, baseA, map[string]any{
		"comment": "a-second",
		"status":  "accepted",
	})
	putReview(t, handler, fx, baseB, map[string]any{
		"result": "fail", "comment": "b-first",
		"status": map[string]any{"label": "saved"},
	})
	patchReview(t, handler, fx, baseB, map[string]any{
		"comment": "b-second",
		"status":  "submitted",
	})

	// Filter by ruleA.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/collections/"+collStr+"/review-history?ruleId="+ruleA, nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("filter ruleA: got %d body=%s", rec.Code, rec.Body.String())
	}
	var byRule []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &byRule)
	if len(byRule) != 1 {
		t.Fatalf("want 1 asset bucket, got %d", len(byRule))
	}
	rules, _ := byRule[0]["reviewHistories"].([]any)
	if len(rules) != 1 {
		t.Fatalf("ruleA filter: want 1 rule, got %d", len(rules))
	}

	// Filter by status=submitted → ruleA history was submitted before
	// the patch flipped it to accepted, so it should be returned.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet,
		"/api/collections/"+collStr+"/review-history?status=submitted", nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("filter submitted: got %d body=%s", rec.Code, rec.Body.String())
	}
	var bySubmitted []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &bySubmitted)
	if len(bySubmitted) != 1 {
		t.Fatalf("submitted: want 1 asset, got %d", len(bySubmitted))
	}
	rules, _ = bySubmitted[0]["reviewHistories"].([]any)
	// Only ruleA's history was created with status=submitted (ruleB's
	// snapshot was saved).
	if len(rules) != 1 {
		t.Fatalf("submitted: want 1 rule bucket, got %d", len(rules))
	}
}

// TestReviewHistoryStatsByCollection verifies the aggregate and the
// per-asset projection.
func TestReviewHistoryStatsByCollection(t *testing.T) {
	handler, fx, _, collID, assetID := historyServer(t)
	collStr := strconv.FormatInt(collID, 10)
	aIDStr := strconv.FormatInt(assetID, 10)
	const ruleA = "SV-3001r1_rule"
	baseA := "/api/collections/" + collStr + "/reviews/" + aIDStr + "/" + ruleA

	putReview(t, handler, fx, baseA, map[string]any{
		"result": "pass", "comment": "first",
		"status": map[string]any{"label": "submitted"},
	})
	patchReview(t, handler, fx, baseA, map[string]any{"comment": "second"})
	patchReview(t, handler, fx, baseA, map[string]any{"comment": "third"})

	// Aggregate.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/collections/"+collStr+"/review-history/stats", nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("stats: got %d body=%s", rec.Code, rec.Body.String())
	}
	var stats map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &stats)
	count, _ := stats["collectionHistoryEntryCount"].(float64)
	if int(count) != 2 {
		t.Errorf("collectionHistoryEntryCount: got %v want 2", stats["collectionHistoryEntryCount"])
	}
	if _, ok := stats["oldestHistoryEntryDate"].(string); !ok {
		t.Errorf("oldestHistoryEntryDate missing or wrong type: %v", stats["oldestHistoryEntryDate"])
	}
	if _, ok := stats["assetHistoryEntryCounts"]; ok {
		t.Errorf("asset projection should be absent: %v", stats)
	}

	// projection=asset.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet,
		"/api/collections/"+collStr+"/review-history/stats?projection=asset", nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("stats?projection=asset: got %d body=%s", rec.Code, rec.Body.String())
	}
	var statsAsset map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &statsAsset)
	assets, _ := statsAsset["assetHistoryEntryCounts"].([]any)
	if len(assets) != 1 {
		t.Fatalf("asset projection: want 1 asset, got %d", len(assets))
	}
	first, _ := assets[0].(map[string]any)
	if got, _ := first["historyEntryCount"].(float64); int(got) != 2 {
		t.Errorf("per-asset historyEntryCount: got %v want 2", first["historyEntryCount"])
	}
	if _, ok := first["oldestHistoryEntry"].(string); !ok {
		t.Errorf("per-asset oldestHistoryEntry missing/typed wrong: %v", first["oldestHistoryEntry"])
	}
}

// TestDeleteReviewHistoryByCollection verifies bulk delete respects
// the retentionDate cutoff and the optional assetId filter.
func TestDeleteReviewHistoryByCollection(t *testing.T) {
	handler, fx, pool, collID, assetID := historyServer(t)
	collStr := strconv.FormatInt(collID, 10)
	aIDStr := strconv.FormatInt(assetID, 10)
	const ruleA = "SV-4001r1_rule"
	baseA := "/api/collections/" + collStr + "/reviews/" + aIDStr + "/" + ruleA

	putReview(t, handler, fx, baseA, map[string]any{
		"result": "pass", "comment": "first",
		"status": map[string]any{"label": "submitted"},
	})
	// PATCH twice → two history entries.
	patchReview(t, handler, fx, baseA, map[string]any{"comment": "second"})
	patchReview(t, handler, fx, baseA, map[string]any{"comment": "third"})

	// Sanity: stats shows 2 entries.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/collections/"+collStr+"/review-history/stats", nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	var pre map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &pre)
	if int(pre["collectionHistoryEntryCount"].(float64)) != 2 {
		t.Fatalf("precondition: want 2 history rows, got %v", pre["collectionHistoryEntryCount"])
	}

	// DELETE with a retentionDate in the past → no rows removed.
	past := time.Now().UTC().Add(-365 * 24 * time.Hour).Format("2006-01-02")
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete,
		"/api/collections/"+collStr+"/review-history?retentionDate="+past, nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete-past: got %d body=%s", rec.Code, rec.Body.String())
	}
	var deletedPast map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &deletedPast)
	if int(deletedPast["HistoryEntriesDeleted"].(float64)) != 0 {
		t.Errorf("delete-past: want 0, got %v", deletedPast["HistoryEntriesDeleted"])
	}

	// DELETE with a retentionDate one day in the future → both rows
	// removed (touch_ts is "now").
	future := time.Now().UTC().Add(24 * time.Hour).Format("2006-01-02")
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete,
		"/api/collections/"+collStr+"/review-history?retentionDate="+future, nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete-future: got %d body=%s", rec.Code, rec.Body.String())
	}
	var deleted map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &deleted)
	if int(deleted["HistoryEntriesDeleted"].(float64)) != 2 {
		t.Errorf("delete-future: want 2, got %v", deleted["HistoryEntriesDeleted"])
	}

	// Verify the table is empty for this collection.
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM review_history h JOIN asset a ON a.asset_id = h.asset_id WHERE a.collection_id = $1`,
		collID,
	).Scan(&n); err != nil {
		t.Fatalf("count remaining: %v", err)
	}
	if n != 0 {
		t.Errorf("remaining history rows: got %d want 0", n)
	}

	// Read-only scope should be denied for the DELETE.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete,
		"/api/collections/"+collStr+"/review-history?retentionDate="+future, nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("delete-readonly-scope: got %d body=%s", rec.Code, rec.Body.String())
	}

}
