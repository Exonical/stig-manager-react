//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Exonical/stig-manager-react/api/internal/store"
)

// seedReviewFixture creates a collection, an asset, and returns the
// helper repos needed by review tests.
func seedReviewFixture(t *testing.T) (*store.ReviewRepo, int64, int64, int64) {
	t.Helper()
	pool := newPGPool(t)
	ctx := context.Background()
	users := store.NewUserRepo(pool)
	cols := store.NewCollectionRepo(pool)
	assets := store.NewAssetRepo(pool)

	owner, err := users.Upsert(ctx, "reviewer-1", "reviewer", "Reviewer", "reviewer@example.com", nil)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	col, err := cols.Create(ctx, store.CollectionCreate{
		Name:   "Reviews-Coll",
		Grants: []store.CollectionCreateGrant{{UserID: owner.UserID, RoleID: 4}},
	})
	if err != nil {
		t.Fatalf("collection: %v", err)
	}
	asset, err := assets.Create(ctx, store.AssetCreate{
		CollectionID: col.CollectionID,
		Name:         "review-host",
	})
	if err != nil {
		t.Fatalf("asset: %v", err)
	}
	return store.NewReviewRepo(pool), owner.UserID, col.CollectionID, asset.AssetID
}

func TestReviewRepo_PutGetHistory(t *testing.T) {
	repo, userID, _, assetID := seedReviewFixture(t)
	ctx := context.Background()
	const ruleID = "SV-001r1_rule"

	// First Put — no prior row, history should still be empty.
	first, err := repo.Put(ctx, assetID, ruleID, userID, store.ReviewWrite{
		Result:      "pass",
		Comment:     "first",
		Detail:      "all clear",
		StatusLabel: "submitted",
		StatusText:  "ready",
		Metadata:    json.RawMessage(`{"source":"manual"}`),
	})
	if err != nil {
		t.Fatalf("put-1: %v", err)
	}
	if first.Result != "pass" || first.StatusLabel != "submitted" {
		t.Fatalf("put-1 mismatch: %+v", first)
	}
	hist, err := repo.History(ctx, assetID, ruleID)
	if err != nil {
		t.Fatalf("history-1: %v", err)
	}
	if len(hist) != 0 {
		t.Fatalf("history-1: want empty, got %d", len(hist))
	}

	// Second Put — overwrites and snapshots the previous row.
	second, err := repo.Put(ctx, assetID, ruleID, userID, store.ReviewWrite{
		Result:      "fail",
		Comment:     "second",
		Detail:      "needs work",
		StatusLabel: "saved",
	})
	if err != nil {
		t.Fatalf("put-2: %v", err)
	}
	if second.Result != "fail" || second.Comment != "second" {
		t.Fatalf("put-2 mismatch: %+v", second)
	}
	hist, err = repo.History(ctx, assetID, ruleID)
	if err != nil {
		t.Fatalf("history-2: %v", err)
	}
	if len(hist) != 1 {
		t.Fatalf("history-2: want 1, got %d (%+v)", len(hist), hist)
	}
	if hist[0].Comment != "first" || hist[0].Result != "pass" {
		t.Fatalf("history snapshot wrong: %+v", hist[0])
	}
}

func TestReviewRepo_PatchAndDelete(t *testing.T) {
	repo, userID, _, assetID := seedReviewFixture(t)
	ctx := context.Background()
	const ruleID = "SV-002r1_rule"

	// PATCH on missing review → ErrNotFound.
	commentPtr := "ignored"
	if _, err := repo.Patch(ctx, assetID, ruleID, userID, store.ReviewPatch{
		Comment: &commentPtr,
	}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("patch-missing: got %v want ErrNotFound", err)
	}

	// Seed via Put.
	if _, err := repo.Put(ctx, assetID, ruleID, userID, store.ReviewWrite{
		Result: "notchecked", Comment: "seed",
	}); err != nil {
		t.Fatalf("seed put: %v", err)
	}

	// PATCH result only — comment / detail untouched.
	res := "informational"
	patched, err := repo.Patch(ctx, assetID, ruleID, userID, store.ReviewPatch{
		Result: &res,
	})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if patched.Result != "informational" || patched.Comment != "seed" {
		t.Fatalf("patch merge wrong: %+v", patched)
	}

	// DELETE — leaves a final history entry, subsequent Get → ErrNotFound.
	if err := repo.Delete(ctx, assetID, ruleID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.Get(ctx, assetID, ruleID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("get-after-delete: got %v want ErrNotFound", err)
	}
	hist, err := repo.History(ctx, assetID, ruleID)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	// Two snapshots: one for the PATCH, one for the DELETE.
	if len(hist) < 2 {
		t.Fatalf("history: want >=2, got %d (%+v)", len(hist), hist)
	}

	// DELETE again → ErrNotFound.
	if err := repo.Delete(ctx, assetID, ruleID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("double-delete: got %v want ErrNotFound", err)
	}
}

func TestReviewRepo_ListFilters(t *testing.T) {
	repo, userID, collID, assetID := seedReviewFixture(t)
	ctx := context.Background()

	// Seed two reviews with different result + status.
	if _, err := repo.Put(ctx, assetID, "SV-A_rule", userID, store.ReviewWrite{
		Result: "pass", StatusLabel: "submitted",
	}); err != nil {
		t.Fatalf("seed A: %v", err)
	}
	if _, err := repo.Put(ctx, assetID, "SV-B_rule", userID, store.ReviewWrite{
		Result: "fail", StatusLabel: "saved",
	}); err != nil {
		t.Fatalf("seed B: %v", err)
	}

	// Full list.
	all, err := repo.List(ctx, store.ListReviewsOptions{CollectionID: collID})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("list: got %d, want 2", len(all))
	}

	// Filter by result.
	passing, err := repo.List(ctx, store.ListReviewsOptions{
		CollectionID: collID, Result: "pass",
	})
	if err != nil {
		t.Fatalf("list-result: %v", err)
	}
	if len(passing) != 1 || passing[0].RuleID != "SV-A_rule" {
		t.Fatalf("list-result wrong: %+v", passing)
	}

	// Filter by status.
	submitted, err := repo.List(ctx, store.ListReviewsOptions{
		CollectionID: collID, Status: "submitted",
	})
	if err != nil {
		t.Fatalf("list-status: %v", err)
	}
	if len(submitted) != 1 || submitted[0].RuleID != "SV-A_rule" {
		t.Fatalf("list-status wrong: %+v", submitted)
	}
}
