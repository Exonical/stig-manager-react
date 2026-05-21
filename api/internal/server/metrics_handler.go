package server

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Exonical/stig-manager-react/api/internal/api"
	"github.com/Exonical/stig-manager-react/api/internal/store"
)

// summaryMetrics is the OpenAPI MetricsSummary.Metrics struct laid out
// as a plain map-builder so we can populate the anonymous inline struct
// produced by the generator without lots of long-winded literals at
// every call site.
//
// Returning map[string]any here is intentional: each of the five
// MetricsSummary* generated types has a different `Metrics` field with
// the same inline shape; copying the values into the typed struct
// would require five duplicated builders. JSON-marshalling the map
// produces an identical payload byte-for-byte.
func summaryMetricsMap(m store.MetricsRow) map[string]any {
	return map[string]any{
		"assessed": m.Assessed,
		"assessedBySeverity": map[string]int64{
			"high":   m.AssessedHigh,
			"medium": m.AssessedMedium,
			"low":    m.AssessedLow,
		},
		"assessments": m.Assessments,
		"assessmentsBySeverity": map[string]int64{
			"high":   m.AssessmentsHigh,
			"medium": m.AssessmentsMedium,
			"low":    m.AssessmentsLow,
		},
		"findings": map[string]int64{
			"high":   m.FindingsHigh,
			"medium": m.FindingsMedium,
			"low":    m.FindingsLow,
		},
		"maxTouchTs": jsonTime(m.MaxTouchTS),
		"maxTs":      jsonTime(m.MaxTS),
		"minTs":      jsonTime(m.MinTS),
		"results": map[string]int64{
			"pass":          m.ResultPass,
			"fail":          m.ResultFail,
			"notapplicable": m.ResultNotApplicable,
			"other":         m.ResultOther,
		},
		"statuses": map[string]int64{
			"saved":     m.StatusSaved,
			"submitted": m.StatusSubmitted,
			"accepted":  m.StatusAccepted,
			"rejected":  m.StatusRejected,
		},
	}
}

// jsonTime marshals a pointer-to-time so a nil pointer becomes JSON
// null and a populated one becomes the RFC3339 string the OpenAPI
// spec asks for.
func jsonTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// jsonLabels is the LabelBasicWithColor[] payload used by the unagg
// and per-asset response shapes.
func jsonLabels(labels []store.MetricsLabelEntry) []map[string]any {
	out := make([]map[string]any, 0, len(labels))
	for _, l := range labels {
		out = append(out, map[string]any{
			"labelId": l.LabelID,
			"name":    l.Name,
			"color":   l.Color,
		})
	}
	return out
}

// parseMetricsFilter pulls the shared metrics query parameters out of
// the parsed-params struct the generator emits and converts them into
// the typed MetricsFilter the store layer expects.
type metricsCommonParams struct {
	benchmarkID *[]*api.BenchmarkId
	assetID     *[]api.AssetId
	labelID     *[]api.LabelId
	labelName   *[]api.LabelName
	labelMatch  *api.LabelMatchQuery
	format      *api.MetricsFormatQuery
}

func (p metricsCommonParams) filter() store.MetricsFilter {
	f := store.MetricsFilter{}
	if p.benchmarkID != nil {
		for _, b := range *p.benchmarkID {
			if b != nil && *b != "" {
				f.BenchmarkIDs = append(f.BenchmarkIDs, *b)
			}
		}
	}
	if p.assetID != nil {
		for _, a := range *p.assetID {
			if id, err := strconv.ParseInt(a, 10, 64); err == nil && id > 0 {
				f.AssetIDs = append(f.AssetIDs, id)
			}
		}
	}
	if p.labelID != nil {
		for _, l := range *p.labelID {
			if l != "" {
				f.LabelIDs = append(f.LabelIDs, l)
			}
		}
	}
	if p.labelName != nil {
		for _, n := range *p.labelName {
			if n != "" {
				f.LabelNames = append(f.LabelNames, n)
			}
		}
	}
	if p.labelMatch != nil && *p.labelMatch == api.LabelMatchQueryNull {
		f.LabelMatchNull = true
	}
	return f
}

// wantsCSV returns true when format=csv was requested.
func wantsCSV(p metricsCommonParams) bool {
	return p.format != nil && *p.format == api.MetricsFormatQueryCsv
}

// loadFilter wraps the auth check + parameter parsing common to every
// metrics endpoint. Returns (filter, collectionID, wantsCSV, ok). When
// ok is false the caller has already written the error response.
func (s APIServer) loadMetricsFilter(
	w http.ResponseWriter, r *http.Request,
	collectionIDPath api.CollectionIdPath, p metricsCommonParams,
) (store.MetricsFilter, int64, bool, bool) {
	collID, ok := parseInt64Path(string(collectionIDPath))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return store.MetricsFilter{}, 0, false, false
	}
	if _, _, _, ok := s.authorizeCollection(w, r, collID, "stig-manager:collection:read", RoleRestricted); !ok {
		return store.MetricsFilter{}, 0, false, false
	}
	if s.Metrics == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "metrics unavailable")
		return store.MetricsFilter{}, 0, false, false
	}
	return p.filter(), collID, wantsCSV(p), true
}

// GetMetricsSummaryByCollection returns one summary row per
// (asset, stig) pair (the "unagg" view).
func (s APIServer) GetMetricsSummaryByCollection(
	w http.ResponseWriter, r *http.Request,
	collectionId api.CollectionIdPath, params api.GetMetricsSummaryByCollectionParams,
) {
	common := metricsCommonParams{
		benchmarkID: params.BenchmarkId, assetID: params.AssetId,
		labelID: params.LabelId, labelName: params.LabelName,
		labelMatch: params.LabelMatch, format: params.Format,
	}
	filter, collID, csv, ok := s.loadMetricsFilter(w, r, collectionId, common)
	if !ok {
		return
	}
	rows, err := s.Metrics.Unagg(r.Context(), collID, filter)
	if err != nil {
		s.logErr(r, "metrics unagg", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to compute metrics")
		return
	}
	if csv {
		writeMetricsCSV(w, unaggCSV(rows))
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{
			"assetId":        strconv.FormatInt(row.AssetID, 10),
			"benchmarkId":    nullableString(row.BenchmarkID),
			"labels":         jsonLabels(row.Labels),
			"name":           row.AssetName,
			"revisionDate":   jsonDate(row.RevisionDate),
			"revisionPinned": row.RevisionPinned,
			"revisionStr":    row.RevisionStr,
			"title":          row.BenchmarkTitle,
			"metrics":        summaryMetricsMap(row.MetricsRow),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// GetMetricsSummaryByCollectionAgg returns the single collection-wide
// rollup row.
func (s APIServer) GetMetricsSummaryByCollectionAgg(
	w http.ResponseWriter, r *http.Request,
	collectionId api.CollectionIdPath, params api.GetMetricsSummaryByCollectionAggParams,
) {
	common := metricsCommonParams{
		benchmarkID: params.BenchmarkId, assetID: params.AssetId,
		labelID: params.LabelId, labelName: params.LabelName,
		labelMatch: params.LabelMatch, format: params.Format,
	}
	filter, collID, csv, ok := s.loadMetricsFilter(w, r, collectionId, common)
	if !ok {
		return
	}
	row, err := s.Metrics.AggCollection(r.Context(), collID, filter)
	if err != nil {
		s.logErr(r, "metrics agg collection", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to compute metrics")
		return
	}
	body := map[string]any{
		"assets":       row.Assets,
		"checklists":   row.Checklists,
		"collectionId": strconv.FormatInt(row.CollectionID, 10),
		"name":         row.CollectionName,
		"stigs":        row.Stigs,
		"metrics":      summaryMetricsMap(row.MetricsRow),
	}
	if csv {
		writeMetricsCSV(w, [][]string{
			collectionAggCSVHeader(),
			collectionAggCSVRow(row),
		})
		return
	}
	writeJSON(w, http.StatusOK, body)
}

// GetMetricsSummaryByCollectionAggAsset returns one row per asset.
func (s APIServer) GetMetricsSummaryByCollectionAggAsset(
	w http.ResponseWriter, r *http.Request,
	collectionId api.CollectionIdPath, params api.GetMetricsSummaryByCollectionAggAssetParams,
) {
	common := metricsCommonParams{
		benchmarkID: params.BenchmarkId, assetID: params.AssetId,
		labelID: params.LabelId, labelName: params.LabelName,
		labelMatch: params.LabelMatch, format: params.Format,
	}
	filter, collID, csv, ok := s.loadMetricsFilter(w, r, collectionId, common)
	if !ok {
		return
	}
	rows, err := s.Metrics.AggAsset(r.Context(), collID, filter)
	if err != nil {
		s.logErr(r, "metrics agg asset", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to compute metrics")
		return
	}
	if csv {
		writeMetricsCSV(w, assetAggCSV(rows))
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		benchmarkIDs := make([]any, 0, len(row.BenchmarkIDs))
		for _, b := range row.BenchmarkIDs {
			benchmarkIDs = append(benchmarkIDs, b)
		}
		body := map[string]any{
			"assetId":      strconv.FormatInt(row.AssetID, 10),
			"benchmarkIds": benchmarkIDs,
			"labels":       jsonLabels(row.Labels),
			"name":         row.AssetName,
			"noncomputing": row.Noncomputing,
			"metrics":      summaryMetricsMap(row.MetricsRow),
		}
		if row.Fqdn != nil {
			body["fqdn"] = *row.Fqdn
		}
		if row.IP != nil {
			body["ip"] = *row.IP
		}
		if row.MAC != nil {
			body["mac"] = *row.MAC
		}
		out = append(out, body)
	}
	writeJSON(w, http.StatusOK, out)
}

// GetMetricsSummaryByCollectionAggStig returns one row per benchmark.
func (s APIServer) GetMetricsSummaryByCollectionAggStig(
	w http.ResponseWriter, r *http.Request,
	collectionId api.CollectionIdPath, params api.GetMetricsSummaryByCollectionAggStigParams,
) {
	common := metricsCommonParams{
		benchmarkID: params.BenchmarkId, assetID: params.AssetId,
		labelID: params.LabelId, labelName: params.LabelName,
		labelMatch: params.LabelMatch, format: params.Format,
	}
	filter, collID, csv, ok := s.loadMetricsFilter(w, r, collectionId, common)
	if !ok {
		return
	}
	rows, err := s.Metrics.AggStig(r.Context(), collID, filter)
	if err != nil {
		s.logErr(r, "metrics agg stig", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to compute metrics")
		return
	}
	if csv {
		writeMetricsCSV(w, stigAggCSV(rows))
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{
			"assets":       row.Assets,
			"benchmarkId":  nullableString(row.BenchmarkID),
			"revisionDate": jsonDate(row.RevisionDate),
			"revisionStr":  row.RevisionStr,
			"ruleCount":    row.RuleCount,
			"title":        row.BenchmarkTitle,
			"metrics":      summaryMetricsMap(row.MetricsRow),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// GetMetricsSummaryByCollectionAggLabel returns one row per label,
// plus a synthetic unlabeled bucket where applicable.
func (s APIServer) GetMetricsSummaryByCollectionAggLabel(
	w http.ResponseWriter, r *http.Request,
	collectionId api.CollectionIdPath, params api.GetMetricsSummaryByCollectionAggLabelParams,
) {
	common := metricsCommonParams{
		benchmarkID: params.BenchmarkId, assetID: params.AssetId,
		labelID: params.LabelId, labelName: params.LabelName,
		labelMatch: params.LabelMatch, format: params.Format,
	}
	filter, collID, csv, ok := s.loadMetricsFilter(w, r, collectionId, common)
	if !ok {
		return
	}
	rows, err := s.Metrics.AggLabel(r.Context(), collID, filter)
	if err != nil {
		s.logErr(r, "metrics agg label", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to compute metrics")
		return
	}
	if csv {
		writeMetricsCSV(w, labelAggCSV(rows))
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		body := map[string]any{
			"assets":  row.Assets,
			"metrics": summaryMetricsMap(row.MetricsRow),
		}
		if row.LabelID == "" {
			body["labelId"] = nil
			body["name"] = nil
		} else {
			body["labelId"] = row.LabelID
			body["name"] = row.Name
		}
		out = append(out, body)
	}
	writeJSON(w, http.StatusOK, out)
}

// nullableString turns an empty string into a JSON null (matching the
// OpenAPI String255Nullable / BenchmarkId shape).
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// jsonDate marshals a *time.Time as a YYYY-MM-DD string for the
// OpenAPI date-only shape (StringDateNullable). Nil -> JSON null.
func jsonDate(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format("2006-01-02")
}

// writeMetricsCSV writes the table as text/csv with a stable header.
func writeMetricsCSV(w http.ResponseWriter, rows [][]string) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	cw := csv.NewWriter(w)
	_ = cw.WriteAll(rows)
	cw.Flush()
}

// metricsCSVHeader returns the column list shared by every metrics
// CSV export. The first columns are endpoint-specific (e.g.
// assetId,name) and concatenated by the per-endpoint helpers.
func metricsCSVHeader() []string {
	return []string{
		"assessed", "assessedHigh", "assessedMedium", "assessedLow",
		"assessments", "assessmentsHigh", "assessmentsMedium", "assessmentsLow",
		"findingsHigh", "findingsMedium", "findingsLow",
		"resultPass", "resultFail", "resultNotApplicable", "resultOther",
		"statusSaved", "statusSubmitted", "statusAccepted", "statusRejected",
		"minTs", "maxTs", "maxTouchTs",
	}
}

// metricsCSVCells returns the 22 cell values matching metricsCSVHeader.
func metricsCSVCells(m store.MetricsRow) []string {
	return []string{
		i64s(m.Assessed), i64s(m.AssessedHigh), i64s(m.AssessedMedium), i64s(m.AssessedLow),
		i64s(m.Assessments), i64s(m.AssessmentsHigh), i64s(m.AssessmentsMedium), i64s(m.AssessmentsLow),
		i64s(m.FindingsHigh), i64s(m.FindingsMedium), i64s(m.FindingsLow),
		i64s(m.ResultPass), i64s(m.ResultFail), i64s(m.ResultNotApplicable), i64s(m.ResultOther),
		i64s(m.StatusSaved), i64s(m.StatusSubmitted), i64s(m.StatusAccepted), i64s(m.StatusRejected),
		tsCell(m.MinTS), tsCell(m.MaxTS), tsCell(m.MaxTouchTS),
	}
}

func i64s(n int64) string { return strconv.FormatInt(n, 10) }

func tsCell(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func collectionAggCSVHeader() []string {
	return append([]string{"collectionId", "name", "assets", "stigs", "checklists"}, metricsCSVHeader()...)
}

func collectionAggCSVRow(row store.MetricsAggCollectionRow) []string {
	return append(
		[]string{
			i64s(row.CollectionID), row.CollectionName,
			i64s(row.Assets), i64s(row.Stigs), i64s(row.Checklists),
		},
		metricsCSVCells(row.MetricsRow)...,
	)
}

func unaggCSV(rows []store.MetricsUnaggRow) [][]string {
	out := [][]string{
		append([]string{
			"assetId", "name", "benchmarkId", "title",
			"revisionStr", "revisionDate", "revisionPinned",
		}, metricsCSVHeader()...),
	}
	for _, row := range rows {
		date := ""
		if row.RevisionDate != nil {
			date = row.RevisionDate.UTC().Format("2006-01-02")
		}
		out = append(out, append(
			[]string{
				i64s(row.AssetID), row.AssetName, row.BenchmarkID, row.BenchmarkTitle,
				row.RevisionStr, date, fmt.Sprintf("%v", row.RevisionPinned),
			},
			metricsCSVCells(row.MetricsRow)...,
		))
	}
	return out
}

func assetAggCSV(rows []store.MetricsAggAssetRow) [][]string {
	out := [][]string{
		append([]string{"assetId", "name", "fqdn", "ip", "mac", "noncomputing"}, metricsCSVHeader()...),
	}
	for _, row := range rows {
		f, ip, mac := "", "", ""
		if row.Fqdn != nil {
			f = *row.Fqdn
		}
		if row.IP != nil {
			ip = *row.IP
		}
		if row.MAC != nil {
			mac = *row.MAC
		}
		out = append(out, append(
			[]string{
				i64s(row.AssetID), row.AssetName, f, ip, mac,
				fmt.Sprintf("%v", row.Noncomputing),
			},
			metricsCSVCells(row.MetricsRow)...,
		))
	}
	return out
}

func stigAggCSV(rows []store.MetricsAggStigRow) [][]string {
	out := [][]string{
		append([]string{
			"benchmarkId", "title", "revisionStr", "revisionDate", "ruleCount", "assets",
		}, metricsCSVHeader()...),
	}
	for _, row := range rows {
		date := ""
		if row.RevisionDate != nil {
			date = row.RevisionDate.UTC().Format("2006-01-02")
		}
		out = append(out, append(
			[]string{
				row.BenchmarkID, row.BenchmarkTitle, row.RevisionStr, date,
				i64s(row.RuleCount), i64s(row.Assets),
			},
			metricsCSVCells(row.MetricsRow)...,
		))
	}
	return out
}

func labelAggCSV(rows []store.MetricsAggLabelRow) [][]string {
	out := [][]string{
		append([]string{"labelId", "name", "color", "assets"}, metricsCSVHeader()...),
	}
	for _, row := range rows {
		out = append(out, append(
			[]string{row.LabelID, row.Name, row.Color, i64s(row.Assets)},
			metricsCSVCells(row.MetricsRow)...,
		))
	}
	return out
}
