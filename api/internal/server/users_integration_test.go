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


func usersServer(t *testing.T, pool *pgxpool.Pool) (http.Handler, *oidcFixture) {
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

// TestUsersAdminCRUD walks the full POST/GET/PATCH/DELETE surface for
// /users against a real Postgres. Auth + scope gating is exercised
// via the OIDC fixture.
func TestUsersAdminCRUD(t *testing.T) {
	pool := newIntegrationPool(t)
	ctx := context.Background()

	// Seed an owner user + collection so the new admin user can be
	// granted access.
	owner, err := store.NewUserRepo(pool).Upsert(ctx, "owner-sub-1", "owner-1", "Owner", "owner@example.com", nil)
	if err != nil {
		t.Fatalf("seed owner: %v", err)
	}
	coll, err := store.NewCollectionRepo(pool).Create(ctx, store.CollectionCreate{
		Name:        "user-it-coll",
		Description: "users admin integration",
		Grants:      []store.CollectionCreateGrant{{UserID: owner.UserID, RoleID: 4}},
	})
	if err != nil {
		t.Fatalf("seed collection: %v", err)
	}

	handler, fx := usersServer(t, pool)
	tokWrite := fx.token(t, "stig-manager:user")
	tokRead := fx.token(t, "stig-manager:user:read")

	// CREATE
	body := map[string]any{
		"username":         "alice",
		"collectionGrants": []map[string]any{{"collectionId": strconv.FormatInt(coll.CollectionID, 10), "roleId": 2}},
	}
	enc, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/users?projection=collectionGrants", bytes.NewReader(enc))
	req.Header.Set("Authorization", "Bearer "+tokWrite)
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: got %d body=%s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	userID, ok := created["userId"].(string)
	if !ok || userID == "" {
		t.Fatalf("created.userId missing: %v", created)
	}
	if created["username"] != "alice" {
		t.Fatalf("username: got %v", created["username"])
	}
	grants, _ := created["collectionGrants"].([]any)
	if len(grants) != 1 {
		t.Fatalf("collectionGrants: got %v", created["collectionGrants"])
	}

	// LIST (by username)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/users?username=alice&username-match=exact", nil)
	req.Header.Set("Authorization", "Bearer "+tokRead)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: got %d body=%s", rec.Code, rec.Body.String())
	}
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 1 || list[0]["username"] != "alice" {
		t.Fatalf("list: got %v", list)
	}

	// PATCH (status only)
	patch := map[string]any{"status": "unavailable"}
	enc, _ = json.Marshal(patch)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api/users/"+userID, bytes.NewReader(enc))
	req.Header.Set("Authorization", "Bearer "+tokWrite)
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: got %d body=%s", rec.Code, rec.Body.String())
	}
	var patched map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &patched)
	if patched["status"] != "unavailable" {
		t.Fatalf("patch status: got %v", patched["status"])
	}

	// GET single (with projections)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/users/"+userID+"?projection=collectionGrants&projection=userGroups", nil)
	req.Header.Set("Authorization", "Bearer "+tokRead)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: got %d body=%s", rec.Code, rec.Body.String())
	}
	var single map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &single); err != nil {
		t.Fatalf("decode single: %v", err)
	}
	if _, ok := single["collectionGrants"]; !ok {
		t.Errorf("collectionGrants projection missing")
	}
	if _, ok := single["userGroups"]; !ok {
		t.Errorf("userGroups projection missing")
	}

	// DELETE (never accessed) — should succeed since sub IS NULL.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/users/"+userID, nil)
	req.Header.Set("Authorization", "Bearer "+tokWrite)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: got %d body=%s", rec.Code, rec.Body.String())
	}

	// GET after delete → 404.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/users/"+userID, nil)
	req.Header.Set("Authorization", "Bearer "+tokRead)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get-after-delete: got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestUsersAuth covers the basic auth/scope gating: 401 no token, 403
// wrong scope.
func TestUsersAuth(t *testing.T) {
	pool := newIntegrationPool(t)
	handler, fx := usersServer(t, pool)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-token: got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/users", nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("wrong-scope: got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestUserGroupsAdminCRUD walks the full POST/GET/PATCH/DELETE surface
// for /user-groups and verifies group-scoped collection grants survive
// the migration cleanly.
func TestUserGroupsAdminCRUD(t *testing.T) {
	pool := newIntegrationPool(t)
	ctx := context.Background()

	// Seed two users + a collection so the new group can pull them
	// in as members and target the collection in a grant.
	users := store.NewUserRepo(pool)
	alice, err := users.CreateAdmin(ctx, store.UserCreate{Username: "ug-alice"})
	if err != nil {
		t.Fatalf("seed alice: %v", err)
	}
	bob, err := users.CreateAdmin(ctx, store.UserCreate{Username: "ug-bob"})
	if err != nil {
		t.Fatalf("seed bob: %v", err)
	}
	owner, err := store.NewUserRepo(pool).Upsert(ctx, "owner-sub-2", "owner-2", "Owner", "owner@example.com", nil)
	if err != nil {
		t.Fatalf("seed owner: %v", err)
	}
	coll, err := store.NewCollectionRepo(pool).Create(ctx, store.CollectionCreate{
		Name:        "ug-it-coll",
		Description: "user groups integration",
		Grants:      []store.CollectionCreateGrant{{UserID: owner.UserID, RoleID: 4}},
	})
	if err != nil {
		t.Fatalf("seed collection: %v", err)
	}

	handler, fx := usersServer(t, pool)
	tokWrite := fx.token(t, "stig-manager:user")
	tokRead := fx.token(t, "stig-manager:user:read")

	// CREATE group with both users and a collection grant.
	body := map[string]any{
		"name":        "operators",
		"description": "ops",
		"userIds": []string{
			strconv.FormatInt(alice.UserID, 10),
			strconv.FormatInt(bob.UserID, 10),
		},
		"collectionGrants": []map[string]any{
			{"collectionId": strconv.FormatInt(coll.CollectionID, 10), "roleId": 3},
		},
	}
	enc, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/user-groups?projection=users&projection=collectionGrants", bytes.NewReader(enc))
	req.Header.Set("Authorization", "Bearer "+tokWrite)
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: got %d body=%s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	gid, _ := created["userGroupId"].(string)
	if gid == "" {
		t.Fatalf("userGroupId missing: %v", created)
	}
	if created["name"] != "operators" {
		t.Fatalf("name: got %v", created["name"])
	}
	members, _ := created["users"].([]any)
	if len(members) != 2 {
		t.Fatalf("expected 2 users, got %v", created["users"])
	}
	grants, _ := created["collectionGrants"].([]any)
	if len(grants) != 1 {
		t.Fatalf("expected 1 group grant, got %v", created["collectionGrants"])
	}

	// LIST
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/user-groups", nil)
	req.Header.Set("Authorization", "Bearer "+tokRead)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: got %d body=%s", rec.Code, rec.Body.String())
	}
	var list []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 || list[0]["userGroupId"] != gid {
		t.Fatalf("list: got %v", list)
	}

	// PATCH — drop bob from the group; keep grants untouched.
	patch := map[string]any{
		"userIds": []string{strconv.FormatInt(alice.UserID, 10)},
	}
	enc, _ = json.Marshal(patch)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api/user-groups/"+gid+"?projection=users", bytes.NewReader(enc))
	req.Header.Set("Authorization", "Bearer "+tokWrite)
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: got %d body=%s", rec.Code, rec.Body.String())
	}
	var patched map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &patched)
	patchedMembers, _ := patched["users"].([]any)
	if len(patchedMembers) != 1 {
		t.Fatalf("after patch: got %v users", len(patchedMembers))
	}

	// DELETE
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/user-groups/"+gid, nil)
	req.Header.Set("Authorization", "Bearer "+tokWrite)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: got %d body=%s", rec.Code, rec.Body.String())
	}

	// GET after delete → 404.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/user-groups/"+gid, nil)
	req.Header.Set("Authorization", "Bearer "+tokRead)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get-after-delete: got %d body=%s", rec.Code, rec.Body.String())
	}

}
