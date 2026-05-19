//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	pgtc "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Exonical/stig-manager-react/api/internal/migrations"
	"github.com/Exonical/stig-manager-react/api/internal/store"
)

// newPGPool spins up a Postgres 18 testcontainer, applies the bundled
// goose migrations, and returns a pgxpool wired to the running
// instance. The container is torn down on test cleanup.
func newPGPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	container, err := pgtc.Run(ctx,
		"postgres:18",
		pgtc.WithDatabase("stigman"),
		pgtc.WithUsername("stigman"),
		pgtc.WithPassword("stigman"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(90*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() {
		_ = container.Terminate(ctx)
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	pool, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := migrations.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	return pool
}

// TestMigrate_UpDown_RoundTrips applies all migrations, rolls them
// back, and re-applies them. Verifies that every Down section is
// reversible and that Up + Down + Up is idempotent.
func TestMigrate_UpDown_RoundTrips(t *testing.T) {
	pool := newPGPool(t)
	ctx := context.Background()

	v0, err := migrations.Version(ctx, pool)
	if err != nil {
		t.Fatalf("initial version: %v", err)
	}
	if v0 == 0 {
		t.Fatalf("initial version: got 0 want > 0 (newPGPool ran migrations up)")
	}

	if err := migrations.Down(ctx, pool); err != nil {
		t.Fatalf("down: %v", err)
	}
	v1, err := migrations.Version(ctx, pool)
	if err != nil {
		t.Fatalf("post-down version: %v", err)
	}
	if v1 >= v0 {
		t.Fatalf("post-down version: got %d want < %d", v1, v0)
	}

	if err := migrations.Migrate(ctx, pool); err != nil {
		t.Fatalf("re-up: %v", err)
	}
	v2, err := migrations.Version(ctx, pool)
	if err != nil {
		t.Fatalf("post-reup version: %v", err)
	}
	if v2 != v0 {
		t.Fatalf("post-reup version: got %d want %d", v2, v0)
	}
}

// TestUserRepo_Upsert verifies that the first call inserts a row and
// the second call updates the cached display/username/claims while
// preserving the user_id.
func TestUserRepo_Upsert(t *testing.T) {
	pool := newPGPool(t)
	ctx := context.Background()
	repo := store.NewUserRepo(pool)

	first, err := repo.Upsert(ctx, "sub-1", "alice", "Alice A.", "alice@example.com", map[string]any{"k": "v"})
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if first.UserID == 0 {
		t.Fatalf("first.UserID: got 0")
	}
	if first.Username != "alice" || first.DisplayName != "Alice A." || first.Email != "alice@example.com" {
		t.Fatalf("first row not populated: %+v", first)
	}

	second, err := repo.Upsert(ctx, "sub-1", "alice2", "Alice B.", "alice2@example.com", map[string]any{"k": "v2"})
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if second.UserID != first.UserID {
		t.Fatalf("user_id changed across upserts: got %d want %d", second.UserID, first.UserID)
	}
	if second.Username != "alice2" || second.DisplayName != "Alice B." {
		t.Fatalf("second row not refreshed: %+v", second)
	}

	got, err := repo.GetBySubject(ctx, "sub-1")
	if err != nil {
		t.Fatalf("get by subject: %v", err)
	}
	if got.UserID != second.UserID {
		t.Fatalf("GetBySubject id mismatch: got %d want %d", got.UserID, second.UserID)
	}

	if _, err := repo.GetBySubject(ctx, "sub-missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing sub: got %v want ErrNotFound", err)
	}
}

// TestCollectionRepo_CreateGetList runs a small end-to-end scenario
// against the live schema: create a user, create two collections with
// grants, list them, and fetch one by id. It also exercises the
// hierarchical name-uniqueness check.
func TestCollectionRepo_CreateGetList(t *testing.T) {
	pool := newPGPool(t)
	ctx := context.Background()
	users := store.NewUserRepo(pool)
	collections := store.NewCollectionRepo(pool)

	owner, err := users.Upsert(ctx, "sub-owner", "owner", "Owner", "owner@example.com", nil)
	if err != nil {
		t.Fatalf("upsert owner: %v", err)
	}

	a, err := collections.Create(ctx, store.CollectionCreate{
		Name:        "Alpha",
		Description: "first",
		Grants:      []store.CollectionCreateGrant{{UserID: owner.UserID, RoleID: 4}},
	})
	if err != nil {
		t.Fatalf("create A: %v", err)
	}
	if a.CollectionID == 0 || a.State != "enabled" {
		t.Fatalf("create A: %+v", a)
	}
	var settings map[string]any
	if err := json.Unmarshal(a.Settings, &settings); err != nil {
		t.Fatalf("settings json: %v", err)
	}

	b, err := collections.Create(ctx, store.CollectionCreate{
		Name:        "Beta",
		Description: "second",
		Settings:    []byte(`{"x": 1}`),
		Grants:      []store.CollectionCreateGrant{{UserID: owner.UserID, RoleID: 4}},
	})
	if err != nil {
		t.Fatalf("create B: %v", err)
	}

	// Duplicate name (case-insensitive) → ErrDuplicateName.
	if _, err := collections.Create(ctx, store.CollectionCreate{
		Name:   "alpha",
		Grants: []store.CollectionCreateGrant{{UserID: owner.UserID, RoleID: 4}},
	}); !errors.Is(err, store.ErrDuplicateName) {
		t.Fatalf("duplicate: got %v want ErrDuplicateName", err)
	}

	got, err := collections.Get(ctx, a.CollectionID)
	if err != nil {
		t.Fatalf("get A: %v", err)
	}
	if got.Name != "Alpha" || got.Description != "first" {
		t.Fatalf("get A: %+v", got)
	}

	all, err := collections.List(ctx, store.ListCollectionsOptions{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("list: got %d want 2", len(all))
	}
	if !strings.EqualFold(all[0].Name, "Alpha") || !strings.EqualFold(all[1].Name, "Beta") {
		t.Fatalf("list order: got %q,%q want alpha,beta", all[0].Name, all[1].Name)
	}

	// Substring filter.
	filtered, err := collections.List(ctx, store.ListCollectionsOptions{NameContains: "et"})
	if err != nil {
		t.Fatalf("list filtered: %v", err)
	}
	if len(filtered) != 1 || filtered[0].CollectionID != b.CollectionID {
		t.Fatalf("filter: got %+v want only Beta", filtered)
	}

	// Grant-scoped list returns owner's collections; an unrelated
	// user should see nothing.
	other, err := users.Upsert(ctx, "sub-other", "other", "Other", "other@example.com", nil)
	if err != nil {
		t.Fatalf("upsert other: %v", err)
	}
	mine, err := collections.List(ctx, store.ListCollectionsOptions{UserID: owner.UserID})
	if err != nil {
		t.Fatalf("list grant: %v", err)
	}
	if len(mine) != 2 {
		t.Fatalf("grant list: got %d want 2", len(mine))
	}
	none, err := collections.List(ctx, store.ListCollectionsOptions{UserID: other.UserID})
	if err != nil {
		t.Fatalf("list grant other: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("grant list other: got %d want 0", len(none))
	}

	// Get on missing id → ErrNotFound.
	if _, err := collections.Get(ctx, 99999); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing get: got %v want ErrNotFound", err)
	}
}
