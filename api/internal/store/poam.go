package store

import (
	"context"
	"fmt"

	"github.com/Exonical/stig-manager-react/api/internal/poam"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PoamRepo aggregates "fail" / Open reviews into POA&M findings.
// Like MetricsRepo it builds on the rule_universe CTE so the
// asset / benchmark resolution logic stays consistent with the
// metrics dashboard.
type PoamRepo struct {
	pool *pgxpool.Pool
}

// NewPoamRepo wraps a pgxpool.Pool.
func NewPoamRepo(pool *pgxpool.Pool) *PoamRepo {
	return &PoamRepo{pool: pool}
}

// PoamAggregator picks between group-level and rule-level aggregation.
type PoamAggregator string

const (
	// AggregateByGroup rolls findings up by stig_rule.group_id (V-…).
	AggregateByGroup PoamAggregator = "groupId"
	// AggregateByRule keeps one finding per stig_rule.rule_id (SV-…).
	AggregateByRule PoamAggregator = "ruleId"
)

// PoamFilter narrows the set of rules considered for the report.
// AcceptedOnly restricts the underlying "fail" reviews to those whose
// review.status_label = 'accepted'. BenchmarkIDs and AssetIDs prune
// the universe to the matching subsets (OR within each list, AND
// across lists). Empty slices mean "no filter on that field".
type PoamFilter struct {
	Aggregator   PoamAggregator
	AcceptedOnly bool
	BenchmarkIDs []string
	AssetIDs     []int64
}

// GetFindings runs the aggregated query and returns one Finding per
// (aggregator key) group, ordered by descending severity then
// identifier.
func (r *PoamRepo) GetFindings(ctx context.Context, collectionID int64, filter PoamFilter) ([]poam.Finding, error) {
	agg := filter.Aggregator
	if agg == "" {
		agg = AggregateByGroup
	}

	var aggCol string
	switch agg {
	case AggregateByRule:
		aggCol = "ru.rule_id"
	default:
		aggCol = "sr.group_id"
	}

	// $1..$6 come from MetricsFilter.bindArgs; $7 = AcceptedOnly.
	// We reuse the metrics universeCTE but drop the label filter
	// columns by passing empty arrays.
	sql := `
WITH ` + universeCTE + `,
findings AS (
    SELECT ` + aggCol + ` AS agg_key,
           ru.rule_pk, ru.rule_id, ru.severity, ru.benchmark_id,
           ru.revision_id, ru.asset_id,
           sr.group_id, sr.group_title, sr.title AS rule_title,
           sr.description AS vuln_discussion,
           a.name AS asset_name,
           srev.revision_str, srev.benchmark_date
    FROM joined j
    JOIN rule_universe ru ON ru.rule_pk = j.rule_pk AND ru.asset_id = j.asset_id
    JOIN stig_rule sr ON sr.rule_pk = ru.rule_pk
    JOIN asset a ON a.asset_id = ru.asset_id
    JOIN stig_revision srev ON srev.revision_id = ru.revision_id
    WHERE j.has_review AND j.result = 'fail'
      AND ($7::boolean = false OR j.status_label = 'accepted')
)
SELECT
    f.agg_key,
    -- pick a representative title: group title when aggregating by
    -- group, rule title when aggregating by rule
    CASE WHEN $8::text = 'groupId' THEN max(f.group_title)
         ELSE max(f.rule_title)
    END AS title,
    -- max severity wins (high > medium > low > unknown)
    CASE
        WHEN bool_or(f.severity = 'high')   THEN 'high'
        WHEN bool_or(f.severity = 'medium') THEN 'medium'
        WHEN bool_or(f.severity = 'low')    THEN 'low'
        ELSE 'unknown'
    END AS severity,
    -- the de-dup'd rule list as parallel arrays so the Go side can
    -- zip them into []Rule without parsing JSON.
    (SELECT array_agg(DISTINCT g.rule_id ORDER BY g.rule_id)         FROM findings g WHERE g.agg_key = f.agg_key) AS rule_ids,
    (SELECT array_agg(DISTINCT g.rule_title ORDER BY g.rule_title)    FROM findings g WHERE g.agg_key = f.agg_key) AS rule_titles,
    (SELECT array_agg(DISTINCT g.severity ORDER BY g.severity)        FROM findings g WHERE g.agg_key = f.agg_key) AS rule_severities,
    -- a single representative vuln discussion: just the first one.
    (SELECT g.vuln_discussion FROM findings g WHERE g.agg_key = f.agg_key
       ORDER BY g.rule_id LIMIT 1) AS vuln_discussion,
    (SELECT array_agg(DISTINCT g.asset_id::text ORDER BY g.asset_id::text) FROM findings g WHERE g.agg_key = f.agg_key) AS asset_ids,
    (SELECT array_agg(DISTINCT g.asset_name ORDER BY g.asset_name)    FROM findings g WHERE g.agg_key = f.agg_key) AS asset_names,
    (SELECT array_agg(DISTINCT g.benchmark_id ORDER BY g.benchmark_id) FROM findings g WHERE g.agg_key = f.agg_key) AS benchmark_ids,
    (SELECT array_agg(DISTINCT g.revision_str ORDER BY g.revision_str) FROM findings g WHERE g.agg_key = f.agg_key) AS revision_strs,
    (SELECT array_agg(DISTINCT to_char(g.benchmark_date, 'YYYY-MM-DD') ORDER BY to_char(g.benchmark_date, 'YYYY-MM-DD'))
       FROM findings g WHERE g.agg_key = f.agg_key) AS benchmark_dates,
    (SELECT array_agg(DISTINCT src.cci ORDER BY src.cci)
       FROM stig_rule_cci src
       JOIN findings g ON g.rule_pk = src.rule_pk
       WHERE g.agg_key = f.agg_key) AS ccis
FROM findings f
GROUP BY f.agg_key
ORDER BY
    CASE
        WHEN bool_or(f.severity = 'high')   THEN 0
        WHEN bool_or(f.severity = 'medium') THEN 1
        WHEN bool_or(f.severity = 'low')    THEN 2
        ELSE 3
    END,
    f.agg_key
`
	args := MetricsFilter{
		BenchmarkIDs: filter.BenchmarkIDs,
		AssetIDs:     filter.AssetIDs,
	}.bindArgs(collectionID)
	args = append(args, filter.AcceptedOnly, string(agg))

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("poam findings query: %w", err)
	}
	defer rows.Close()

	findings, err := scanFindings(rows, agg)
	if err != nil {
		return nil, err
	}
	if findings == nil {
		findings = []poam.Finding{}
	}
	return findings, nil
}

func scanFindings(rows pgx.Rows, agg PoamAggregator) ([]poam.Finding, error) {
	var out []poam.Finding
	for rows.Next() {
		var (
			aggKey         string
			title          string
			severity       string
			ruleIDs        []string
			ruleTitles     []string
			ruleSeverities []string
			vulnDiscussion *string
			assetIDs       []string
			assetNames     []string
			benchmarkIDs   []string
			revisionStrs   []string
			benchmarkDates []string
			ccis           []string
		)
		if err := rows.Scan(
			&aggKey, &title, &severity,
			&ruleIDs, &ruleTitles, &ruleSeverities,
			&vulnDiscussion,
			&assetIDs, &assetNames,
			&benchmarkIDs, &revisionStrs, &benchmarkDates,
			&ccis,
		); err != nil {
			return nil, fmt.Errorf("poam findings scan: %w", err)
		}
		f := poam.Finding{
			Title:    title,
			Severity: severity,
		}
		switch agg {
		case AggregateByRule:
			f.RuleID = aggKey
		default:
			f.GroupID = aggKey
		}
		// Zip the parallel rule arrays. ruleIDs is the authoritative
		// length; the others arrive in matched DISTINCT order, so the
		// indices line up.
		f.Rules = make([]poam.Rule, 0, len(ruleIDs))
		disc := ""
		if vulnDiscussion != nil {
			disc = *vulnDiscussion
		}
		for i, rid := range ruleIDs {
			rt := ""
			if i < len(ruleTitles) {
				rt = ruleTitles[i]
			}
			sev := ""
			if i < len(ruleSeverities) {
				sev = ruleSeverities[i]
			}
			f.Rules = append(f.Rules, poam.Rule{
				RuleID:         rid,
				Title:          rt,
				Severity:       sev,
				VulnDiscussion: disc,
			})
		}
		// Zip assets.
		f.Assets = make([]poam.Asset, 0, len(assetIDs))
		for i, id := range assetIDs {
			name := ""
			if i < len(assetNames) {
				name = assetNames[i]
			}
			f.Assets = append(f.Assets, poam.Asset{AssetID: id, Name: name})
		}
		// Zip stigs.
		f.Stigs = make([]poam.Stig, 0, len(benchmarkIDs))
		for i, bid := range benchmarkIDs {
			rev := ""
			if i < len(revisionStrs) {
				rev = revisionStrs[i]
			}
			bd := ""
			if i < len(benchmarkDates) {
				bd = benchmarkDates[i]
			}
			f.Stigs = append(f.Stigs, poam.Stig{
				BenchmarkID:   bid,
				RevisionStr:   rev,
				BenchmarkDate: bd,
			})
		}
		// CCIs are bare identifiers right now (we don't yet ingest the
		// CCI definition XML); set only the CCI field.
		f.CCIs = make([]poam.CCI, 0, len(ccis))
		for _, c := range ccis {
			f.CCIs = append(f.CCIs, poam.CCI{CCI: c})
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
