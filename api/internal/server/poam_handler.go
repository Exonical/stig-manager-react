package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Exonical/stig-manager-react/api/internal/api"
	"github.com/Exonical/stig-manager-react/api/internal/poam"
	"github.com/Exonical/stig-manager-react/api/internal/store"
)

// GetPoamByCollection produces a POA&M xlsx workbook for the
// collection's open ("fail") findings. The aggregator query param
// flips the row grouping between Group (V-…) and Rule (SV-…) level.
//
// Required scope: stig-manager:collection:read with at least
// AccessLevelRestricted on the collection.
func (s APIServer) GetPoamByCollection(
	w http.ResponseWriter, r *http.Request,
	collectionId api.CollectionIdPath, params api.GetPoamByCollectionParams,
) {
	if !params.Aggregator.Valid() {
		writeAuthError(w, http.StatusBadRequest, "invalid aggregator")
		return
	}
	collID, ok := parseInt64Path(string(collectionId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return
	}
	if _, _, _, ok := s.authorizeCollection(w, r, collID, "stig-manager:collection:read", RoleRestricted); !ok {
		return
	}
	if s.Poam == nil || s.Collections == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "poam export unavailable")
		return
	}

	filter := store.PoamFilter{
		Aggregator: store.PoamAggregator(string(params.Aggregator)),
	}
	if params.AcceptedOnly != nil {
		filter.AcceptedOnly = *params.AcceptedOnly
	}
	if params.BenchmarkId != nil {
		if id := strings.TrimSpace(string(*params.BenchmarkId)); id != "" {
			filter.BenchmarkIDs = append(filter.BenchmarkIDs, id)
		}
	}
	if params.AssetId != nil {
		if id, err := strconv.ParseInt(string(*params.AssetId), 10, 64); err == nil && id > 0 {
			filter.AssetIDs = append(filter.AssetIDs, id)
		}
	}

	findings, err := s.Poam.GetFindings(r.Context(), collID, filter)
	if err != nil {
		s.logErr(r, "poam findings", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to compute findings")
		return
	}

	defaults := poam.Defaults{}
	if params.Date != nil {
		defaults.Date = *params.Date
	}
	if params.Office != nil {
		defaults.Office = *params.Office
	}
	if params.Status != nil {
		defaults.Status = *params.Status
	}
	if params.MccastPackageId != nil {
		defaults.MccastPackageID = *params.MccastPackageId
	}
	if params.MccastAuthName != nil {
		defaults.MccastAuthName = *params.MccastAuthName
	}

	format := poam.FormatEMASS
	if params.Format != nil && *params.Format == api.GetPoamByCollectionParamsFormatMCCAST {
		format = poam.FormatMCCAST
	}

	blob, err := poam.Write(findings, format, defaults)
	if err != nil {
		s.logErr(r, "poam write", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to write poam workbook")
		return
	}

	// Look up the collection name for the suggested filename.
	collName := "collection"
	if col, err := s.Collections.Get(r.Context(), collID); err == nil {
		collName = col.Name
	}
	filename := poam.SuggestedFilename(collName, format)

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(blob)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(blob)
}
