package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Collection mirrors the upstream Collection schema (collectionId,
// name, description, settings, metadata, created). Settings and
// Metadata are stored as raw JSON so they roundtrip the OpenAPI types
// without losing fields the database layer does not yet model.
type Collection struct {
	CollectionID int64
	Name         string
	Description  string
	Settings     json.RawMessage
	Metadata     json.RawMessage
	State        string
	CreatedAt    time.Time
}

// CollectionCreateGrant captures the per-grant subset upstream requires
// at create time: role_id (1..4) for an existing user.
type CollectionCreateGrant struct {
	UserID int64
	RoleID int16
}

// CollectionCreate is the input bundle for CollectionRepo.Create.
type CollectionCreate struct {
	Name        string
	Description string
	Settings    json.RawMessage
	Metadata    json.RawMessage
	Grants      []CollectionCreateGrant
}

// ListCollectionsOptions narrows a Collection list query. Empty means
// no filter.
type ListCollectionsOptions struct {
	NameContains string
	UserID       int64 // when > 0, restrict to collections the user has a grant on
	Limit        int32 // when > 0, LIMIT the result set
}

// CollectionRepo provides CRUD for collections. Like UserRepo it owns
// the pgxpool wrapper to keep the surface narrow.
type CollectionRepo struct {
	pool *pgxpool.Pool
}

// NewCollectionRepo returns a CollectionRepo backed by pool.
func NewCollectionRepo(pool *pgxpool.Pool) *CollectionRepo {
	return &CollectionRepo{pool: pool}
}

// Create inserts a Collection and its grants in a single transaction.
// Returns the created row's identity + timestamps. The collection is
// always enabled.
func (r *CollectionRepo) Create(ctx context.Context, in CollectionCreate) (Collection, error) {
	if in.Name == "" {
		return Collection{}, errors.New("store: collection name must not be empty")
	}
	if len(in.Grants) == 0 {
		return Collection{}, errors.New("store: at least one grant required")
	}

	settings := in.Settings
	if len(settings) == 0 {
		settings = []byte(`{}`)
	}
	metadata := in.Metadata
	if len(metadata) == 0 {
		metadata = []byte(`{}`)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Collection{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const insertColl = `
INSERT INTO collection (name, description, settings, metadata)
VALUES ($1, NULLIF($2, ''), $3::jsonb, $4::jsonb)
RETURNING collection_id, name, COALESCE(description, ''), settings, metadata, state, created_at
`
	var out Collection
	if err := tx.QueryRow(ctx, insertColl, in.Name, in.Description, settings, metadata).Scan(
		&out.CollectionID, &out.Name, &out.Description,
		&out.Settings, &out.Metadata, &out.State, &out.CreatedAt,
	); err != nil {
		return Collection{}, fmt.Errorf("insert collection: %w", classifyCreateErr(err))
	}

	const insertGrant = `
INSERT INTO collection_grant (collection_id, user_id, role_id)
VALUES ($1, $2, $3)
ON CONFLICT (collection_id, user_id) DO UPDATE SET role_id = EXCLUDED.role_id
`
	for _, g := range in.Grants {
		if _, err := tx.Exec(ctx, insertGrant, out.CollectionID, g.UserID, g.RoleID); err != nil {
			return Collection{}, fmt.Errorf("insert grant: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Collection{}, fmt.Errorf("commit: %w", err)
	}
	return out, nil
}

// Get returns the row identified by collectionID. Returns ErrNotFound
// when missing.
func (r *CollectionRepo) Get(ctx context.Context, collectionID int64) (Collection, error) {
	const q = `
SELECT collection_id, name, COALESCE(description, ''), settings, metadata, state, created_at
FROM collection
WHERE collection_id = $1
`
	var out Collection
	if err := r.pool.QueryRow(ctx, q, collectionID).Scan(
		&out.CollectionID, &out.Name, &out.Description,
		&out.Settings, &out.Metadata, &out.State, &out.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Collection{}, ErrNotFound
		}
		return Collection{}, fmt.Errorf("get collection: %w", err)
	}
	return out, nil
}

// List returns enabled collections, optionally filtered by name
// substring and by which user has a grant on the collection. Ordered
// by name (case-insensitive, ascending) — the same default as
// upstream.
func (r *CollectionRepo) List(ctx context.Context, opt ListCollectionsOptions) ([]Collection, error) {
	const base = `
SELECT c.collection_id, c.name, COALESCE(c.description, ''),
       c.settings, c.metadata, c.state, c.created_at
FROM collection c
`
	var (
		joins      []string
		conditions []string
		args       []any
	)
	conditions = append(conditions, "c.state = 'enabled'")
	if opt.UserID > 0 {
		joins = append(joins, "JOIN collection_grant cg ON cg.collection_id = c.collection_id")
		args = append(args, opt.UserID)
		conditions = append(conditions, fmt.Sprintf("cg.user_id = $%d", len(args)))
	}
	if opt.NameContains != "" {
		args = append(args, "%"+strings.ToLower(opt.NameContains)+"%")
		conditions = append(conditions, fmt.Sprintf("lower(c.name) LIKE $%d", len(args)))
	}

	var b strings.Builder
	b.WriteString(base)
	for _, j := range joins {
		b.WriteString(j)
		b.WriteByte('\n')
	}
	if len(conditions) > 0 {
		b.WriteString("WHERE ")
		b.WriteString(strings.Join(conditions, " AND "))
		b.WriteByte('\n')
	}
	b.WriteString("ORDER BY lower(c.name) ASC")
	if opt.Limit > 0 {
		args = append(args, opt.Limit)
		b.WriteString(fmt.Sprintf("\nLIMIT $%d", len(args)))
	}

	rows, err := r.pool.Query(ctx, b.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	defer rows.Close()

	var out []Collection
	for rows.Next() {
		var c Collection
		if err := rows.Scan(
			&c.CollectionID, &c.Name, &c.Description,
			&c.Settings, &c.Metadata, &c.State, &c.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan collection: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iter collections: %w", err)
	}
	return out, nil
}

// classifyCreateErr maps Postgres unique-violation errors to a stable
// sentinel so handlers can return 400 Duplicate Name without sniffing
// SQLSTATE strings themselves.
func classifyCreateErr(err error) error {
	if err == nil {
		return nil
	}
	// pgx surfaces pq-style unique violations as messages containing
	// "duplicate key value". Avoid coupling to pgconn here so tests
	// against a mock can verify the same path.
	if strings.Contains(err.Error(), "idx_collection_name_unique") {
		return ErrDuplicateName
	}
	return err
}
