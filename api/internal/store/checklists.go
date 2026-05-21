package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ChecklistAsset is the asset projection used by checklist exports
// (CKL / CKLB / XCCDF). It mirrors the upstream Asset fields that
// surface in the file headers.
type ChecklistAsset struct {
	AssetID      int64
	CollectionID int64
	Name         string
	FQDN         string
	IP           string
	MAC          string
	Description  string
	Noncomputing bool
}

// ChecklistRule is one rule's content joined with its current review.
// All review fields are zero-valued when no review exists for the
// (asset, rule) pair. Severity uses the upstream low/medium/high
// vocabulary; unknown maps to the literal string "unknown".
type ChecklistRule struct {
	RuleID       string
	VersionStr   string
	GroupID      string
	GroupTitle   string
	Severity     string
	Weight       string
	Title        string
	Description  string
	CheckSystem  string
	CheckContent string
	FixID        string
	FixText      string
	CCIs         []string

	// Review fields. Result == "" means the rule has no review yet.
	Result      string
	Detail      string
	Comment     string
	AutoResult  bool
	StatusLabel string
	TS          *time.Time
	TouchTS     *time.Time
	Username    string
}

// ChecklistStig is the per-benchmark slice of a checklist: the
// revision metadata plus every rule in that revision (with review).
type ChecklistStig struct {
	BenchmarkID  string
	RevisionStr  string
	Version      string
	Release      string
	ReleaseDate  *time.Time
	Title        string
	Description  string
	Source       string
	Rules        []ChecklistRule
}

// AssetChecklist is the full export payload for a single asset. Stigs
// is sorted by BenchmarkID ASC; rules within each STIG are sorted by
// VersionStr (the upstream "Vuln_Num") ASC.
type AssetChecklist struct {
	Asset ChecklistAsset
	Stigs []ChecklistStig
}

// CollectionChecklistRow is one rule's roll-up across every asset in a
// collection that has the benchmark applied. Returned by
// ChecklistRepo.CollectionSummary.
type CollectionChecklistRow struct {
	GroupID    string
	GroupTitle string
	RuleID     string
	RuleTitle  string
	VersionStr string
	Severity   string

	PassCount          int
	FailCount          int
	NotApplicableCount int
	OtherCount         int

	SavedCount     int
	SubmittedCount int
	AcceptedCount  int
	RejectedCount  int

	MinTs      *time.Time
	MaxTs      *time.Time
	MinTouchTs *time.Time
	MaxTouchTs *time.Time
}

// ChecklistRepo assembles checklist export payloads from the database.
type ChecklistRepo struct {
	pool *pgxpool.Pool
}

// NewChecklistRepo constructs a ChecklistRepo bound to pool.
func NewChecklistRepo(pool *pgxpool.Pool) *ChecklistRepo {
	return &ChecklistRepo{pool: pool}
}

// resolveRevisionID returns the revision_id for (benchmarkID, revisionStr).
// When revisionStr is "" or "latest" the most recently imported
// revision is returned. Returns ErrNotFound when no matching revision
// exists.
func (r *ChecklistRepo) resolveRevisionID(ctx context.Context, benchmarkID, revisionStr string) (int64, string, error) {
	if revisionStr == "" || revisionStr == "latest" {
		const q = `
SELECT revision_id, revision_str
FROM stig_revision
WHERE benchmark_id = $1
ORDER BY imported_at DESC, revision_id DESC
LIMIT 1
`
		var id int64
		var rs string
		if err := r.pool.QueryRow(ctx, q, benchmarkID).Scan(&id, &rs); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return 0, "", ErrNotFound
			}
			return 0, "", fmt.Errorf("resolve latest revision: %w", err)
		}
		return id, rs, nil
	}
	const q = `
SELECT revision_id, revision_str
FROM stig_revision
WHERE benchmark_id = $1 AND revision_str = $2
`
	var id int64
	var rs string
	if err := r.pool.QueryRow(ctx, q, benchmarkID, revisionStr).Scan(&id, &rs); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, "", ErrNotFound
		}
		return 0, "", fmt.Errorf("resolve revision: %w", err)
	}
	return id, rs, nil
}

// loadAsset returns a single asset projection for checklist exports.
// Returns ErrNotFound when the asset doesn't exist or is disabled.
func (r *ChecklistRepo) loadAsset(ctx context.Context, assetID int64) (ChecklistAsset, error) {
	const q = `
SELECT asset_id, collection_id, name,
       COALESCE(fqdn,''), COALESCE(ip,''), COALESCE(mac,''),
       COALESCE(description,''), noncomputing
FROM asset
WHERE asset_id = $1 AND state = 'enabled'
`
	var out ChecklistAsset
	if err := r.pool.QueryRow(ctx, q, assetID).Scan(
		&out.AssetID, &out.CollectionID, &out.Name,
		&out.FQDN, &out.IP, &out.MAC,
		&out.Description, &out.Noncomputing,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ChecklistAsset{}, ErrNotFound
		}
		return ChecklistAsset{}, fmt.Errorf("load asset: %w", err)
	}
	return out, nil
}

// loadStig fetches the per-revision STIG slice including rules and
// review state for assetID. Returns ErrNotFound when revisionID is not
// mapped to benchmarkID or has no rules.
func (r *ChecklistRepo) loadStig(ctx context.Context, assetID, revisionID int64) (ChecklistStig, error) {
	const stigQ = `
SELECT
    sr.benchmark_id,
    sr.revision_str,
    sr.version,
    sr.release,
    sr.release_date,
    COALESCE(s.title,''),
    COALESCE(sr.description,''),
    COALESCE(sr.source,'')
FROM stig_revision sr
JOIN stig s ON s.benchmark_id = sr.benchmark_id
WHERE sr.revision_id = $1
`
	out := ChecklistStig{}
	if err := r.pool.QueryRow(ctx, stigQ, revisionID).Scan(
		&out.BenchmarkID, &out.RevisionStr, &out.Version, &out.Release,
		&out.ReleaseDate, &out.Title, &out.Description, &out.Source,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ChecklistStig{}, ErrNotFound
		}
		return ChecklistStig{}, fmt.Errorf("load stig revision: %w", err)
	}

	const rulesQ = `
SELECT
    sru.rule_id,
    COALESCE(sru.version_str,''),
    COALESCE(sru.group_id,''),
    COALESCE(sru.group_title,''),
    COALESCE(sru.severity,'unknown'),
    COALESCE(sru.weight,''),
    COALESCE(sru.title,''),
    COALESCE(sru.description,''),
    COALESCE(sru.check_system,''),
    COALESCE(sru.check_content,''),
    COALESCE(sru.fix_id,''),
    COALESCE(sru.fix_text,''),
    COALESCE((
        SELECT array_agg(cci ORDER BY cci)
        FROM stig_rule_cci src WHERE src.rule_pk = sru.rule_pk
    ), ARRAY[]::text[]) AS ccis,
    COALESCE(rv.result,''),
    COALESCE(rv.detail,''),
    COALESCE(rv.comment,''),
    COALESCE(rv.auto_result, false),
    COALESCE(rv.status_label,''),
    rv.ts,
    rv.touch_ts,
    COALESCE(usr.username,'')
FROM stig_rule sru
LEFT JOIN review rv
    ON rv.rule_id = sru.rule_id AND rv.asset_id = $2
LEFT JOIN app_user usr ON usr.user_id = rv.user_id
WHERE sru.revision_id = $1
ORDER BY sru.version_str ASC, sru.rule_id ASC
`
	rows, err := r.pool.Query(ctx, rulesQ, revisionID, assetID)
	if err != nil {
		return ChecklistStig{}, fmt.Errorf("query rules: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var rule ChecklistRule
		if err := rows.Scan(
			&rule.RuleID, &rule.VersionStr, &rule.GroupID, &rule.GroupTitle,
			&rule.Severity, &rule.Weight, &rule.Title, &rule.Description,
			&rule.CheckSystem, &rule.CheckContent, &rule.FixID, &rule.FixText,
			&rule.CCIs,
			&rule.Result, &rule.Detail, &rule.Comment, &rule.AutoResult,
			&rule.StatusLabel, &rule.TS, &rule.TouchTS, &rule.Username,
		); err != nil {
			return ChecklistStig{}, fmt.Errorf("scan rule: %w", err)
		}
		out.Rules = append(out.Rules, rule)
	}
	if err := rows.Err(); err != nil {
		return ChecklistStig{}, fmt.Errorf("rules iter: %w", err)
	}
	return out, nil
}

// resolveAssetStigRevision returns the effective revision_id for
// (assetID, benchmarkID) honouring the asset_stig.revision_id pin when
// present, otherwise falling back to the latest revision. revisionStr
// "" or "latest" applies the same fallback; an explicit revisionStr is
// resolved verbatim (and must be present in stig_revision).
func (r *ChecklistRepo) resolveAssetStigRevision(ctx context.Context, assetID int64, benchmarkID, revisionStr string) (int64, string, error) {
	if revisionStr != "" && revisionStr != "latest" {
		return r.resolveRevisionID(ctx, benchmarkID, revisionStr)
	}
	const q = `
SELECT as_.revision_id
FROM asset_stig as_
WHERE as_.asset_id = $1 AND as_.benchmark_id = $2
`
	var pinned *int64
	switch err := r.pool.QueryRow(ctx, q, assetID, benchmarkID).Scan(&pinned); {
	case errors.Is(err, pgx.ErrNoRows):
		return r.resolveRevisionID(ctx, benchmarkID, "")
	case err != nil:
		return 0, "", fmt.Errorf("resolve pin: %w", err)
	}
	if pinned != nil {
		const rs = `SELECT revision_str FROM stig_revision WHERE revision_id = $1`
		var rstr string
		if err := r.pool.QueryRow(ctx, rs, *pinned).Scan(&rstr); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return 0, "", ErrNotFound
			}
			return 0, "", fmt.Errorf("lookup pin rev_str: %w", err)
		}
		return *pinned, rstr, nil
	}
	return r.resolveRevisionID(ctx, benchmarkID, "")
}

// AssetSingle returns the export payload for a single (asset, benchmark, revisionStr).
// revisionStr may be the literal string "latest" or "" to mean "the
// asset's effective revision for this benchmark".
func (r *ChecklistRepo) AssetSingle(ctx context.Context, assetID int64, benchmarkID, revisionStr string) (AssetChecklist, error) {
	asset, err := r.loadAsset(ctx, assetID)
	if err != nil {
		return AssetChecklist{}, err
	}
	revID, _, err := r.resolveAssetStigRevision(ctx, assetID, benchmarkID, revisionStr)
	if err != nil {
		return AssetChecklist{}, err
	}
	stig, err := r.loadStig(ctx, assetID, revID)
	if err != nil {
		return AssetChecklist{}, err
	}
	return AssetChecklist{Asset: asset, Stigs: []ChecklistStig{stig}}, nil
}

// AssetMulti returns the export payload for every benchmark currently
// mapped to assetID. Each benchmark uses its asset_stig.revision_id
// pin (or latest when unpinned). When benchmarkIDs is non-empty only
// those benchmarks are exported.
func (r *ChecklistRepo) AssetMulti(ctx context.Context, assetID int64, benchmarkIDs []string) (AssetChecklist, error) {
	asset, err := r.loadAsset(ctx, assetID)
	if err != nil {
		return AssetChecklist{}, err
	}
	const q = `
SELECT as_.benchmark_id, as_.revision_id
FROM asset_stig as_
WHERE as_.asset_id = $1
ORDER BY as_.benchmark_id ASC
`
	rows, err := r.pool.Query(ctx, q, assetID)
	if err != nil {
		return AssetChecklist{}, fmt.Errorf("list asset_stig: %w", err)
	}
	defer rows.Close()

	type pair struct {
		Benchmark string
		Pinned    *int64
	}
	var pairs []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.Benchmark, &p.Pinned); err != nil {
			return AssetChecklist{}, fmt.Errorf("scan asset_stig: %w", err)
		}
		pairs = append(pairs, p)
	}
	if err := rows.Err(); err != nil {
		return AssetChecklist{}, fmt.Errorf("asset_stig iter: %w", err)
	}

	wanted := map[string]struct{}{}
	for _, b := range benchmarkIDs {
		wanted[b] = struct{}{}
	}

	out := AssetChecklist{Asset: asset}
	for _, p := range pairs {
		if len(wanted) > 0 {
			if _, ok := wanted[p.Benchmark]; !ok {
				continue
			}
		}
		var revID int64
		if p.Pinned != nil {
			revID = *p.Pinned
		} else {
			id, _, err := r.resolveRevisionID(ctx, p.Benchmark, "")
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					continue
				}
				return AssetChecklist{}, err
			}
			revID = id
		}
		stig, err := r.loadStig(ctx, assetID, revID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return AssetChecklist{}, err
		}
		out.Stigs = append(out.Stigs, stig)
	}
	return out, nil
}

// CollectionSummary returns one row per rule in (benchmark, revision)
// rolled up across every asset in the collection that has the
// benchmark applied.
func (r *ChecklistRepo) CollectionSummary(ctx context.Context, collectionID int64, benchmarkID, revisionStr string) ([]CollectionChecklistRow, error) {
	revID, _, err := r.resolveRevisionID(ctx, benchmarkID, revisionStr)
	if err != nil {
		return nil, err
	}
	const q = `
WITH targets AS (
    SELECT a.asset_id
    FROM asset a
    JOIN asset_stig as_ ON as_.asset_id = a.asset_id
    WHERE a.collection_id = $1 AND a.state = 'enabled' AND as_.benchmark_id = $2
)
SELECT
    sru.group_id,
    sru.group_title,
    sru.rule_id,
    sru.title,
    sru.version_str,
    sru.severity,
    COUNT(*) FILTER (WHERE rv.result = 'pass'),
    COUNT(*) FILTER (WHERE rv.result = 'fail'),
    COUNT(*) FILTER (WHERE rv.result = 'notapplicable'),
    COUNT(*) FILTER (WHERE rv.result IS NOT NULL AND rv.result NOT IN ('pass','fail','notapplicable')),
    COUNT(*) FILTER (WHERE rv.status_label = 'saved'),
    COUNT(*) FILTER (WHERE rv.status_label = 'submitted'),
    COUNT(*) FILTER (WHERE rv.status_label = 'accepted'),
    COUNT(*) FILTER (WHERE rv.status_label = 'rejected'),
    MIN(rv.ts),
    MAX(rv.ts),
    MIN(rv.touch_ts),
    MAX(rv.touch_ts)
FROM stig_rule sru
LEFT JOIN targets t ON true
LEFT JOIN review rv
    ON rv.rule_id = sru.rule_id AND rv.asset_id = t.asset_id
WHERE sru.revision_id = $3
GROUP BY sru.group_id, sru.group_title, sru.rule_id, sru.title, sru.version_str, sru.severity
ORDER BY sru.version_str ASC, sru.rule_id ASC
`
	rows, err := r.pool.Query(ctx, q, collectionID, benchmarkID, revID)
	if err != nil {
		return nil, fmt.Errorf("collection summary: %w", err)
	}
	defer rows.Close()

	var out []CollectionChecklistRow
	for rows.Next() {
		var row CollectionChecklistRow
		if err := rows.Scan(
			&row.GroupID, &row.GroupTitle, &row.RuleID, &row.RuleTitle,
			&row.VersionStr, &row.Severity,
			&row.PassCount, &row.FailCount, &row.NotApplicableCount, &row.OtherCount,
			&row.SavedCount, &row.SubmittedCount, &row.AcceptedCount, &row.RejectedCount,
			&row.MinTs, &row.MaxTs, &row.MinTouchTs, &row.MaxTouchTs,
		); err != nil {
			return nil, fmt.Errorf("scan summary row: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
