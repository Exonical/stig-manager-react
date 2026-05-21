package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Label mirrors the upstream Label schema (labelId, name, color,
// description, uses).
type Label struct {
	LabelID      string
	CollectionID int64
	Name         string
	Color        string
	Description  string
	Uses         int
}

// LabelCreate is the input bundle for LabelRepo.Create.
type LabelCreate struct {
	CollectionID int64
	Name         string
	Color        string
	Description  string
}

// LabelUpdate is a partial-update bundle: every non-nil field is
// written. nil pointers are left untouched.
type LabelUpdate struct {
	Name        *string
	Color       *string
	Description *string
}

// LabelRepo provides CRUD for collection labels.
type LabelRepo struct {
	pool *pgxpool.Pool
}

// NewLabelRepo returns a LabelRepo backed by pool.
func NewLabelRepo(pool *pgxpool.Pool) *LabelRepo {
	return &LabelRepo{pool: pool}
}

// Create inserts a new label.
func (r *LabelRepo) Create(ctx context.Context, in LabelCreate) (Label, error) {
	if in.CollectionID == 0 {
		return Label{}, errors.New("store: label: collection id required")
	}
	if strings.TrimSpace(in.Name) == "" {
		return Label{}, errors.New("store: label: name required")
	}
	if strings.TrimSpace(in.Color) == "" {
		return Label{}, errors.New("store: label: color required")
	}
	const q = `
INSERT INTO collection_label (collection_id, name, color, description)
VALUES ($1, $2, $3, NULLIF($4, ''))
RETURNING label_id::text, collection_id, name, color, COALESCE(description, '')
`
	var out Label
	if err := r.pool.QueryRow(ctx, q, in.CollectionID, in.Name, in.Color, in.Description).Scan(
		&out.LabelID, &out.CollectionID, &out.Name, &out.Color, &out.Description,
	); err != nil {
		return Label{}, fmt.Errorf("insert label: %w", classifyLabelErr(err))
	}
	return out, nil
}

// Get returns a single label by id, scoped to collectionID.
func (r *LabelRepo) Get(ctx context.Context, collectionID int64, labelID string) (Label, error) {
	const q = `
SELECT cl.label_id::text, cl.collection_id, cl.name, cl.color, COALESCE(cl.description, ''),
       (SELECT count(*) FROM collection_label_asset WHERE label_id = cl.label_id) AS uses
FROM collection_label cl
WHERE cl.label_id = $1::uuid AND cl.collection_id = $2
`
	var out Label
	if err := r.pool.QueryRow(ctx, q, labelID, collectionID).Scan(
		&out.LabelID, &out.CollectionID, &out.Name, &out.Color, &out.Description, &out.Uses,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Label{}, ErrNotFound
		}
		return Label{}, fmt.Errorf("get label: %w", err)
	}
	return out, nil
}

// List returns every label in the supplied collection, ordered by
// case-insensitive name ascending.
func (r *LabelRepo) List(ctx context.Context, collectionID int64) ([]Label, error) {
	const q = `
SELECT cl.label_id::text, cl.collection_id, cl.name, cl.color, COALESCE(cl.description, ''),
       (SELECT count(*) FROM collection_label_asset WHERE label_id = cl.label_id) AS uses
FROM collection_label cl
WHERE cl.collection_id = $1
ORDER BY lower(cl.name) ASC
`
	rows, err := r.pool.Query(ctx, q, collectionID)
	if err != nil {
		return nil, fmt.Errorf("list labels: %w", err)
	}
	defer rows.Close()
	var out []Label
	for rows.Next() {
		var l Label
		if err := rows.Scan(
			&l.LabelID, &l.CollectionID, &l.Name, &l.Color, &l.Description, &l.Uses,
		); err != nil {
			return nil, fmt.Errorf("scan label: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// Patch merges the supplied fields onto an existing label.
func (r *LabelRepo) Patch(ctx context.Context, collectionID int64, labelID string, in LabelUpdate) (Label, error) {
	var (
		sets []string
		args []any
	)
	if in.Name != nil {
		args = append(args, *in.Name)
		sets = append(sets, fmt.Sprintf("name = $%d", len(args)))
	}
	if in.Color != nil {
		args = append(args, *in.Color)
		sets = append(sets, fmt.Sprintf("color = $%d", len(args)))
	}
	if in.Description != nil {
		args = append(args, *in.Description)
		sets = append(sets, fmt.Sprintf("description = NULLIF($%d, '')", len(args)))
	}
	if len(sets) == 0 {
		return r.Get(ctx, collectionID, labelID)
	}
	args = append(args, labelID, collectionID)
	q := fmt.Sprintf(
		`UPDATE collection_label SET %s WHERE label_id = $%d::uuid AND collection_id = $%d`,
		strings.Join(sets, ", "), len(args)-1, len(args),
	)
	ct, err := r.pool.Exec(ctx, q, args...)
	if err != nil {
		return Label{}, fmt.Errorf("patch label: %w", classifyLabelErr(err))
	}
	if ct.RowsAffected() == 0 {
		return Label{}, ErrNotFound
	}
	return r.Get(ctx, collectionID, labelID)
}

// Delete removes a label and its asset mappings.
func (r *LabelRepo) Delete(ctx context.Context, collectionID int64, labelID string) error {
	ct, err := r.pool.Exec(ctx,
		`DELETE FROM collection_label WHERE label_id = $1::uuid AND collection_id = $2`,
		labelID, collectionID,
	)
	if err != nil {
		return fmt.Errorf("delete label: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AssetIDs lists the assets a label is currently applied to.
func (r *LabelRepo) AssetIDs(ctx context.Context, collectionID int64, labelID string) ([]int64, error) {
	const q = `
SELECT a.asset_id
FROM collection_label_asset cla
JOIN asset a ON a.asset_id = cla.asset_id
WHERE cla.label_id = $1::uuid AND a.collection_id = $2 AND a.state = 'enabled'
ORDER BY lower(a.name) ASC
`
	rows, err := r.pool.Query(ctx, q, labelID, collectionID)
	if err != nil {
		return nil, fmt.Errorf("query label assets: %w", err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan asset id: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// SetAssets replaces the assets mapped to labelID with the supplied
// list. Each asset must belong to collectionID; otherwise ErrConflict.
func (r *LabelRepo) SetAssets(ctx context.Context, collectionID int64, labelID string, assetIDs []int64) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Verify the label belongs to the collection.
	var owned bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM collection_label WHERE label_id = $1::uuid AND collection_id = $2)`,
		labelID, collectionID,
	).Scan(&owned); err != nil {
		return fmt.Errorf("verify label: %w", err)
	}
	if !owned {
		return ErrNotFound
	}

	if _, err := tx.Exec(ctx, `DELETE FROM collection_label_asset WHERE label_id = $1::uuid`, labelID); err != nil {
		return fmt.Errorf("clear label assets: %w", err)
	}
	for _, aid := range assetIDs {
		ct, err := tx.Exec(ctx, `
INSERT INTO collection_label_asset (label_id, asset_id)
SELECT $1::uuid, $2
WHERE EXISTS (SELECT 1 FROM asset WHERE asset_id = $2 AND collection_id = $3 AND state = 'enabled')
ON CONFLICT (label_id, asset_id) DO NOTHING
`, labelID, aid, collectionID)
		if err != nil {
			return fmt.Errorf("apply asset %d: %w", aid, err)
		}
		if ct.RowsAffected() == 0 {
			return fmt.Errorf("asset %d: %w", aid, ErrConflict)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// classifyLabelErr maps Postgres unique-violation errors on labels to
// ErrDuplicateName.
func classifyLabelErr(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "idx_collection_label_name") {
		return ErrDuplicateName
	}
	return err
}
