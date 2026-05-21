package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// decodeLabels parses the json_agg payload produced by the metrics
// queries into a flat MetricsLabelEntry slice. The query falls back
// to '[]'::json for assets with no labels, so a nil/empty slice is
// the natural unlabeled case.
func decodeLabels(raw []byte) []MetricsLabelEntry {
	if len(raw) == 0 {
		return []MetricsLabelEntry{}
	}
	var rows []struct {
		LabelID string `json:"label_id"`
		Name    string `json:"name"`
		Color   string `json:"color"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return []MetricsLabelEntry{}
	}
	out := make([]MetricsLabelEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, MetricsLabelEntry{LabelID: r.LabelID, Name: r.Name, Color: r.Color})
	}
	return out
}

// MetricsRepo aggregates compliance metrics across a Collection's
// asset × stig × rule space and joins them with each rule's current
// Review state (if any). The four agg shapes plus the unagg variant
// correspond to upstream's `/collections/{cid}/metrics/summary*`
// endpoints. CSV serialisation lives in the server layer; this repo
// returns typed rows.
//
// The query layer is built around a shared `rule_universe` CTE that
// emits exactly one row per "expected" rule (i.e. (asset, stig
// revision, rule) tuple where the rule belongs to the revision the
// asset has pinned or the benchmark's latest revision when the asset
// is on the default). A LEFT JOIN onto `review` decorates each row
// with the current evaluator decision; rows with no review contribute
// to `assessments` but not `assessed`.
type MetricsRepo struct {
	pool *pgxpool.Pool
}

// NewMetricsRepo wraps a pgxpool.Pool.
func NewMetricsRepo(pool *pgxpool.Pool) *MetricsRepo {
	return &MetricsRepo{pool: pool}
}

// MetricsFilter narrows the asset universe queried by every metrics
// endpoint. The four positive filters act as an OR within their
// category and AND across categories; LabelMatchNull restricts the
// asset universe to assets that have no labels applied at all (which
// is upstream's `labelMatch=null` idiom for surfacing "orphan"
// assets). All slices may be nil/empty for the unfiltered case.
type MetricsFilter struct {
	BenchmarkIDs   []string
	AssetIDs       []int64
	LabelIDs       []string
	LabelNames     []string
	LabelMatchNull bool
}

// metricsBindArgs converts a MetricsFilter into the positional bind
// list the universe SQL expects ($1=collectionID, $2=assetIDs,
// $3=labelIDs, $4=labelNamesLower, $5=labelMatchNull,
// $6=benchmarkIDs). Slices are normalised to empty (not nil) so pgx
// can bind them as Postgres arrays without driver complaints.
func (f MetricsFilter) bindArgs(collectionID int64) []any {
	assetIDs := append([]int64{}, f.AssetIDs...)
	labelIDs := append([]string{}, f.LabelIDs...)
	labelNames := make([]string, 0, len(f.LabelNames))
	for _, n := range f.LabelNames {
		labelNames = append(labelNames, strings.ToLower(n))
	}
	benchIDs := append([]string{}, f.BenchmarkIDs...)
	return []any{collectionID, assetIDs, labelIDs, labelNames, f.LabelMatchNull, benchIDs}
}

// universeCTE is the SQL fragment defining target_assets,
// asset_benchmark, effective_revisions, rule_universe, and joined.
// Embedded verbatim in every metrics query. Parameters are documented
// on bindArgs above.
const universeCTE = `
target_assets AS (
    SELECT a.asset_id, a.name, a.fqdn, a.ip, a.mac, a.noncomputing
    FROM asset a
    WHERE a.collection_id = $1
      AND a.state = 'enabled'
      AND (cardinality($2::bigint[]) = 0 OR a.asset_id = ANY($2::bigint[]))
      AND (
          -- No label filter at all: include every asset.
          (cardinality($3::uuid[]) = 0 AND cardinality($4::text[]) = 0 AND NOT $5::boolean)
          -- Positive label filter (labelId / labelName).
          OR (cardinality($3::uuid[]) > 0 AND EXISTS (
              SELECT 1 FROM collection_label_asset cla
              WHERE cla.asset_id = a.asset_id AND cla.label_id = ANY($3::uuid[])))
          OR (cardinality($4::text[]) > 0 AND EXISTS (
              SELECT 1 FROM collection_label_asset cla
                JOIN collection_label cl ON cl.label_id = cla.label_id
              WHERE cla.asset_id = a.asset_id AND lower(cl.name) = ANY($4::text[])))
          -- labelMatch=null: assets with NO labels.
          OR ($5::boolean AND NOT EXISTS (
              SELECT 1 FROM collection_label_asset cla
              WHERE cla.asset_id = a.asset_id))
      )
),
asset_benchmark AS (
    SELECT ast.asset_id, ast.benchmark_id, ast.revision_id AS pinned_revision
    FROM asset_stig ast
    JOIN target_assets ta ON ta.asset_id = ast.asset_id
    WHERE cardinality($6::text[]) = 0 OR ast.benchmark_id = ANY($6::text[])
),
effective_revisions AS (
    SELECT ab.asset_id, ab.benchmark_id,
           COALESCE(
               ab.pinned_revision,
               (SELECT sr.revision_id FROM stig_revision sr
                 WHERE sr.benchmark_id = ab.benchmark_id
                 ORDER BY sr.release_date DESC NULLS LAST, sr.revision_id DESC
                 LIMIT 1)
           ) AS revision_id,
           ab.pinned_revision IS NOT NULL AS revision_pinned
    FROM asset_benchmark ab
),
rule_universe AS (
    SELECT er.asset_id, er.benchmark_id, er.revision_id, er.revision_pinned,
           sr.rule_pk, sr.rule_id, sr.severity
    FROM effective_revisions er
    JOIN stig_rule sr ON sr.revision_id = er.revision_id
),
joined AS (
    SELECT ru.asset_id, ru.benchmark_id, ru.revision_id, ru.revision_pinned,
           ru.rule_pk, ru.rule_id, ru.severity,
           r.review_id IS NOT NULL AS has_review,
           r.result, r.status_label, r.ts, r.touch_ts
    FROM rule_universe ru
    LEFT JOIN review r ON r.asset_id = ru.asset_id AND r.rule_id = ru.rule_id
)`

// metricsAggExprs is the list of count/min/max expressions shared by
// every aggregation query. Used verbatim with `joined` as the source
// (optionally with a GROUP BY in front). Ordering must match the
// scanRowMetrics() implementation.
const metricsAggExprs = `
count(*) FILTER (WHERE has_review)                                                  AS assessed,
count(*) FILTER (WHERE has_review AND severity='high')                              AS assessed_high,
count(*) FILTER (WHERE has_review AND severity='medium')                            AS assessed_medium,
count(*) FILTER (WHERE has_review AND severity='low')                               AS assessed_low,
count(*)                                                                            AS assessments,
count(*) FILTER (WHERE severity='high')                                             AS assess_high,
count(*) FILTER (WHERE severity='medium')                                           AS assess_medium,
count(*) FILTER (WHERE severity='low')                                              AS assess_low,
count(*) FILTER (WHERE result='fail' AND severity='high')                           AS findings_high,
count(*) FILTER (WHERE result='fail' AND severity='medium')                         AS findings_medium,
count(*) FILTER (WHERE result='fail' AND severity='low')                            AS findings_low,
count(*) FILTER (WHERE result='pass')                                               AS r_pass,
count(*) FILTER (WHERE result='fail')                                               AS r_fail,
count(*) FILTER (WHERE result='notapplicable')                                      AS r_na,
count(*) FILTER (WHERE has_review AND result NOT IN ('pass','fail','notapplicable')) AS r_other,
count(*) FILTER (WHERE status_label='saved')                                        AS s_saved,
count(*) FILTER (WHERE status_label='submitted')                                    AS s_submitted,
count(*) FILTER (WHERE status_label='accepted')                                     AS s_accepted,
count(*) FILTER (WHERE status_label='rejected')                                     AS s_rejected,
min(ts)                                                                             AS min_ts,
max(ts)                                                                             AS max_ts,
max(touch_ts)                                                                       AS max_touch_ts`

// MetricsRow is the shared "MetricsSummary" payload populated for
// every endpoint. Counts are emitted as int64 from Postgres; the
// handler downcasts to the OpenAPI `int` shape after a sanity check.
type MetricsRow struct {
	Assessed            int64
	AssessedHigh        int64
	AssessedMedium      int64
	AssessedLow         int64
	Assessments         int64
	AssessmentsHigh     int64
	AssessmentsMedium   int64
	AssessmentsLow      int64
	FindingsHigh        int64
	FindingsMedium      int64
	FindingsLow         int64
	ResultPass          int64
	ResultFail          int64
	ResultNotApplicable int64
	ResultOther         int64
	StatusSaved         int64
	StatusSubmitted     int64
	StatusAccepted      int64
	StatusRejected      int64
	MinTS               *time.Time
	MaxTS               *time.Time
	MaxTouchTS          *time.Time
}

// MetricsLabelEntry is one row of a label join attached to an asset
// or unagg row.
type MetricsLabelEntry struct {
	LabelID string
	Name    string
	Color   string
}

// MetricsUnaggRow is one (asset, benchmark) row carrying the metrics
// for the rules in that pair's effective revision.
type MetricsUnaggRow struct {
	AssetID        int64
	AssetName      string
	BenchmarkID    string
	BenchmarkTitle string
	RevisionStr    string
	RevisionDate   *time.Time
	RevisionPinned bool
	Labels         []MetricsLabelEntry
	MetricsRow
}

// MetricsAggCollectionRow is the collection-wide rollup.
type MetricsAggCollectionRow struct {
	CollectionID   int64
	CollectionName string
	Assets         int64
	Stigs          int64
	Checklists     int64
	MetricsRow
}

// MetricsAggAssetRow is one row per asset.
type MetricsAggAssetRow struct {
	AssetID      int64
	AssetName    string
	Fqdn         *string
	IP           *string
	MAC          *string
	Noncomputing bool
	BenchmarkIDs []string
	Labels       []MetricsLabelEntry
	MetricsRow
}

// MetricsAggStigRow is one row per benchmark in the universe. The
// RevisionStr/RevisionDate/RuleCount are taken from the benchmark's
// *latest* revision (matching upstream's "agg by stig" panel which
// shows the canonical revision metadata even when individual assets
// are pinned to older revisions).
type MetricsAggStigRow struct {
	BenchmarkID    string
	BenchmarkTitle string
	RevisionStr    string
	RevisionDate   *time.Time
	RuleCount      int64
	Assets         int64
	MetricsRow
}

// MetricsAggLabelRow is one row per collection label, plus a synthetic
// "unlabeled" bucket with an empty LabelID for assets carrying no
// labels.
type MetricsAggLabelRow struct {
	LabelID string
	Name    string
	Color   string
	Assets  int64
	MetricsRow
}

// Unagg returns one row per (asset, benchmark) in the filtered
// universe. Labels are attached via array_agg.
func (r *MetricsRepo) Unagg(ctx context.Context, collectionID int64, f MetricsFilter) ([]MetricsUnaggRow, error) {
	q := fmt.Sprintf(`
WITH %s
SELECT
    j.asset_id, ta.name AS asset_name,
    j.benchmark_id,
    s.title AS benchmark_title,
    sv.revision_str, sv.release_date, j.revision_pinned,
    COALESCE(
        (SELECT json_agg(json_build_object(
            'label_id', cl.label_id,
            'name', cl.name,
            'color', cl.color
        ) ORDER BY cl.name)
         FROM collection_label_asset cla
         JOIN collection_label cl ON cl.label_id = cla.label_id
         WHERE cla.asset_id = j.asset_id),
        '[]'::json
    ) AS labels,
    %s
FROM joined j
JOIN target_assets ta ON ta.asset_id = j.asset_id
JOIN stig s ON s.benchmark_id = j.benchmark_id
JOIN stig_revision sv ON sv.revision_id = j.revision_id
GROUP BY j.asset_id, ta.name, j.benchmark_id, s.title,
         sv.revision_str, sv.release_date, j.revision_pinned
ORDER BY ta.name, j.benchmark_id`,
		universeCTE, metricsAggExprs)

	rows, err := r.pool.Query(ctx, q, f.bindArgs(collectionID)...)
	if err != nil {
		return nil, fmt.Errorf("metrics unagg: %w", err)
	}
	defer rows.Close()
	out := []MetricsUnaggRow{}
	for rows.Next() {
		var row MetricsUnaggRow
		var labelsJSON []byte
		err := rows.Scan(
			&row.AssetID, &row.AssetName,
			&row.BenchmarkID, &row.BenchmarkTitle,
			&row.RevisionStr, &row.RevisionDate, &row.RevisionPinned,
			&labelsJSON,
			&row.Assessed, &row.AssessedHigh, &row.AssessedMedium, &row.AssessedLow,
			&row.Assessments, &row.AssessmentsHigh, &row.AssessmentsMedium, &row.AssessmentsLow,
			&row.FindingsHigh, &row.FindingsMedium, &row.FindingsLow,
			&row.ResultPass, &row.ResultFail, &row.ResultNotApplicable, &row.ResultOther,
			&row.StatusSaved, &row.StatusSubmitted, &row.StatusAccepted, &row.StatusRejected,
			&row.MinTS, &row.MaxTS, &row.MaxTouchTS,
		)
		if err != nil {
			return nil, fmt.Errorf("scan unagg: %w", err)
		}
		row.Labels = decodeLabels(labelsJSON)
		out = append(out, row)
	}
	return out, rows.Err()
}

// AggCollection returns the collection-wide rollup.
func (r *MetricsRepo) AggCollection(ctx context.Context, collectionID int64, f MetricsFilter) (MetricsAggCollectionRow, error) {
	q := fmt.Sprintf(`
WITH %s
SELECT
    c.collection_id, c.name,
    (SELECT count(DISTINCT asset_id) FROM target_assets)         AS assets,
    (SELECT count(DISTINCT benchmark_id) FROM asset_benchmark)   AS stigs,
    (SELECT count(*) FROM asset_benchmark)                       AS checklists,
    %s
FROM joined j
RIGHT JOIN collection c ON c.collection_id = $1
GROUP BY c.collection_id, c.name`,
		universeCTE, metricsAggExprs)

	var row MetricsAggCollectionRow
	err := r.pool.QueryRow(ctx, q, f.bindArgs(collectionID)...).Scan(
		&row.CollectionID, &row.CollectionName,
		&row.Assets, &row.Stigs, &row.Checklists,
		&row.Assessed, &row.AssessedHigh, &row.AssessedMedium, &row.AssessedLow,
		&row.Assessments, &row.AssessmentsHigh, &row.AssessmentsMedium, &row.AssessmentsLow,
		&row.FindingsHigh, &row.FindingsMedium, &row.FindingsLow,
		&row.ResultPass, &row.ResultFail, &row.ResultNotApplicable, &row.ResultOther,
		&row.StatusSaved, &row.StatusSubmitted, &row.StatusAccepted, &row.StatusRejected,
		&row.MinTS, &row.MaxTS, &row.MaxTouchTS,
	)
	if err != nil {
		return row, fmt.Errorf("metrics agg collection: %w", err)
	}
	return row, nil
}

// AggAsset returns one row per asset in the filtered universe.
func (r *MetricsRepo) AggAsset(ctx context.Context, collectionID int64, f MetricsFilter) ([]MetricsAggAssetRow, error) {
	q := fmt.Sprintf(`
WITH %s
SELECT
    j.asset_id, ta.name, ta.fqdn, ta.ip, ta.mac, ta.noncomputing,
    COALESCE(
        (SELECT array_agg(DISTINCT ab.benchmark_id ORDER BY ab.benchmark_id)
         FROM asset_benchmark ab WHERE ab.asset_id = j.asset_id),
        '{}'::text[]
    ) AS benchmark_ids,
    COALESCE(
        (SELECT json_agg(json_build_object(
            'label_id', cl.label_id,
            'name', cl.name,
            'color', cl.color
        ) ORDER BY cl.name)
         FROM collection_label_asset cla
         JOIN collection_label cl ON cl.label_id = cla.label_id
         WHERE cla.asset_id = j.asset_id),
        '[]'::json
    ) AS labels,
    %s
FROM joined j
JOIN target_assets ta ON ta.asset_id = j.asset_id
GROUP BY j.asset_id, ta.name, ta.fqdn, ta.ip, ta.mac, ta.noncomputing
ORDER BY ta.name`,
		universeCTE, metricsAggExprs)

	rows, err := r.pool.Query(ctx, q, f.bindArgs(collectionID)...)
	if err != nil {
		return nil, fmt.Errorf("metrics agg asset: %w", err)
	}
	defer rows.Close()
	out := []MetricsAggAssetRow{}
	for rows.Next() {
		var row MetricsAggAssetRow
		var labelsJSON []byte
		err := rows.Scan(
			&row.AssetID, &row.AssetName, &row.Fqdn, &row.IP, &row.MAC, &row.Noncomputing,
			&row.BenchmarkIDs, &labelsJSON,
			&row.Assessed, &row.AssessedHigh, &row.AssessedMedium, &row.AssessedLow,
			&row.Assessments, &row.AssessmentsHigh, &row.AssessmentsMedium, &row.AssessmentsLow,
			&row.FindingsHigh, &row.FindingsMedium, &row.FindingsLow,
			&row.ResultPass, &row.ResultFail, &row.ResultNotApplicable, &row.ResultOther,
			&row.StatusSaved, &row.StatusSubmitted, &row.StatusAccepted, &row.StatusRejected,
			&row.MinTS, &row.MaxTS, &row.MaxTouchTS,
		)
		if err != nil {
			return nil, fmt.Errorf("scan agg asset: %w", err)
		}
		row.Labels = decodeLabels(labelsJSON)
		out = append(out, row)
	}
	return out, rows.Err()
}

// AggStig returns one row per benchmark in the filtered universe.
// Revision metadata is drawn from the benchmark's latest revision; the
// per-asset effective revision still drives the rule_universe so an
// asset pinned to an older revision will contribute counts even if
// its rules are absent from the displayed (latest) revision.
func (r *MetricsRepo) AggStig(ctx context.Context, collectionID int64, f MetricsFilter) ([]MetricsAggStigRow, error) {
	q := fmt.Sprintf(`
WITH %s, latest_revision AS (
    SELECT DISTINCT ON (sr.benchmark_id)
           sr.benchmark_id, sr.revision_id, sr.revision_str, sr.release_date
    FROM stig_revision sr
    ORDER BY sr.benchmark_id, sr.release_date DESC NULLS LAST, sr.revision_id DESC
)
SELECT
    j.benchmark_id,
    s.title,
    lr.revision_str, lr.release_date,
    (SELECT count(*) FROM stig_rule WHERE revision_id = lr.revision_id) AS rule_count,
    count(DISTINCT j.asset_id) AS assets,
    %s
FROM joined j
JOIN stig s ON s.benchmark_id = j.benchmark_id
LEFT JOIN latest_revision lr ON lr.benchmark_id = j.benchmark_id
GROUP BY j.benchmark_id, s.title, lr.revision_str, lr.release_date, lr.revision_id
ORDER BY j.benchmark_id`,
		universeCTE, metricsAggExprs)

	rows, err := r.pool.Query(ctx, q, f.bindArgs(collectionID)...)
	if err != nil {
		return nil, fmt.Errorf("metrics agg stig: %w", err)
	}
	defer rows.Close()
	out := []MetricsAggStigRow{}
	for rows.Next() {
		var row MetricsAggStigRow
		err := rows.Scan(
			&row.BenchmarkID, &row.BenchmarkTitle,
			&row.RevisionStr, &row.RevisionDate, &row.RuleCount, &row.Assets,
			&row.Assessed, &row.AssessedHigh, &row.AssessedMedium, &row.AssessedLow,
			&row.Assessments, &row.AssessmentsHigh, &row.AssessmentsMedium, &row.AssessmentsLow,
			&row.FindingsHigh, &row.FindingsMedium, &row.FindingsLow,
			&row.ResultPass, &row.ResultFail, &row.ResultNotApplicable, &row.ResultOther,
			&row.StatusSaved, &row.StatusSubmitted, &row.StatusAccepted, &row.StatusRejected,
			&row.MinTS, &row.MaxTS, &row.MaxTouchTS,
		)
		if err != nil {
			return nil, fmt.Errorf("scan agg stig: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// AggLabel returns one row per collection label that is applied to at
// least one asset in the universe, plus an "unlabeled" bucket
// (LabelID == "") for assets carrying no labels. An asset with N
// labels contributes once to each of its labels.
func (r *MetricsRepo) AggLabel(ctx context.Context, collectionID int64, f MetricsFilter) ([]MetricsAggLabelRow, error) {
	q := fmt.Sprintf(`
WITH %s, joined_labels AS (
    SELECT j.*, cla.label_id, cl.name, cl.color
    FROM joined j
    LEFT JOIN collection_label_asset cla ON cla.asset_id = j.asset_id
    LEFT JOIN collection_label cl ON cl.label_id = cla.label_id
)
SELECT
    label_id, COALESCE(name, ''), COALESCE(color, ''),
    count(DISTINCT asset_id) AS assets,
    %s
FROM joined_labels
GROUP BY label_id, name, color
ORDER BY (label_id IS NULL), name`,
		universeCTE, metricsAggExprs)

	rows, err := r.pool.Query(ctx, q, f.bindArgs(collectionID)...)
	if err != nil {
		return nil, fmt.Errorf("metrics agg label: %w", err)
	}
	defer rows.Close()
	out := []MetricsAggLabelRow{}
	for rows.Next() {
		var row MetricsAggLabelRow
		var labelID *string
		err := rows.Scan(
			&labelID, &row.Name, &row.Color, &row.Assets,
			&row.Assessed, &row.AssessedHigh, &row.AssessedMedium, &row.AssessedLow,
			&row.Assessments, &row.AssessmentsHigh, &row.AssessmentsMedium, &row.AssessmentsLow,
			&row.FindingsHigh, &row.FindingsMedium, &row.FindingsLow,
			&row.ResultPass, &row.ResultFail, &row.ResultNotApplicable, &row.ResultOther,
			&row.StatusSaved, &row.StatusSubmitted, &row.StatusAccepted, &row.StatusRejected,
			&row.MinTS, &row.MaxTS, &row.MaxTouchTS,
		)
		if err != nil {
			return nil, fmt.Errorf("scan agg label: %w", err)
		}
		if labelID != nil {
			row.LabelID = *labelID
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
