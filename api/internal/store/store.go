// Package store is the Postgres 18 data layer.
//
// Open() constructs a pgxpool; repository types (UserRepo,
// CollectionRepo, …) wrap it with operation-shaped methods that
// return domain structs (AppUser, Collection, …).
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool wraps a pgxpool.Pool.
type Pool = pgxpool.Pool

// ErrNotFound is returned by repository getters when the requested
// row does not exist.
var ErrNotFound = errors.New("store: not found")

// ErrDuplicateName is returned by CollectionRepo.Create when a
// collection with the same case-insensitive name is already enabled.
var ErrDuplicateName = errors.New("store: duplicate collection name")

// ErrConflict is returned when a write violates a uniqueness or
// referential-integrity constraint that is meaningful to the API
// client (e.g. an asset name already exists in the same collection,
// or a label is being applied to an asset in another collection).
var ErrConflict = errors.New("store: conflict")

// Open creates and pings a Postgres connection pool.
func Open(ctx context.Context, dsn string) (*Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse pgx config: %w", err)
	}
	cfg.MaxConns = 10
	cfg.MaxConnLifetime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("new pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}
