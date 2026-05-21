//go:build integration

package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Exonical/stig-manager-react/api/internal/auth"
	"github.com/Exonical/stig-manager-react/api/internal/store"
)

// seedCollectionWithOwner creates a single collection that grants the
// requested user the Owner role and returns the resulting ids. The
// matching app_user is also created.
func seedCollectionWithOwner(t *testing.T, pool *pgxpool.Pool, subject, username string) (userID, collectionID int64) {
	t.Helper()
	ctx := context.Background()
	users := store.NewUserRepo(pool)
	cols := store.NewCollectionRepo(pool)

	owner, err := users.Upsert(ctx, subject, username, username, username+"@example.com", nil)
	if err != nil {
		t.Fatalf("upsert owner: %v", err)
	}
	col, err := cols.Create(ctx, store.CollectionCreate{
		Name:   "Assets-HTTP-" + username,
		Grants: []store.CollectionCreateGrant{{UserID: owner.UserID, RoleID: 4}},
	})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}
	return owner.UserID, col.CollectionID
}

func TestAssetsHTTPFlow(t *testing.T) {
	pool := newIntegrationPool(t)

	// JWT fixture issues sub="user-1"; seed that user as the Owner
	// so authorizeCollection passes the grant check.
	_, collID := seedCollectionWithOwner(t, pool, "user-1", "alice")

	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(context.Background(), auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	handler := newTestServer(t, withAuth(prov), withPool(pool))

	collStr := strconv.FormatInt(collID, 10)

	// list (empty) returns [].
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/assets?collectionId="+collStr, nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list empty: got %d body=%s", rec.Code, rec.Body.String())
	}
	var empty []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &empty)
	if len(empty) != 0 {
		t.Fatalf("list empty: got %d items", len(empty))
	}

	// create one.
	body := map[string]any{
		"collectionId": collStr,
		"name":         "host-1",
		"fqdn":         "host1.example.com",
		"ip":           "10.0.0.1",
		"noncomputing": false,
		"metadata":     map[string]any{},
		"stigs":        []string{},
		"labelIds":     []string{},
	}
	raw, _ := json.Marshal(body)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/assets", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: got %d body=%s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	assetID, _ := created["assetId"].(string)
	if assetID == "" {
		t.Fatalf("create: missing assetId: %v", created)
	}

	// create with wrong scope → 403.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/assets", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("create wrong scope: got %d want 403 (body=%s)", rec.Code, rec.Body.String())
	}

	// list one.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/assets?collectionId="+collStr, nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list one: got %d", rec.Code)
	}
	var list []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 || list[0]["assetId"] != assetID {
		t.Fatalf("list one: %v", list)
	}

	// patch description.
	patch, _ := json.Marshal(map[string]any{"description": "patched"})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api/assets/"+assetID, bytes.NewReader(patch))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: got %d body=%s", rec.Code, rec.Body.String())
	}
	var patched map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &patched)
	if patched["description"] != "patched" {
		t.Fatalf("patch: got %v", patched["description"])
	}

	// delete.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/assets/"+assetID, nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusNoContent {
		t.Fatalf("delete: got %d body=%s", rec.Code, rec.Body.String())
	}

	// get after delete → 404.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/assets/"+assetID, nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("post-delete get: got %d want 404", rec.Code)
	}
}

func TestLabelsHTTPFlow(t *testing.T) {
	pool := newIntegrationPool(t)
	_, collID := seedCollectionWithOwner(t, pool, "user-1", "alice")

	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(context.Background(), auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	handler := newTestServer(t, withAuth(prov), withPool(pool))

	base := fmt.Sprintf("/api/collections/%d/labels", collID)

	// create.
	body, _ := json.Marshal(map[string]any{
		"name": "Red", "color": "ff0000", "description": "danger",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, base, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("label create: got %d body=%s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	labelID, _ := created["labelId"].(string)
	if labelID == "" {
		t.Fatalf("create: missing labelId: %v", created)
	}

	// list returns one.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, base, nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("label list: got %d", rec.Code)
	}
	var list []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("label list: got %d want 1", len(list))
	}

	// patch.
	patch, _ := json.Marshal(map[string]any{"color": "00ff00"})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, base+"/"+labelID, bytes.NewReader(patch))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("label patch: got %d body=%s", rec.Code, rec.Body.String())
	}

	// delete.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, base+"/"+labelID, nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("label delete: got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGrantsHTTPFlow(t *testing.T) {
	pool := newIntegrationPool(t)
	ownerID, collID := seedCollectionWithOwner(t, pool, "user-1", "alice")

	// Seed a second user we'll add a grant for.
	users := store.NewUserRepo(pool)
	other, err := users.Upsert(context.Background(), "grant-other", "olive", "Olive", "olive@example.com", nil)
	if err != nil {
		t.Fatalf("upsert other: %v", err)
	}

	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(context.Background(), auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	handler := newTestServer(t, withAuth(prov), withPool(pool))

	base := fmt.Sprintf("/api/collections/%d/grants", collID)

	// list returns the seeded grant for owner.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, base, nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: got %d body=%s", rec.Code, rec.Body.String())
	}
	var list []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("list seeded: got %d want 1", len(list))
	}

	_ = ownerID // silence "declared and not used" when assertions trim

	// post a new grant.
	body, _ := json.Marshal([]map[string]any{{
		"userId": strconv.FormatInt(other.UserID, 10),
		"roleId": 3,
	}})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, base, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("post: got %d body=%s", rec.Code, rec.Body.String())
	}

	// list returns two.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, base, nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list2: got %d", rec.Code)
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 2 {
		t.Fatalf("list2: got %d want 2", len(list))
	}
}
