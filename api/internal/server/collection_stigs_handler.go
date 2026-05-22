package server

import (
	"errors"
	"net/http"

	"github.com/Exonical/stig-manager-react/api/internal/api"
	"github.com/Exonical/stig-manager-react/api/internal/store"
)

// GetStigsByCollection returns each STIG mapped (via at least one
// asset) into the collection, together with the count of distinct
// assets in the collection that reference it. Mirrors upstream's
// CollectionStigWithAssetCount[] response.
func (s APIServer) GetStigsByCollection(
	w http.ResponseWriter, r *http.Request,
	collectionId api.CollectionIdPath, _ api.GetStigsByCollectionParams,
) {
	collID, ok := parseInt64Path(string(collectionId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return
	}
	if _, _, _, ok := s.authorizeCollection(w, r, collID, "stig-manager:collection:read", RoleRestricted); !ok {
		return
	}
	if s.Stigs == nil {
		writeJSON(w, http.StatusOK, []api.CollectionStigWithAssetCount{})
		return
	}
	rows, err := s.Stigs.ListByCollection(r.Context(), collID)
	if err != nil {
		s.logErr(r, "list stigs by collection", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to list collection stigs")
		return
	}
	out := make([]api.CollectionStigWithAssetCount, 0, len(rows))
	for _, row := range rows {
		out = append(out, collectionStigToAPI(row))
	}
	writeJSON(w, http.StatusOK, out)
}

// GetStigByCollection returns a single STIG mapped into the collection.
// 204 No Content is returned when no asset in the collection
// references the benchmark, matching upstream behaviour.
func (s APIServer) GetStigByCollection(
	w http.ResponseWriter, r *http.Request,
	collectionId api.CollectionIdPath, benchmarkId api.BenchmarkIdPath, _ api.GetStigByCollectionParams,
) {
	collID, ok := parseInt64Path(string(collectionId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return
	}
	if _, _, _, ok := s.authorizeCollection(w, r, collID, "stig-manager:collection:read", RoleRestricted); !ok {
		return
	}
	if s.Stigs == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	row, err := s.Stigs.GetByCollection(r.Context(), collID, string(benchmarkId))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		s.logErr(r, "get stig by collection", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to get collection stig")
		return
	}
	writeJSON(w, http.StatusOK, collectionStigToAPI(*row))
}

func collectionStigToAPI(row store.CollectionSTIG) api.CollectionStigWithAssetCount {
	bid := api.BenchmarkId(row.BenchmarkID)
	count := api.AssetCount(row.AssetCount)
	rc := api.RuleCount(row.RuleCount)
	out := api.CollectionStigWithAssetCount{
		BenchmarkId:    &bid,
		AssetCount:     count,
		RevisionStr:    api.RevisionStr(row.RevisionStr),
		RevisionPinned: row.RevisionPinned,
		RuleCount:      &rc,
	}
	if row.BenchmarkDate != nil {
		nullable := api.StringDateNullable{Time: *row.BenchmarkDate}
		out.BenchmarkDate = &nullable
	}
	if row.Title != "" {
		title := api.StatusText(row.Title)
		out.Title = &title
	}
	return out
}
