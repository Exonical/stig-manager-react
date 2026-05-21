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

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Exonical/stig-manager-react/api/internal/auth"
	"github.com/Exonical/stig-manager-react/api/internal/store"
)

// seedAssetForReviews creates a collection that the JWT subject owns
// and a single asset within it. Returns the ids for the test to use
// when calling /collections/{cid}/reviews/{aid}/{rid}.
func seedAssetForReviews(t *testing.T, pool *pgxpool.Pool, subject, username string) (userID, collectionID, assetID int64) {
	t.Helper()
	userID, collectionID = seedCollectionWithOwner(t, pool, subject, username)

	assets := store.NewAssetRepo(pool)
	a, err := assets.Create(context.Background(), store.AssetCreate{
		CollectionID: collectionID,
		Name:         "review-host-" + username,
		Noncomputing: false,
	})
	if err != nil {
		t.Fatalf("create asset: %v", err)
	}
	return userID, collectionID, a.AssetID
}

func TestReviewsHTTPFlow(t *testing.T) {
	pool := newIntegrationPool(t)
	_, collID, assetID := seedAssetForReviews(t, pool, "user-1", "alice")
	collStr := strconv.FormatInt(collID, 10)
	aIDStr := strconv.FormatInt(assetID, 10)
	const ruleID = "SV-12345r1_rule"
	base := "/api/collections/" + collStr + "/reviews/" + aIDStr + "/" + ruleID

	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(context.Background(), auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	handler := newTestServer(t, withAuth(prov), withPool(pool))

	// GET before any PUT → 404.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, base, nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get-missing: got %d body=%s", rec.Code, rec.Body.String())
	}

	// PUT — create.
	put := map[string]any{
		"result":  "pass",
		"detail":  "all clear",
		"comment": "first review",
		"status": map[string]any{
			"label": "submitted",
			"text":  "ready for review",
		},
		"metadata": map[string]any{"source": "manual"},
	}
	raw, _ := json.Marshal(put)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, base, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put: got %d body=%s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created["result"] != "pass" {
		t.Fatalf("put: wrong result: %v", created)
	}
	if status, _ := created["status"].(map[string]any); status["label"] != "submitted" {
		t.Fatalf("put: wrong status: %v", status)
	}

	// GET — round-trip.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, base, nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: got %d body=%s", rec.Code, rec.Body.String())
	}

	// PATCH — change comment + status (snapshots history).
	patch := map[string]any{
		"comment": "second review",
		"status":  "accepted",
	}
	raw, _ = json.Marshal(patch)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, base, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: got %d body=%s", rec.Code, rec.Body.String())
	}
	var patched map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &patched)
	if patched["comment"] != "second review" {
		t.Fatalf("patch: wrong comment: %v", patched)
	}
	if status, _ := patched["status"].(map[string]any); status["label"] != "accepted" {
		t.Fatalf("patch: status not updated: %v", status)
	}

	// GET ?projection=history — confirm history captures the PUT snapshot.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, base+"?projection=history", nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get-history: got %d body=%s", rec.Code, rec.Body.String())
	}
	var withHistory map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &withHistory)
	history, _ := withHistory["history"].([]any)
	if len(history) < 1 {
		t.Fatalf("history missing: %v", withHistory)
	}
	// Newest history entry should hold the *pre-patch* comment.
	first, _ := history[0].(map[string]any)
	if first["comment"] != "first review" {
		t.Fatalf("history snapshot wrong: %v", first)
	}

	// DELETE.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, base, nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: got %d body=%s", rec.Code, rec.Body.String())
	}

	// GET after DELETE → 404.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, base, nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get-after-delete: got %d", rec.Code)
	}
}

func TestReviewsScopeAndRoleGating(t *testing.T) {
	pool := newIntegrationPool(t)
	_, collID, assetID := seedAssetForReviews(t, pool, "user-1", "alice")
	collStr := strconv.FormatInt(collID, 10)
	aIDStr := strconv.FormatInt(assetID, 10)
	const ruleID = "SV-99999r1_rule"
	base := "/api/collections/" + collStr + "/reviews/" + aIDStr + "/" + ruleID

	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(context.Background(), auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	handler := newTestServer(t, withAuth(prov), withPool(pool))

	put, _ := json.Marshal(map[string]any{"result": "fail"})

	// PUT without bearer → 401.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, base, bytes.NewReader(put)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("put-no-token: got %d", rec.Code)
	}

	// PUT with read-only scope → 403.
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, base, bytes.NewReader(put))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("put-wrong-scope: got %d body=%s", rec.Code, rec.Body.String())
	}

	// LIST scoped to the collection.
	list := "/api/collections/" + collStr + "/reviews"
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, list, nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: got %d body=%s", rec.Code, rec.Body.String())
	}

	// LIST without scope → 403.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, list, nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:op:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("list-wrong-scope: got %d", rec.Code)
	}
}

func TestReviewsCrossCollectionGuard(t *testing.T) {
	// The grant + asset path must agree. Posting to
	// /collections/{A}/reviews/{assetInB}/… should return 404 even when
	// the caller owns both collections.
	pool := newIntegrationPool(t)
	_, collA, _ := seedAssetForReviews(t, pool, "user-1", "alice")
	_, _, assetB := seedAssetForReviews(t, pool, "user-1", "alice2")
	collAStr := strconv.FormatInt(collA, 10)
	assetBStr := strconv.FormatInt(assetB, 10)
	const ruleID = "SV-00001r1_rule"

	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(context.Background(), auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	handler := newTestServer(t, withAuth(prov), withPool(pool))

	put, _ := json.Marshal(map[string]any{"result": "pass"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/collections/"+collAStr+"/reviews/"+assetBStr+"/"+ruleID, bytes.NewReader(put))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-collection PUT: got %d body=%s", rec.Code, rec.Body.String())
	}
}
