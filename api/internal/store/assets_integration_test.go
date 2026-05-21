//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Exonical/stig-manager-react/api/internal/store"
)

// TestAssetRepo_CRUD walks an asset through create, list, get, update,
// and delete, verifying the projections round-trip through Postgres.
func TestAssetRepo_CRUD(t *testing.T) {
	pool := newPGPool(t)
	ctx := context.Background()
	users := store.NewUserRepo(pool)
	cols := store.NewCollectionRepo(pool)
	assets := store.NewAssetRepo(pool)

	owner, err := users.Upsert(ctx, "owner-1", "owner", "Owner", "owner@example.com", nil)
	if err != nil {
		t.Fatalf("upsert owner: %v", err)
	}
	col, err := cols.Create(ctx, store.CollectionCreate{
		Name: "Assets-Coll",
		Grants: []store.CollectionCreateGrant{{
			UserID: owner.UserID, RoleID: 4,
		}},
	})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}

	// Create
	asset, err := assets.Create(ctx, store.AssetCreate{
		CollectionID: col.CollectionID,
		Name:         "host-alpha",
		FQDN:         "alpha.example.com",
		IP:           "10.0.0.1",
		Description:  "first",
	})
	if err != nil {
		t.Fatalf("create asset: %v", err)
	}
	if asset.AssetID == 0 || asset.State != "enabled" {
		t.Fatalf("create: %+v", asset)
	}

	// Duplicate name in same collection → ErrDuplicateName.
	if _, err := assets.Create(ctx, store.AssetCreate{
		CollectionID: col.CollectionID,
		Name:         "HOST-ALPHA",
	}); !errors.Is(err, store.ErrDuplicateName) {
		t.Fatalf("dup: got %v want ErrDuplicateName", err)
	}

	// Get / List.
	got, err := assets.Get(ctx, asset.AssetID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "host-alpha" || got.FQDN != "alpha.example.com" {
		t.Fatalf("get: %+v", got)
	}
	list, err := assets.List(ctx, store.ListAssetsOptions{CollectionID: col.CollectionID})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list: got %d want 1", len(list))
	}

	// Patch.
	newDesc := "updated"
	updated, err := assets.Update(ctx, asset.AssetID, store.AssetUpdate{
		Description: &newDesc,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Description != "updated" {
		t.Fatalf("update: %+v", updated)
	}

	// Delete soft-deletes; subsequent Get → ErrNotFound.
	if err := assets.Delete(ctx, asset.AssetID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := assets.Get(ctx, asset.AssetID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("post-delete get: got %v want ErrNotFound", err)
	}
}

// TestLabelRepo_CRUDAndAssetMapping covers the label CRUD surface and
// the apply/replace flow.
func TestLabelRepo_CRUDAndAssetMapping(t *testing.T) {
	pool := newPGPool(t)
	ctx := context.Background()
	users := store.NewUserRepo(pool)
	cols := store.NewCollectionRepo(pool)
	labels := store.NewLabelRepo(pool)
	assets := store.NewAssetRepo(pool)

	owner, err := users.Upsert(ctx, "labelowner", "owner", "Owner", "owner@example.com", nil)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	col, err := cols.Create(ctx, store.CollectionCreate{
		Name:   "Labels-Coll",
		Grants: []store.CollectionCreateGrant{{UserID: owner.UserID, RoleID: 4}},
	})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}

	// Two labels.
	red, err := labels.Create(ctx, store.LabelCreate{
		CollectionID: col.CollectionID, Name: "Red", Color: "ff0000",
	})
	if err != nil {
		t.Fatalf("create red: %v", err)
	}
	if _, err := labels.Create(ctx, store.LabelCreate{
		CollectionID: col.CollectionID, Name: "RED", Color: "00ff00",
	}); !errors.Is(err, store.ErrDuplicateName) {
		t.Fatalf("dup: got %v want ErrDuplicateName", err)
	}
	if _, err := labels.Create(ctx, store.LabelCreate{
		CollectionID: col.CollectionID, Name: "Blue", Color: "0000ff",
	}); err != nil {
		t.Fatalf("create blue: %v", err)
	}

	list, err := labels.List(ctx, col.CollectionID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list: got %d want 2", len(list))
	}

	// Patch.
	newDesc := "danger"
	patched, err := labels.Patch(ctx, col.CollectionID, red.LabelID, store.LabelUpdate{
		Description: &newDesc,
	})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if patched.Description != "danger" {
		t.Fatalf("patch: %+v", patched)
	}

	// Apply label to a new asset.
	asset, err := assets.Create(ctx, store.AssetCreate{
		CollectionID: col.CollectionID,
		Name:         "tagged",
	})
	if err != nil {
		t.Fatalf("create asset: %v", err)
	}
	if err := labels.SetAssets(ctx, col.CollectionID, red.LabelID, []int64{asset.AssetID}); err != nil {
		t.Fatalf("set assets: %v", err)
	}
	ids, err := labels.AssetIDs(ctx, col.CollectionID, red.LabelID)
	if err != nil {
		t.Fatalf("asset ids: %v", err)
	}
	if len(ids) != 1 || ids[0] != asset.AssetID {
		t.Fatalf("asset ids: %v want [%d]", ids, asset.AssetID)
	}

	// Empty replace clears.
	if err := labels.SetAssets(ctx, col.CollectionID, red.LabelID, nil); err != nil {
		t.Fatalf("clear: %v", err)
	}
	ids, _ = labels.AssetIDs(ctx, col.CollectionID, red.LabelID)
	if len(ids) != 0 {
		t.Fatalf("post-clear: %v want []", ids)
	}

	// Delete.
	if err := labels.Delete(ctx, col.CollectionID, red.LabelID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := labels.Get(ctx, col.CollectionID, red.LabelID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("post-delete: got %v want ErrNotFound", err)
	}
}

// TestGrantRepo_CRUDAndLastOwner asserts the basic grant CRUD surface
// plus the "cannot remove the last Owner" guard.
func TestGrantRepo_CRUDAndLastOwner(t *testing.T) {
	pool := newPGPool(t)
	ctx := context.Background()
	users := store.NewUserRepo(pool)
	cols := store.NewCollectionRepo(pool)
	grants := store.NewGrantRepo(pool)

	owner, err := users.Upsert(ctx, "grant-owner", "owner", "Owner", "owner@example.com", nil)
	if err != nil {
		t.Fatalf("upsert owner: %v", err)
	}
	col, err := cols.Create(ctx, store.CollectionCreate{
		Name:   "Grants-Coll",
		Grants: []store.CollectionCreateGrant{{UserID: owner.UserID, RoleID: 4}},
	})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}

	// The Create already seeded a grant for owner.
	list, err := grants.List(ctx, col.CollectionID)
	if err != nil {
		t.Fatalf("list seeded: %v", err)
	}
	if len(list) != 1 || list[0].RoleID != 4 {
		t.Fatalf("seeded grant: %+v", list)
	}
	ownerGrantID := list[0].GrantID

	// Adding a second user via GrantRepo.
	other, err := users.Upsert(ctx, "grant-other", "other", "Other", "other@example.com", nil)
	if err != nil {
		t.Fatalf("upsert other: %v", err)
	}
	g, err := grants.Create(ctx, store.GrantCreate{
		CollectionID: col.CollectionID, UserID: other.UserID, RoleID: 3,
	})
	if err != nil {
		t.Fatalf("create grant: %v", err)
	}
	if g.RoleID != 3 || g.UserID != other.UserID {
		t.Fatalf("create grant: %+v", g)
	}

	// Cannot create a duplicate.
	if _, err := grants.Create(ctx, store.GrantCreate{
		CollectionID: col.CollectionID, UserID: other.UserID, RoleID: 2,
	}); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("dup: got %v want ErrConflict", err)
	}

	// Update role.
	g2, err := grants.UpdateRole(ctx, col.CollectionID, g.GrantID, 4)
	if err != nil {
		t.Fatalf("update role: %v", err)
	}
	if g2.RoleID != 4 {
		t.Fatalf("update role: %+v", g2)
	}

	// Now both are Owners; deleting one is allowed.
	if err := grants.Delete(ctx, col.CollectionID, g.GrantID); err != nil {
		t.Fatalf("delete one owner: %v", err)
	}

	// Deleting the LAST owner must fail.
	if err := grants.Delete(ctx, col.CollectionID, ownerGrantID); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("delete last owner: got %v want ErrConflict", err)
	}

	// ResolveUserRole returns the active role for owner.
	role, err := grants.ResolveUserRole(ctx, col.CollectionID, owner.UserID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if role != 4 {
		t.Fatalf("resolve: got %d want 4", role)
	}
	// Missing grant → 0, nil.
	zero, err := grants.ResolveUserRole(ctx, col.CollectionID, other.UserID)
	if err != nil {
		t.Fatalf("resolve missing: %v", err)
	}
	if zero != 0 {
		t.Fatalf("resolve missing: got %d want 0", zero)
	}
}
