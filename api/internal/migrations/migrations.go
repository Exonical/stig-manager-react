// Package migrations bundles the schema as embedded SQL files driven by
// goose. The migrations are run by the API binary on start-up (unless
// disabled) and by tests against a Postgres testcontainers instance, so
// the same SQL is exercised end-to-end.
package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var FS embed.FS

const goosePostgresDialect = "postgres"

// Migrate runs all pending Up migrations against pool. The function is
// safe to call concurrently from multiple processes — goose holds an
// advisory lock for the duration of each migration.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	provider, db, err := newProvider(pool)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := provider.UpContext(ctx, db); err != nil {
		return fmt.Errorf("migrations up: %w", err)
	}
	return nil
}

// Down rolls back the most recently applied migration. Intended for
// dev workflows and the goose round-trip CI check; production rollbacks
// should use targeted SQL.
func Down(ctx context.Context, pool *pgxpool.Pool) error {
	provider, db, err := newProvider(pool)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := provider.DownContext(ctx, db); err != nil {
		return fmt.Errorf("migrations down: %w", err)
	}
	return nil
}

// Version returns the most-recently-applied migration version. Returns
// 0 when the goose tracking table is empty (fresh database).
func Version(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	provider, db, err := newProvider(pool)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	v, err := provider.GetDBVersionContext(ctx, db)
	if err != nil {
		return 0, fmt.Errorf("migrations version: %w", err)
	}
	return v, nil
}

// newProvider configures a goose provider that reads SQL from the
// embedded filesystem and runs migrations through a database/sql
// handle backed by pgx's stdlib driver. We open a fresh *sql.DB per
// call because goose owns the connection lifecycle during migration
// operations.
func newProvider(pool *pgxpool.Pool) (*gooseProvider, *sql.DB, error) {
	if pool == nil {
		return nil, nil, fmt.Errorf("migrations: pool is nil")
	}
	db := stdlib.OpenDBFromPool(pool)
	if err := goose.SetDialect(goosePostgresDialect); err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("set goose dialect: %w", err)
	}
	goose.SetBaseFS(FS)
	return &gooseProvider{}, db, nil
}

// gooseProvider isolates the goose package surface so callers can mock
// or replace the implementation without touching every call site.
type gooseProvider struct{}

func (gooseProvider) UpContext(ctx context.Context, db *sql.DB) error {
	return goose.UpContext(ctx, db, ".")
}

func (gooseProvider) DownContext(ctx context.Context, db *sql.DB) error {
	return goose.DownContext(ctx, db, ".")
}

func (gooseProvider) GetDBVersionContext(ctx context.Context, db *sql.DB) (int64, error) {
	return goose.GetDBVersionContext(ctx, db)
}
