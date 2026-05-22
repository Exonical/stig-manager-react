//go:build integration

package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/Exonical/stig-manager-react/api/internal/auth"
	"github.com/Exonical/stig-manager-react/api/internal/store"
	"github.com/Exonical/stig-manager-react/api/internal/xccdf"
)

// TestGetStigsByCollectionHTTPFlow exercises
// GET /collections/{cid}/stigs and GET /collections/{cid}/stigs/{bid}.
func TestGetStigsByCollectionHTTPFlow(t *testing.T) {
	ctx := context.Background()
	pool := newIntegrationPool(t)
	userID, collID := seedCollectionWithOwner(t, pool, "user-1", "rachel")

	// Import the canonical XCCDF so the library has a benchmark.
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
	benchID := bench.BenchmarkID

	// Create an asset in the collection, with the STIG mapped.
	assets := store.NewAssetRepo(pool)
	a, err := assets.Create(ctx, store.AssetCreate{
		CollectionID: collID, Name: "host-1", Noncomputing: false,
	})
	if err != nil {
		t.Fatalf("create asset: %v", err)
	}
	stigs := []string{benchID}
	if _, err := assets.Update(ctx, a.AssetID, store.AssetUpdate{BenchmarkIDs: &stigs}); err != nil {
		t.Fatalf("assign stig: %v", err)
	}
	_ = userID

	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(ctx, auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	handler := newTestServer(t, withAuth(prov), withPool(pool))

	// GET /collections/{cid}/stigs → 200 with 1 row.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/collections/"+strconv.FormatInt(collID, 10)+"/stigs", nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list collection stigs: got %d body=%s", rec.Code, rec.Body.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d (%v)", len(rows), rows)
	}
	if rows[0]["benchmarkId"] != benchID {
		t.Fatalf("benchmarkId: got %v want %s", rows[0]["benchmarkId"], benchID)
	}
	if ac, _ := rows[0]["assetCount"].(float64); ac != 1 {
		t.Fatalf("assetCount: got %v want 1", rows[0]["assetCount"])
	}

	// GET /collections/{cid}/stigs/{bid} → 200.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet,
		"/api/collections/"+strconv.FormatInt(collID, 10)+"/stigs/"+benchID, nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get collection stig: got %d body=%s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if got["benchmarkId"] != benchID {
		t.Fatalf("get.benchmarkId: %v", got["benchmarkId"])
	}

	// GET /collections/{cid}/stigs/{unknown-bid} → 204.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet,
		"/api/collections/"+strconv.FormatInt(collID, 10)+"/stigs/UNKNOWN_STIG", nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("get unknown stig: got %d body=%s", rec.Code, rec.Body.String())
	}

	// Unauthorised (no token) → 401.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet,
		"/api/collections/"+strconv.FormatInt(collID, 10)+"/stigs", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anon list: got %d", rec.Code)
	}
}
