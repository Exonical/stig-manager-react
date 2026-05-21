package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Grant mirrors the upstream collection_grant projection for a single
// user-shaped grant. UserGroup-shaped grants land in a later milestone.
type Grant struct {
	GrantID      int64
	CollectionID int64
	UserID       int64
	Username     string
	DisplayName  string
	Email        string
	RoleID       int16
}

// GrantCreate is the input bundle for GrantRepo.Create (one user-shaped
// grant on collectionID).
type GrantCreate struct {
	CollectionID int64
	UserID       int64
	RoleID       int16
}

// GrantRepo provides CRUD for collection_grant rows.
type GrantRepo struct {
	pool *pgxpool.Pool
}

// NewGrantRepo returns a GrantRepo backed by pool.
func NewGrantRepo(pool *pgxpool.Pool) *GrantRepo {
	return &GrantRepo{pool: pool}
}

// List returns every grant on a collection ordered by username
// ascending.
func (r *GrantRepo) List(ctx context.Context, collectionID int64) ([]Grant, error) {
	const q = `
SELECT cg.grant_id, cg.collection_id, cg.user_id,
       au.username, au.display_name, au.email, cg.role_id
FROM collection_grant cg
JOIN app_user au ON au.user_id = cg.user_id
WHERE cg.collection_id = $1
ORDER BY lower(au.username) ASC
`
	rows, err := r.pool.Query(ctx, q, collectionID)
	if err != nil {
		return nil, fmt.Errorf("list grants: %w", err)
	}
	defer rows.Close()
	var out []Grant
	for rows.Next() {
		var g Grant
		if err := rows.Scan(
			&g.GrantID, &g.CollectionID, &g.UserID,
			&g.Username, &g.DisplayName, &g.Email, &g.RoleID,
		); err != nil {
			return nil, fmt.Errorf("scan grant: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// Get returns a single grant by id.
func (r *GrantRepo) Get(ctx context.Context, collectionID, grantID int64) (Grant, error) {
	const q = `
SELECT cg.grant_id, cg.collection_id, cg.user_id,
       au.username, au.display_name, au.email, cg.role_id
FROM collection_grant cg
JOIN app_user au ON au.user_id = cg.user_id
WHERE cg.grant_id = $1 AND cg.collection_id = $2
`
	var g Grant
	if err := r.pool.QueryRow(ctx, q, grantID, collectionID).Scan(
		&g.GrantID, &g.CollectionID, &g.UserID,
		&g.Username, &g.DisplayName, &g.Email, &g.RoleID,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Grant{}, ErrNotFound
		}
		return Grant{}, fmt.Errorf("get grant: %w", err)
	}
	return g, nil
}

// Create inserts a new grant. If (collection, user) already exists
// ErrConflict is returned.
func (r *GrantRepo) Create(ctx context.Context, in GrantCreate) (Grant, error) {
	if in.RoleID < 1 || in.RoleID > 4 {
		return Grant{}, errors.New("store: grant role must be 1..4")
	}
	const q = `
INSERT INTO collection_grant (collection_id, user_id, role_id)
VALUES ($1, $2, $3)
RETURNING grant_id
`
	var gid int64
	if err := r.pool.QueryRow(ctx, q, in.CollectionID, in.UserID, in.RoleID).Scan(&gid); err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return Grant{}, ErrConflict
		}
		if strings.Contains(err.Error(), "violates foreign key") {
			return Grant{}, ErrConflict
		}
		return Grant{}, fmt.Errorf("insert grant: %w", err)
	}
	return r.Get(ctx, in.CollectionID, gid)
}

// UpdateRole changes the role of an existing grant.
func (r *GrantRepo) UpdateRole(ctx context.Context, collectionID, grantID int64, roleID int16) (Grant, error) {
	if roleID < 1 || roleID > 4 {
		return Grant{}, errors.New("store: grant role must be 1..4")
	}
	ct, err := r.pool.Exec(ctx,
		`UPDATE collection_grant SET role_id = $1 WHERE grant_id = $2 AND collection_id = $3`,
		roleID, grantID, collectionID,
	)
	if err != nil {
		return Grant{}, fmt.Errorf("update grant: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return Grant{}, ErrNotFound
	}
	return r.Get(ctx, collectionID, grantID)
}

// Delete removes a grant. The last owner-role grant on a collection
// may not be deleted; doing so returns ErrConflict (matching upstream's
// guard).
func (r *GrantRepo) Delete(ctx context.Context, collectionID, grantID int64) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var role int16
	if err := tx.QueryRow(ctx,
		`SELECT role_id FROM collection_grant WHERE grant_id = $1 AND collection_id = $2`,
		grantID, collectionID,
	).Scan(&role); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("lookup grant: %w", err)
	}
	if role == 4 {
		var owners int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM collection_grant WHERE collection_id = $1 AND role_id = 4`,
			collectionID,
		).Scan(&owners); err != nil {
			return fmt.Errorf("count owners: %w", err)
		}
		if owners <= 1 {
			return ErrConflict
		}
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM collection_grant WHERE grant_id = $1 AND collection_id = $2`,
		grantID, collectionID,
	); err != nil {
		return fmt.Errorf("delete grant: %w", err)
	}
	return tx.Commit(ctx)
}

// ResolveUserRole returns the role_id the user holds on collectionID,
// or 0 when they have no grant. Used by the access-check middleware.
func (r *GrantRepo) ResolveUserRole(ctx context.Context, collectionID, userID int64) (int16, error) {
	const q = `
SELECT role_id FROM collection_grant
WHERE collection_id = $1 AND user_id = $2
`
	var role int16
	if err := r.pool.QueryRow(ctx, q, collectionID, userID).Scan(&role); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("resolve user role: %w", err)
	}
	return role, nil
}

// Ensure the *pgxpool.Pool import survives even when only used by other
// store files (lint cleanliness across the package).
var _ = pgxpool.Pool{}
