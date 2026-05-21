package server

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Exonical/stig-manager-react/api/internal/api"
	"github.com/Exonical/stig-manager-react/api/internal/checklist"
	"github.com/Exonical/stig-manager-react/api/internal/store"
)

// GetChecklistByAsset returns a multi-STIG CKL/CKLB file for an Asset
// across one or more of its mapped STIGs. Defaults to CKL when format
// is omitted.
func (s APIServer) GetChecklistByAsset(
	w http.ResponseWriter, r *http.Request,
	assetId api.AssetIdPath, params api.GetChecklistByAssetParams,
) {
	aID, ok := parseInt64Path(string(assetId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid assetId")
		return
	}
	repo, asset, ok := s.resolveChecklistContext(w, r, aID, "stig-manager:collection:read", RoleRestricted)
	if !ok {
		return
	}

	var benchmarkIDs []string
	if params.BenchmarkId != nil {
		for _, b := range *params.BenchmarkId {
			if b == nil {
				continue
			}
			id := strings.TrimSpace(string(*b))
			if id != "" {
				benchmarkIDs = append(benchmarkIDs, id)
			}
		}
	}

	multi, err := repo.AssetMulti(r.Context(), asset.AssetID, benchmarkIDs)
	if err != nil {
		s.logErr(r, "asset multi checklist", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to load checklist")
		return
	}

	format := "ckl"
	if params.Format != nil {
		format = strings.ToLower(strings.TrimSpace(*params.Format))
	}
	export := toExportData(multi)

	switch format {
	case "cklb":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", contentDisposition(asset.Name+".cklb"))
		_ = checklist.WriteCKLB(w, export)
	case "ckl", "":
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Header().Set("Content-Disposition", contentDisposition(asset.Name+".ckl"))
		_ = checklist.WriteCKL(w, export)
	default:
		writeAuthError(w, http.StatusBadRequest, "unsupported format: "+format)
	}
}

// GetChecklistByAssetStig returns a single-STIG export for an Asset in
// one of JSON / CKL / CKLB / XCCDF (per the OpenAPI spec). The
// json-access projection is deferred to a later milestone (returns
// 501 today).
func (s APIServer) GetChecklistByAssetStig(
	w http.ResponseWriter, r *http.Request,
	assetId api.AssetIdPath, benchmarkId api.BenchmarkIdPath, revisionStr api.RevisionStrPath,
	params api.GetChecklistByAssetStigParams,
) {
	aID, ok := parseInt64Path(string(assetId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid assetId")
		return
	}
	bID := strings.TrimSpace(string(benchmarkId))
	if bID == "" {
		writeAuthError(w, http.StatusBadRequest, "invalid benchmarkId")
		return
	}
	rev := strings.TrimSpace(string(revisionStr))

	repo, asset, ok := s.resolveChecklistContext(w, r, aID, "stig-manager:collection:read", RoleRestricted)
	if !ok {
		return
	}

	data, err := repo.AssetSingle(r.Context(), asset.AssetID, bID, rev)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "benchmark or revision not found")
			return
		}
		s.logErr(r, "asset single checklist", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to load checklist")
		return
	}
	export := toExportData(data)

	format := "json"
	if params.Format != nil {
		format = strings.ToLower(strings.TrimSpace(string(*params.Format)))
	}
	fileBase := asset.Name + "_" + bID
	if len(data.Stigs) > 0 && data.Stigs[0].RevisionStr != "" {
		fileBase += "-" + data.Stigs[0].RevisionStr
	}

	switch format {
	case "ckl":
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Header().Set("Content-Disposition", contentDisposition(fileBase+".ckl"))
		_ = checklist.WriteCKL(w, export)
	case "cklb":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", contentDisposition(fileBase+".cklb"))
		_ = checklist.WriteCKLB(w, export)
	case "xccdf":
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Header().Set("Content-Disposition", contentDisposition(fileBase+"-xccdf-results.xml"))
		_ = checklist.WriteXCCDFResults(w, export)
	case "json", "":
		writeJSON(w, http.StatusOK, jsonAssetChecklist(data))
	case "json-access":
		// Per-rule access projection lands with the per-rule ACL
		// milestone; today the spec slot is reserved with a 501.
		w.WriteHeader(http.StatusNotImplemented)
	default:
		writeAuthError(w, http.StatusBadRequest, "unsupported format: "+format)
	}
}

// GetChecklistByCollectionStig returns the per-rule rollup across all
// assets in the collection that have the benchmark applied.
func (s APIServer) GetChecklistByCollectionStig(
	w http.ResponseWriter, r *http.Request,
	collectionId api.CollectionIdPath, benchmarkId api.BenchmarkIdPath, revisionStr api.RevisionStrPath,
) {
	cID, ok := parseInt64Path(string(collectionId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return
	}
	bID := strings.TrimSpace(string(benchmarkId))
	if bID == "" {
		writeAuthError(w, http.StatusBadRequest, "invalid benchmarkId")
		return
	}
	if _, _, _, ok := s.authorizeCollection(w, r, cID, "stig-manager:collection:read", RoleRestricted); !ok {
		return
	}
	if s.Checklists == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	rows, err := s.Checklists.CollectionSummary(r.Context(), cID, bID, strings.TrimSpace(string(revisionStr)))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "benchmark or revision not found")
			return
		}
		s.logErr(r, "collection checklist summary", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to summarise checklist")
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{
			"groupId":      row.GroupID,
			"groupTitle":   row.GroupTitle,
			"ruleId":       row.RuleID,
			"ruleTitle":    row.RuleTitle,
			"version":      row.VersionStr,
			"severity":     row.Severity,
			"pass":         row.PassCount,
			"fail":         row.FailCount,
			"notapplicable": row.NotApplicableCount,
			"other":        row.OtherCount,
			"saved":        row.SavedCount,
			"submitted":    row.SubmittedCount,
			"accepted":     row.AcceptedCount,
			"rejected":     row.RejectedCount,
			"minTs":        row.MinTs,
			"maxTs":        row.MaxTs,
			"minTouchTs":   row.MinTouchTs,
			"maxTouchTs":   row.MaxTouchTs,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// PostCklArchiveByCollection streams a ZIP archive of CKL files for the
// supplied AssetStigSelection list. The implementation accepts the
// mode query parameter for API compatibility but always emits one CKL
// per (asset, benchmark) pair (mono behaviour).
func (s APIServer) PostCklArchiveByCollection(
	w http.ResponseWriter, r *http.Request,
	collectionId api.CollectionIdPath, _ api.PostCklArchiveByCollectionParams,
) {
	s.streamArchive(w, r, collectionId, "ckl")
}

// PostCklbArchiveByCollection streams a ZIP archive of CKLB files.
func (s APIServer) PostCklbArchiveByCollection(
	w http.ResponseWriter, r *http.Request,
	collectionId api.CollectionIdPath, _ api.PostCklbArchiveByCollectionParams,
) {
	s.streamArchive(w, r, collectionId, "cklb")
}

// PostXccdfArchiveByCollection streams a ZIP archive of XCCDF results.
func (s APIServer) PostXccdfArchiveByCollection(
	w http.ResponseWriter, r *http.Request,
	collectionId api.CollectionIdPath,
) {
	s.streamArchive(w, r, collectionId, "xccdf")
}

// streamArchive resolves an AssetStigSelection request body and emits
// a ZIP containing one checklist file per (asset, benchmark) pair.
func (s APIServer) streamArchive(
	w http.ResponseWriter, r *http.Request,
	collectionId api.CollectionIdPath, format string,
) {
	cID, ok := parseInt64Path(string(collectionId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return
	}
	if _, _, _, ok := s.authorizeCollection(w, r, cID, "stig-manager:collection:read", RoleRestricted); !ok {
		return
	}
	if s.Checklists == nil || s.Assets == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}

	var selections []api.AssetStigSelection
	if err := json.NewDecoder(r.Body).Decode(&selections); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if len(selections) == 0 {
		writeAuthError(w, http.StatusBadRequest, "request must include at least one selection")
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", contentDisposition("collection-"+strconv.FormatInt(cID, 10)+"-"+format+".zip"))

	zw := zip.NewWriter(w)
	defer zw.Close()

	for _, sel := range selections {
		assetID, ok := parseInt64Path(string(sel.AssetId))
		if !ok {
			continue
		}
		asset, err := s.Assets.Get(r.Context(), assetID)
		if err != nil || asset.CollectionID != cID {
			continue
		}

		benchmarks := resolveSelection(sel)
		// If no stigs specified, export every benchmark currently
		// mapped to the asset.
		if len(benchmarks) == 0 {
			data, err := s.Checklists.AssetMulti(r.Context(), assetID, nil)
			if err != nil {
				continue
			}
			for _, st := range data.Stigs {
				if err := writeArchiveEntry(zw, format, asset.Name, ChecklistOneStig(data, st)); err != nil {
					return
				}
			}
			continue
		}

		for _, b := range benchmarks {
			data, err := s.Checklists.AssetSingle(r.Context(), assetID, b.benchmarkID, b.revisionStr)
			if err != nil {
				continue
			}
			if err := writeArchiveEntry(zw, format, asset.Name, data); err != nil {
				return
			}
		}
	}
}

// resolveChecklistContext authorises an asset-scoped checklist request.
// It returns the ChecklistRepo, the asset record (so callers can use
// its name for Content-Disposition filenames), and a sentinel ok bool.
func (s APIServer) resolveChecklistContext(
	w http.ResponseWriter, r *http.Request,
	assetID int64, scope string, minRole int16,
) (*store.ChecklistRepo, store.Asset, bool) {
	if s.Assets == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return nil, store.Asset{}, false
	}
	asset, err := s.Assets.Get(r.Context(), assetID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "asset not found")
			return nil, store.Asset{}, false
		}
		s.logErr(r, "lookup asset for checklist", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to load asset")
		return nil, store.Asset{}, false
	}
	if _, _, _, ok := s.authorizeCollection(w, r, asset.CollectionID, scope, minRole); !ok {
		return nil, store.Asset{}, false
	}
	if s.Checklists == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return nil, store.Asset{}, false
	}
	return s.Checklists, asset, true
}

// selectionEntry is a normalised (benchmark, revision) pair extracted
// from an AssetStigSelection_Stigs_Item union.
type selectionEntry struct {
	benchmarkID string
	revisionStr string
}

func resolveSelection(sel api.AssetStigSelection) []selectionEntry {
	if sel.Stigs == nil {
		return nil
	}
	var out []selectionEntry
	for _, item := range *sel.Stigs {
		if s, err := item.AsString255(); err == nil && strings.TrimSpace(string(s)) != "" {
			out = append(out, selectionEntry{benchmarkID: strings.TrimSpace(string(s))})
			continue
		}
		if rb, err := item.AsRevisionBasic(); err == nil && rb.BenchmarkId != nil && strings.TrimSpace(string(*rb.BenchmarkId)) != "" {
			out = append(out, selectionEntry{
				benchmarkID: strings.TrimSpace(string(*rb.BenchmarkId)),
				revisionStr: strings.TrimSpace(string(rb.RevisionStr)),
			})
		}
	}
	return out
}

// writeArchiveEntry adds one checklist file to the active zip writer.
func writeArchiveEntry(zw *zip.Writer, format, assetName string, data store.AssetChecklist) error {
	if len(data.Stigs) == 0 {
		return nil
	}
	st := data.Stigs[0]
	baseName := assetName + "_" + st.BenchmarkID
	if st.RevisionStr != "" {
		baseName += "-" + st.RevisionStr
	}
	var ext string
	switch format {
	case "cklb":
		ext = ".cklb"
	case "xccdf":
		ext = "-xccdf-results.xml"
	default:
		ext = ".ckl"
	}
	wf, err := zw.Create(baseName + ext)
	if err != nil {
		return fmt.Errorf("zip create: %w", err)
	}
	export := toExportData(data)
	switch format {
	case "cklb":
		return checklist.WriteCKLB(wf, export)
	case "xccdf":
		return checklist.WriteXCCDFResults(wf, export)
	default:
		return checklist.WriteCKL(wf, export)
	}
}

// ChecklistOneStig returns a single-STIG slice of a multi-STIG asset
// checklist payload, preserving the asset header.
func ChecklistOneStig(data store.AssetChecklist, st store.ChecklistStig) store.AssetChecklist {
	return store.AssetChecklist{Asset: data.Asset, Stigs: []store.ChecklistStig{st}}
}

// contentDisposition returns a sanitised Content-Disposition header
// value: attachment; filename="..."
func contentDisposition(name string) string {
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-' || r == '_' || r == '.':
			return r
		}
		return '_'
	}, name)
	return `attachment; filename="` + name + `"`
}

// toExportData adapts the store projection to the writer input format.
func toExportData(data store.AssetChecklist) checklist.ExportData {
	out := checklist.ExportData{
		Asset: checklist.ExportAsset{
			HostName:  data.Asset.Name,
			FQDN:      data.Asset.FQDN,
			IP:        data.Asset.IP,
			MAC:       data.Asset.MAC,
			TargetKey: strconv.FormatInt(data.Asset.AssetID, 10),
		},
	}
	if data.Asset.Noncomputing {
		out.Asset.AssetType = "Non-Computing"
	}
	for _, st := range data.Stigs {
		es := checklist.ExportStig{
			BenchmarkID: st.BenchmarkID,
			RevisionStr: st.RevisionStr,
			Version:     st.Version,
			Release:     st.Release,
			Title:       st.Title,
			Description: st.Description,
			Source:      st.Source,
		}
		if st.ReleaseDate != nil {
			es.ReleaseDate = st.ReleaseDate.Format("2006-01-02")
		}
		for _, ru := range st.Rules {
			er := checklist.ExportRule{
				RuleID:       ru.RuleID,
				VersionStr:   ru.VersionStr,
				GroupID:      ru.GroupID,
				GroupTitle:   ru.GroupTitle,
				Severity:     ru.Severity,
				Weight:       ru.Weight,
				Title:        ru.Title,
				Description:  ru.Description,
				CheckSystem:  ru.CheckSystem,
				CheckContent: ru.CheckContent,
				FixID:        ru.FixID,
				FixText:      ru.FixText,
				CCIs:         ru.CCIs,
				Result:       ru.Result,
				Detail:       ru.Detail,
				Comment:      ru.Comment,
				AutoResult:   ru.AutoResult,
				StatusLabel:  ru.StatusLabel,
				Username:     ru.Username,
			}
			if ru.TS != nil {
				er.TS = ru.TS.UTC().Format("2006-01-02T15:04:05Z")
			}
			if ru.TouchTS != nil {
				er.TouchTS = ru.TouchTS.UTC().Format("2006-01-02T15:04:05Z")
			}
			es.Rules = append(es.Rules, er)
		}
		out.Stigs = append(out.Stigs, es)
	}
	return out
}

// jsonAssetChecklist returns the JSON projection for the single-STIG
// asset checklist endpoint: per-rule summary including review state.
func jsonAssetChecklist(data store.AssetChecklist) []map[string]any {
	out := []map[string]any{}
	if len(data.Stigs) == 0 {
		return out
	}
	st := data.Stigs[0]
	for _, r := range st.Rules {
		ts := ""
		if r.TS != nil {
			ts = r.TS.UTC().Format("2006-01-02T15:04:05Z")
		}
		out = append(out, map[string]any{
			"ruleId":      r.RuleID,
			"groupId":     r.GroupID,
			"groupTitle":  r.GroupTitle,
			"ruleTitle":   r.Title,
			"version":     r.VersionStr,
			"severity":    r.Severity,
			"result":      r.Result,
			"detail":      r.Detail,
			"comment":     r.Comment,
			"autoResult":  r.AutoResult,
			"status":      r.StatusLabel,
			"ts":          ts,
			"username":    r.Username,
			"ccis":        r.CCIs,
		})
	}
	return out
}

// io.Copy keeper to ensure import is referenced when handlers are
// extended in follow-up milestones.
var _ = io.Copy
