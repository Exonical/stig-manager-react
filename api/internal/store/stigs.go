package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Exonical/stig-manager-react/api/internal/xccdf"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// STIG is the projection returned by STIGRepo.List / Get.
type STIG struct {
	BenchmarkID     string
	Title           string
	LastRevisionStr string
	LastRevisionDate *time.Time
	RuleCount       int
	Status          string
	Marking         string
	RevisionStrs    []string
}

// CollectionSTIG mirrors the upstream CollectionStigWithAssetCount
// model: a STIG mapped (via at least one asset) into a Collection,
// together with the count of assets in that Collection using it.
type CollectionSTIG struct {
	BenchmarkID    string
	Title          string
	RevisionStr    string
	BenchmarkDate  *time.Time
	RuleCount      int
	AssetCount     int
	RevisionPinned bool
}

// Revision is a single imported version of a STIG.
type Revision struct {
	RevisionID    int64
	BenchmarkID   string
	RevisionStr   string
	Version       string
	Release       string
	BenchmarkDate *time.Time
	Status        string
	Description   string
	Marking       string
	RuleCount     int
	ImportedAt    time.Time
}

// RuleProjection is the projection returned by STIGRepo.GetRuleByRuleId.
type RuleProjection struct {
	RuleID        string
	VersionStr    string
	Severity      string
	Title         string
	Description   string
	CheckSystem   string
	CheckContent  string
	FixID         string
	FixText       string
	CCIs          []string
	BenchmarkID   string
	RevisionStr   string
}

// CCI is the projection returned by STIGRepo.GetCCI.
type CCI struct {
	CCI         string
	Definition  string
	Type        string
	Status      string
	PublishDate *time.Time
	Stigs       []RevisionRef
}

// RevisionRef is a tuple of (benchmarkId, revisionStr).
type RevisionRef struct {
	BenchmarkID string
	RevisionStr string
}

// STIGRepo provides CRUD operations on the STIG / revision / rule / CCI
// tables.
type STIGRepo struct {
	pool *pgxpool.Pool
}

// NewSTIGRepo constructs a STIGRepo bound to pool.
func NewSTIGRepo(pool *pgxpool.Pool) *STIGRepo {
	return &STIGRepo{pool: pool}
}

// ImportRevision writes a parsed XCCDF Benchmark into the database. When
// the (benchmark_id, revision_str) tuple already exists and clobber is
// false, ErrDuplicateName is returned. When clobber is true the existing
// revision and all its rules are deleted and replaced.
//
// Imports happen inside a single transaction so a partial failure leaves
// the database untouched.
func (r *STIGRepo) ImportRevision(ctx context.Context, b *xccdf.Benchmark, clobber bool) (*Revision, error) {
	if b == nil {
		return nil, errors.New("import: benchmark is nil")
	}
	if b.BenchmarkID == "" {
		return nil, errors.New("import: benchmark id is empty")
	}
	revStr := b.RevisionStr()

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("import begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO stig (benchmark_id, title) VALUES ($1, $2)
		ON CONFLICT (benchmark_id) DO UPDATE SET title = EXCLUDED.title
	`, b.BenchmarkID, b.Title); err != nil {
		return nil, fmt.Errorf("upsert stig: %w", err)
	}

	var existingRev int64
	err = tx.QueryRow(ctx, `
		SELECT revision_id FROM stig_revision
		WHERE benchmark_id = $1 AND revision_str = $2
	`, b.BenchmarkID, revStr).Scan(&existingRev)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// new revision
	case err != nil:
		return nil, fmt.Errorf("lookup existing revision: %w", err)
	default:
		if !clobber {
			return nil, ErrDuplicateName
		}
		if _, err := tx.Exec(ctx, `DELETE FROM stig_revision WHERE revision_id = $1`, existingRev); err != nil {
			return nil, fmt.Errorf("delete existing revision: %w", err)
		}
	}

	var revisionID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO stig_revision (
			benchmark_id, revision_str, version, release,
			release_date, status, status_date, benchmark_date,
			description, source, marking
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING revision_id
	`,
		b.BenchmarkID, revStr, b.Version, b.Release,
		nullDate(b.ReleaseDate), b.Status, nullDate(b.StatusDate), nullDate(b.BenchmarkDate),
		b.Description, b.Source, nullString(b.Marking),
	).Scan(&revisionID); err != nil {
		return nil, fmt.Errorf("insert revision: %w", err)
	}

	for _, rule := range b.Rules {
		var rulePK int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO stig_rule (
				revision_id, rule_id, version_str, group_id, group_title,
				severity, weight, title, description,
				check_system, check_content, fix_id, fix_text
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
			) RETURNING rule_pk
		`,
			revisionID, rule.RuleID, rule.VersionStr, rule.GroupID, rule.GroupTitle,
			rule.Severity, rule.Weight, rule.Title, rule.Description,
			rule.CheckSystem, rule.CheckContent, rule.FixID, rule.FixText,
		).Scan(&rulePK); err != nil {
			return nil, fmt.Errorf("insert rule %s: %w", rule.RuleID, err)
		}

		for _, cci := range rule.CCIs {
			if _, err := tx.Exec(ctx, `
				INSERT INTO cci (cci, definition) VALUES ($1, '')
				ON CONFLICT (cci) DO NOTHING
			`, cci); err != nil {
				return nil, fmt.Errorf("upsert cci %s: %w", cci, err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO stig_rule_cci (rule_pk, cci) VALUES ($1, $2)
				ON CONFLICT DO NOTHING
			`, rulePK, cci); err != nil {
				return nil, fmt.Errorf("link rule %s -> %s: %w", rule.RuleID, cci, err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("import commit: %w", err)
	}

	return &Revision{
		RevisionID:    revisionID,
		BenchmarkID:   b.BenchmarkID,
		RevisionStr:   revStr,
		Version:       b.Version,
		Release:       b.Release,
		BenchmarkDate: timePtr(b.BenchmarkDate),
		Status:        b.Status,
		Description:   b.Description,
		Marking:       b.Marking,
		RuleCount:     len(b.Rules),
	}, nil
}

// ListSTIGsOptions filters the List query.
type ListSTIGsOptions struct {
	// TitleContains is matched case-insensitively against stig.title.
	TitleContains string
}

// List returns STIG summaries with their latest revision metadata. The
// "latest" revision is the one with the largest revision_id (i.e. most
// recently imported), matching upstream behaviour.
func (r *STIGRepo) List(ctx context.Context, opt ListSTIGsOptions) ([]STIG, error) {
	const q = `
		WITH latest AS (
			SELECT DISTINCT ON (benchmark_id)
				revision_id, benchmark_id, revision_str, benchmark_date, status, marking
			FROM stig_revision
			ORDER BY benchmark_id, revision_id DESC
		)
		SELECT s.benchmark_id,
		       s.title,
		       COALESCE(l.revision_str, ''),
		       l.benchmark_date,
		       COALESCE(l.status, ''),
		       COALESCE(l.marking, ''),
		       COALESCE((SELECT COUNT(*) FROM stig_rule sr WHERE sr.revision_id = l.revision_id), 0),
		       COALESCE(
		           (SELECT array_agg(rv.revision_str ORDER BY rv.revision_id DESC)
		              FROM stig_revision rv WHERE rv.benchmark_id = s.benchmark_id),
		           '{}'
		       )
		FROM stig s
		LEFT JOIN latest l ON l.benchmark_id = s.benchmark_id
		WHERE ($1::text = '' OR s.title ILIKE '%' || $1 || '%')
		ORDER BY s.benchmark_id`
	rows, err := r.pool.Query(ctx, q, opt.TitleContains)
	if err != nil {
		return nil, fmt.Errorf("list stigs: %w", err)
	}
	defer rows.Close()

	out := make([]STIG, 0)
	for rows.Next() {
		var s STIG
		var date *time.Time
		if err := rows.Scan(
			&s.BenchmarkID, &s.Title, &s.LastRevisionStr, &date,
			&s.Status, &s.Marking, &s.RuleCount, &s.RevisionStrs,
		); err != nil {
			return nil, fmt.Errorf("list scan: %w", err)
		}
		s.LastRevisionDate = date
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list rows: %w", err)
	}
	return out, nil
}

// Get returns a single STIG by benchmarkID with revision summary.
func (r *STIGRepo) Get(ctx context.Context, benchmarkID string) (*STIG, error) {
	const q = `
		WITH latest AS (
			SELECT revision_id, benchmark_id, revision_str, benchmark_date, status, marking
			FROM stig_revision
			WHERE benchmark_id = $1
			ORDER BY revision_id DESC
			LIMIT 1
		)
		SELECT s.benchmark_id,
		       s.title,
		       COALESCE(l.revision_str, ''),
		       l.benchmark_date,
		       COALESCE(l.status, ''),
		       COALESCE(l.marking, ''),
		       COALESCE((SELECT COUNT(*) FROM stig_rule sr WHERE sr.revision_id = l.revision_id), 0),
		       COALESCE(
		           (SELECT array_agg(rv.revision_str ORDER BY rv.revision_id DESC)
		              FROM stig_revision rv WHERE rv.benchmark_id = s.benchmark_id),
		           '{}'
		       )
		FROM stig s
		LEFT JOIN latest l ON l.benchmark_id = s.benchmark_id
		WHERE s.benchmark_id = $1`
	var s STIG
	var date *time.Time
	err := r.pool.QueryRow(ctx, q, benchmarkID).Scan(
		&s.BenchmarkID, &s.Title, &s.LastRevisionStr, &date,
		&s.Status, &s.Marking, &s.RuleCount, &s.RevisionStrs,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get stig: %w", err)
	}
	s.LastRevisionDate = date
	return &s, nil
}

// ListByCollection returns each STIG mapped (via at least one asset)
// into the given collection, together with the count of assets using
// it. The revisionStr surfaced is the latest imported revision for
// that benchmark, matching List's "latest" semantics.
func (r *STIGRepo) ListByCollection(ctx context.Context, collectionID int64) ([]CollectionSTIG, error) {
	const q = `
		WITH latest AS (
			SELECT DISTINCT ON (benchmark_id)
				revision_id, benchmark_id, revision_str, benchmark_date
			FROM stig_revision
			ORDER BY benchmark_id, revision_id DESC
		)
		SELECT s.benchmark_id,
		       s.title,
		       COALESCE(l.revision_str, ''),
		       l.benchmark_date,
		       COALESCE((SELECT COUNT(*) FROM stig_rule sr WHERE sr.revision_id = l.revision_id), 0)::int AS rule_count,
		       COUNT(DISTINCT a.asset_id)::int AS asset_count
		FROM stig s
		JOIN asset_stig as_ ON as_.benchmark_id = s.benchmark_id
		JOIN asset a ON a.asset_id = as_.asset_id
		LEFT JOIN latest l ON l.benchmark_id = s.benchmark_id
		WHERE a.collection_id = $1
		GROUP BY s.benchmark_id, s.title, l.revision_str, l.benchmark_date, l.revision_id
		ORDER BY s.benchmark_id`
	rows, err := r.pool.Query(ctx, q, collectionID)
	if err != nil {
		return nil, fmt.Errorf("list stigs by collection: %w", err)
	}
	defer rows.Close()
	out := make([]CollectionSTIG, 0)
	for rows.Next() {
		var c CollectionSTIG
		var date *time.Time
		if err := rows.Scan(&c.BenchmarkID, &c.Title, &c.RevisionStr, &date, &c.RuleCount, &c.AssetCount); err != nil {
			return nil, fmt.Errorf("scan collection stig: %w", err)
		}
		c.BenchmarkDate = date
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows collection stigs: %w", err)
	}
	return out, nil
}

// GetByCollection returns the CollectionSTIG row for a single
// benchmark mapped into the collection. Returns ErrNotFound when no
// asset in the collection references the benchmark.
func (r *STIGRepo) GetByCollection(ctx context.Context, collectionID int64, benchmarkID string) (*CollectionSTIG, error) {
	const q = `
		WITH latest AS (
			SELECT revision_id, benchmark_id, revision_str, benchmark_date
			FROM stig_revision
			WHERE benchmark_id = $2
			ORDER BY revision_id DESC
			LIMIT 1
		)
		SELECT s.benchmark_id,
		       s.title,
		       COALESCE(l.revision_str, ''),
		       l.benchmark_date,
		       COALESCE((SELECT COUNT(*) FROM stig_rule sr WHERE sr.revision_id = l.revision_id), 0)::int AS rule_count,
		       COUNT(DISTINCT a.asset_id)::int AS asset_count
		FROM stig s
		JOIN asset_stig as_ ON as_.benchmark_id = s.benchmark_id
		JOIN asset a ON a.asset_id = as_.asset_id
		LEFT JOIN latest l ON l.benchmark_id = s.benchmark_id
		WHERE a.collection_id = $1 AND s.benchmark_id = $2
		GROUP BY s.benchmark_id, s.title, l.revision_str, l.benchmark_date, l.revision_id`
	var c CollectionSTIG
	var date *time.Time
	err := r.pool.QueryRow(ctx, q, collectionID, benchmarkID).Scan(
		&c.BenchmarkID, &c.Title, &c.RevisionStr, &date, &c.RuleCount, &c.AssetCount,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get collection stig: %w", err)
	}
	c.BenchmarkDate = date
	return &c, nil
}

// GetRuleByRuleID returns the most-recent imported version of a rule by
// its ruleId. Resolution prefers the largest revision_id when the same
// ruleId exists across multiple revisions.
func (r *STIGRepo) GetRuleByRuleID(ctx context.Context, ruleID string) (*RuleProjection, error) {
	const q = `
		SELECT sr.rule_pk, sr.rule_id, sr.version_str, sr.severity, sr.title,
		       sr.description, sr.check_system, sr.check_content, sr.fix_id, sr.fix_text,
		       rv.benchmark_id, rv.revision_str
		FROM stig_rule sr
		JOIN stig_revision rv ON rv.revision_id = sr.revision_id
		WHERE sr.rule_id = $1
		ORDER BY sr.revision_id DESC, sr.rule_pk DESC
		LIMIT 1`
	var (
		rulePK int64
		out    RuleProjection
	)
	err := r.pool.QueryRow(ctx, q, ruleID).Scan(
		&rulePK, &out.RuleID, &out.VersionStr, &out.Severity, &out.Title,
		&out.Description, &out.CheckSystem, &out.CheckContent, &out.FixID, &out.FixText,
		&out.BenchmarkID, &out.RevisionStr,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get rule: %w", err)
	}

	cciRows, err := r.pool.Query(ctx, `
		SELECT cci FROM stig_rule_cci WHERE rule_pk = $1 ORDER BY cci
	`, rulePK)
	if err != nil {
		return nil, fmt.Errorf("get rule ccis: %w", err)
	}
	defer cciRows.Close()
	for cciRows.Next() {
		var cci string
		if err := cciRows.Scan(&cci); err != nil {
			return nil, fmt.Errorf("scan cci: %w", err)
		}
		out.CCIs = append(out.CCIs, cci)
	}
	if err := cciRows.Err(); err != nil {
		return nil, fmt.Errorf("rows cci: %w", err)
	}
	return &out, nil
}

// GetCCI returns a single CCI projection along with the STIGs that
// reference it. Returns ErrNotFound when no rule references the CCI
// and the CCI row itself is absent.
func (r *STIGRepo) GetCCI(ctx context.Context, cci string) (*CCI, error) {
	const q = `
		SELECT cci, definition, type, status, publish_date
		FROM cci WHERE cci = $1`
	var out CCI
	err := r.pool.QueryRow(ctx, q, cci).Scan(
		&out.CCI, &out.Definition, &out.Type, &out.Status, &out.PublishDate,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get cci: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT rv.benchmark_id, rv.revision_str
		FROM stig_rule_cci src
		JOIN stig_rule sr ON sr.rule_pk = src.rule_pk
		JOIN stig_revision rv ON rv.revision_id = sr.revision_id
		WHERE src.cci = $1
		ORDER BY rv.benchmark_id, rv.revision_str
	`, cci)
	if err != nil {
		return nil, fmt.Errorf("get cci stigs: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var ref RevisionRef
		if err := rows.Scan(&ref.BenchmarkID, &ref.RevisionStr); err != nil {
			return nil, fmt.Errorf("scan cci stigs: %w", err)
		}
		out.Stigs = append(out.Stigs, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows cci stigs: %w", err)
	}
	return &out, nil
}

// helpers

func nullDate(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
