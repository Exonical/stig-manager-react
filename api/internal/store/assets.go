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

// Asset mirrors the upstream Asset schema: a host/device tracked in a
// single Collection. The free-form `Metadata` blob is preserved as
// raw JSON so it roundtrips the OpenAPI Metadata type.
type Asset struct {
	AssetID      int64
	CollectionID int64
	Name         string
	FQDN         string
	IP           string
	MAC          string
	Description  string
	Noncomputing bool
	Metadata     json.RawMessage
	State        string
	CreatedAt    time.Time
}

// AssetStig records that a benchmark is assigned to an Asset, optionally
// pinned to a specific revision.
type AssetStig struct {
	BenchmarkID  string
	RevisionStr  string // empty means "default" (latest available)
	RevisionDate *time.Time
	RuleCount    int
}

// AssetCreate is the input bundle for AssetRepo.Create. CollectionID is
// required. Stigs and LabelIDs may be empty.
type AssetCreate struct {
	CollectionID int64
	Name         string
	FQDN         string
	IP           string
	MAC          string
	Description  string
	Noncomputing bool
	Metadata     json.RawMessage
	BenchmarkIDs []string
	LabelIDs     []string
}

// AssetUpdate is a partial-update bundle: every pointer field that is
// non-nil is written. Nil pointers are left untouched.
type AssetUpdate struct {
	Name         *string
	FQDN         *string
	IP           *string
	MAC          *string
	Description  *string
	Noncomputing *bool
	Metadata     json.RawMessage
	BenchmarkIDs *[]string
	LabelIDs     *[]string
	CollectionID *int64
}

// ListAssetsOptions narrows an Asset listing. Empty fields mean no
// filter.
type ListAssetsOptions struct {
	CollectionID int64
	NameContains string
	BenchmarkID  string
	LabelID      string
}

// AssetRepo provides CRUD for assets and their per-asset STIG / label
// mappings.
type AssetRepo struct {
	pool *pgxpool.Pool
}

// NewAssetRepo returns an AssetRepo backed by pool.
func NewAssetRepo(pool *pgxpool.Pool) *AssetRepo {
	return &AssetRepo{pool: pool}
}

// Create inserts an Asset and its initial STIG / label mappings inside
// a single transaction. Returns the created row plus its mappings.
func (r *AssetRepo) Create(ctx context.Context, in AssetCreate) (Asset, error) {
	if in.CollectionID == 0 {
		return Asset{}, errors.New("store: asset: collection id required")
	}
	if strings.TrimSpace(in.Name) == "" {
		return Asset{}, errors.New("store: asset: name required")
	}
	metadata := in.Metadata
	if len(metadata) == 0 {
		metadata = []byte(`{}`)
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Asset{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const insertAsset = `
INSERT INTO asset (collection_id, name, fqdn, ip, mac, description, noncomputing, metadata)
VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), $7, $8::jsonb)
RETURNING asset_id, collection_id, name,
          COALESCE(fqdn,''), COALESCE(ip,''), COALESCE(mac,''),
          COALESCE(description,''), noncomputing, metadata, state, created_at
`
	var out Asset
	if err := tx.QueryRow(ctx, insertAsset,
		in.CollectionID, in.Name, in.FQDN, in.IP, in.MAC, in.Description, in.Noncomputing, metadata,
	).Scan(
		&out.AssetID, &out.CollectionID, &out.Name,
		&out.FQDN, &out.IP, &out.MAC,
		&out.Description, &out.Noncomputing, &out.Metadata, &out.State, &out.CreatedAt,
	); err != nil {
		return Asset{}, fmt.Errorf("insert asset: %w", classifyAssetErr(err))
	}

	if err := upsertAssetStigs(ctx, tx, out.AssetID, in.BenchmarkIDs); err != nil {
		return Asset{}, err
	}
	if err := setAssetLabels(ctx, tx, out.AssetID, out.CollectionID, in.LabelIDs); err != nil {
		return Asset{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Asset{}, fmt.Errorf("commit: %w", err)
	}
	return out, nil
}

// Get returns a single Asset by id. Returns ErrNotFound when missing.
func (r *AssetRepo) Get(ctx context.Context, assetID int64) (Asset, error) {
	const q = `
SELECT asset_id, collection_id, name,
       COALESCE(fqdn,''), COALESCE(ip,''), COALESCE(mac,''),
       COALESCE(description,''), noncomputing, metadata, state, created_at
FROM asset
WHERE asset_id = $1 AND state = 'enabled'
`
	var out Asset
	if err := r.pool.QueryRow(ctx, q, assetID).Scan(
		&out.AssetID, &out.CollectionID, &out.Name,
		&out.FQDN, &out.IP, &out.MAC,
		&out.Description, &out.Noncomputing, &out.Metadata, &out.State, &out.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Asset{}, ErrNotFound
		}
		return Asset{}, fmt.Errorf("get asset: %w", err)
	}
	return out, nil
}

// List returns enabled assets matching the provided filters. Ordered by
// case-insensitive name ascending, mirroring upstream's default.
func (r *AssetRepo) List(ctx context.Context, opt ListAssetsOptions) ([]Asset, error) {
	var (
		joins      []string
		conditions = []string{"a.state = 'enabled'"}
		args       []any
	)
	if opt.CollectionID > 0 {
		args = append(args, opt.CollectionID)
		conditions = append(conditions, fmt.Sprintf("a.collection_id = $%d", len(args)))
	}
	if opt.NameContains != "" {
		args = append(args, "%"+strings.ToLower(opt.NameContains)+"%")
		conditions = append(conditions, fmt.Sprintf("lower(a.name) LIKE $%d", len(args)))
	}
	if opt.BenchmarkID != "" {
		joins = append(joins, "JOIN asset_stig as_ ON as_.asset_id = a.asset_id")
		args = append(args, opt.BenchmarkID)
		conditions = append(conditions, fmt.Sprintf("as_.benchmark_id = $%d", len(args)))
	}
	if opt.LabelID != "" {
		joins = append(joins, "JOIN collection_label_asset cla ON cla.asset_id = a.asset_id")
		args = append(args, opt.LabelID)
		conditions = append(conditions, fmt.Sprintf("cla.label_id = $%d", len(args)))
	}

	var b strings.Builder
	b.WriteString(`SELECT a.asset_id, a.collection_id, a.name,
       COALESCE(a.fqdn,''), COALESCE(a.ip,''), COALESCE(a.mac,''),
       COALESCE(a.description,''), a.noncomputing, a.metadata, a.state, a.created_at
FROM asset a
`)
	for _, j := range joins {
		b.WriteString(j)
		b.WriteByte('\n')
	}
	if len(conditions) > 0 {
		b.WriteString("WHERE ")
		b.WriteString(strings.Join(conditions, " AND "))
		b.WriteByte('\n')
	}
	b.WriteString("ORDER BY lower(a.name) ASC")

	rows, err := r.pool.Query(ctx, b.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("list assets: %w", err)
	}
	defer rows.Close()

	var out []Asset
	for rows.Next() {
		var a Asset
		if err := rows.Scan(
			&a.AssetID, &a.CollectionID, &a.Name,
			&a.FQDN, &a.IP, &a.MAC,
			&a.Description, &a.Noncomputing, &a.Metadata, &a.State, &a.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan asset: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Update merges the provided fields onto an existing asset. STIG and
// label mappings, when non-nil, fully replace the existing set
// (matching upstream's PATCH semantics for these arrays).
func (r *AssetRepo) Update(ctx context.Context, assetID int64, in AssetUpdate) (Asset, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Asset{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Build an UPDATE that only touches fields the caller supplied.
	var (
		sets []string
		args []any
	)
	if in.Name != nil {
		args = append(args, *in.Name)
		sets = append(sets, fmt.Sprintf("name = $%d", len(args)))
	}
	if in.FQDN != nil {
		args = append(args, *in.FQDN)
		sets = append(sets, fmt.Sprintf("fqdn = NULLIF($%d, '')", len(args)))
	}
	if in.IP != nil {
		args = append(args, *in.IP)
		sets = append(sets, fmt.Sprintf("ip = NULLIF($%d, '')", len(args)))
	}
	if in.MAC != nil {
		args = append(args, *in.MAC)
		sets = append(sets, fmt.Sprintf("mac = NULLIF($%d, '')", len(args)))
	}
	if in.Description != nil {
		args = append(args, *in.Description)
		sets = append(sets, fmt.Sprintf("description = NULLIF($%d, '')", len(args)))
	}
	if in.Noncomputing != nil {
		args = append(args, *in.Noncomputing)
		sets = append(sets, fmt.Sprintf("noncomputing = $%d", len(args)))
	}
	if len(in.Metadata) > 0 {
		args = append(args, []byte(in.Metadata))
		sets = append(sets, fmt.Sprintf("metadata = $%d::jsonb", len(args)))
	}
	if in.CollectionID != nil {
		args = append(args, *in.CollectionID)
		sets = append(sets, fmt.Sprintf("collection_id = $%d", len(args)))
	}

	if len(sets) > 0 {
		args = append(args, assetID)
		q := fmt.Sprintf(
			`UPDATE asset SET %s WHERE asset_id = $%d AND state = 'enabled'`,
			strings.Join(sets, ", "), len(args),
		)
		ct, err := tx.Exec(ctx, q, args...)
		if err != nil {
			return Asset{}, fmt.Errorf("update asset: %w", classifyAssetErr(err))
		}
		if ct.RowsAffected() == 0 {
			return Asset{}, ErrNotFound
		}
	}

	if in.BenchmarkIDs != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM asset_stig WHERE asset_id = $1`, assetID); err != nil {
			return Asset{}, fmt.Errorf("clear asset_stig: %w", err)
		}
		if err := upsertAssetStigs(ctx, tx, assetID, *in.BenchmarkIDs); err != nil {
			return Asset{}, err
		}
	}
	if in.LabelIDs != nil {
		// We need the asset's collection id to validate label membership.
		var collID int64
		if err := tx.QueryRow(ctx,
			`SELECT collection_id FROM asset WHERE asset_id = $1`, assetID,
		).Scan(&collID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Asset{}, ErrNotFound
			}
			return Asset{}, fmt.Errorf("lookup asset collection: %w", err)
		}
		if err := setAssetLabels(ctx, tx, assetID, collID, *in.LabelIDs); err != nil {
			return Asset{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Asset{}, fmt.Errorf("commit: %w", err)
	}
	return r.Get(ctx, assetID)
}

// Delete soft-deletes the asset by flipping state to 'disabled'.
// Returns ErrNotFound when no enabled row matches.
func (r *AssetRepo) Delete(ctx context.Context, assetID int64) error {
	ct, err := r.pool.Exec(ctx,
		`UPDATE asset SET state = 'disabled' WHERE asset_id = $1 AND state = 'enabled'`,
		assetID,
	)
	if err != nil {
		return fmt.Errorf("delete asset: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AssignedStigs returns the benchmarks currently mapped to the asset
// (with the default-revision projection used by GET
// /assets/{assetId}/stigs).
func (r *AssetRepo) AssignedStigs(ctx context.Context, assetID int64) ([]AssetStig, error) {
	const q = `
SELECT
    as_.benchmark_id,
    COALESCE(latest.revision_str, ''),
    latest.benchmark_date,
    COALESCE(latest.rule_count, 0)
FROM asset_stig as_
LEFT JOIN LATERAL (
    SELECT sr.revision_str, sr.benchmark_date,
           (SELECT count(*) FROM stig_rule WHERE revision_id = sr.revision_id) AS rule_count
    FROM stig_revision sr
    WHERE sr.benchmark_id = as_.benchmark_id
    ORDER BY sr.imported_at DESC, sr.revision_id DESC
    LIMIT 1
) latest ON true
WHERE as_.asset_id = $1
ORDER BY as_.benchmark_id ASC
`
	rows, err := r.pool.Query(ctx, q, assetID)
	if err != nil {
		return nil, fmt.Errorf("query asset stigs: %w", err)
	}
	defer rows.Close()
	var out []AssetStig
	for rows.Next() {
		var as AssetStig
		if err := rows.Scan(&as.BenchmarkID, &as.RevisionStr, &as.RevisionDate, &as.RuleCount); err != nil {
			return nil, fmt.Errorf("scan asset stig: %w", err)
		}
		out = append(out, as)
	}
	return out, rows.Err()
}

// AssignedLabelIDs returns the labels currently applied to assetID.
func (r *AssetRepo) AssignedLabelIDs(ctx context.Context, assetID int64) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT label_id::text FROM collection_label_asset WHERE asset_id = $1`, assetID)
	if err != nil {
		return nil, fmt.Errorf("query asset labels: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, fmt.Errorf("scan label_id: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// upsertAssetStigs writes a new set of STIG mappings for assetID. The
// caller is expected to have cleared the existing mappings if doing a
// replace; this helper is purely additive (idempotent via the PK).
func upsertAssetStigs(ctx context.Context, tx pgx.Tx, assetID int64, benchmarkIDs []string) error {
	if len(benchmarkIDs) == 0 {
		return nil
	}
	const q = `
INSERT INTO asset_stig (asset_id, benchmark_id)
VALUES ($1, $2)
ON CONFLICT (asset_id, benchmark_id) DO NOTHING
`
	for _, id := range benchmarkIDs {
		if _, err := tx.Exec(ctx, q, assetID, id); err != nil {
			// Foreign-key violations bubble up as conflicts so the API
			// can return a 400 referencing the unknown benchmark.
			return fmt.Errorf("insert asset_stig %q: %w", id, classifyAssetErr(err))
		}
	}
	return nil
}

// setAssetLabels replaces the asset's label set wholesale. Validates
// each label belongs to the asset's collection — applying a label from
// another collection returns ErrConflict.
func setAssetLabels(ctx context.Context, tx pgx.Tx, assetID, collectionID int64, labelIDs []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM collection_label_asset WHERE asset_id = $1`, assetID); err != nil {
		return fmt.Errorf("clear labels: %w", err)
	}
	if len(labelIDs) == 0 {
		return nil
	}
	const q = `
INSERT INTO collection_label_asset (label_id, asset_id)
SELECT $1::uuid, $2
WHERE EXISTS (
    SELECT 1 FROM collection_label
    WHERE label_id = $1::uuid AND collection_id = $3
)
`
	for _, id := range labelIDs {
		ct, err := tx.Exec(ctx, q, id, assetID, collectionID)
		if err != nil {
			return fmt.Errorf("apply label %q: %w", id, classifyAssetErr(err))
		}
		if ct.RowsAffected() == 0 {
			return fmt.Errorf("label %q: %w", id, ErrConflict)
		}
	}
	return nil
}

// classifyAssetErr maps Postgres errors that handlers want to surface
// distinctly (uniqueness, foreign-key) onto sentinel store errors.
func classifyAssetErr(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "idx_asset_name_per_collection"):
		return ErrDuplicateName
	case strings.Contains(msg, "violates foreign key constraint"):
		return ErrConflict
	}
	return err
}
