package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/Exonical/stig-manager-react/api/internal/api"
	"github.com/Exonical/stig-manager-react/api/internal/store"
)

// GetCollectionLabels returns every label in the collection.
func (s APIServer) GetCollectionLabels(w http.ResponseWriter, r *http.Request, collectionId api.CollectionIdPath) {
	collID, ok := parseInt64Path(string(collectionId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return
	}
	if _, _, _, ok := s.authorizeCollection(w, r, collID, "stig-manager:collection:read", RoleRestricted); !ok {
		return
	}
	if s.Labels == nil {
		writeJSON(w, http.StatusOK, []api.Label{})
		return
	}
	rows, err := s.Labels.List(r.Context(), collID)
	if err != nil {
		s.logErr(r, "list labels", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to list labels")
		return
	}
	out := make([]api.Label, 0, len(rows))
	for _, l := range rows {
		out = append(out, storeToAPILabel(l))
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateCollectionLabel creates a single label in the collection.
func (s APIServer) CreateCollectionLabel(w http.ResponseWriter, r *http.Request, collectionId api.CollectionIdPath) {
	collID, ok := parseInt64Path(string(collectionId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return
	}
	if _, _, _, ok := s.authorizeCollection(w, r, collID, "stig-manager:collection", RoleManage); !ok {
		return
	}
	if s.Labels == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	var in api.LabelCreate
	if err := json.Unmarshal(body, &in); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	created, err := s.Labels.Create(r.Context(), store.LabelCreate{
		CollectionID: collID,
		Name:         string(in.Name),
		Color:        string(in.Color),
		Description:  derefStr((*string)(in.Description)),
	})
	if err != nil {
		s.handleLabelWriteErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, storeToAPILabel(created))
}

// CreateCollectionLabels creates a batch of labels in the collection.
func (s APIServer) CreateCollectionLabels(w http.ResponseWriter, r *http.Request, collectionId api.CollectionIdPath) {
	collID, ok := parseInt64Path(string(collectionId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return
	}
	if _, _, _, ok := s.authorizeCollection(w, r, collID, "stig-manager:collection", RoleManage); !ok {
		return
	}
	if s.Labels == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	var in []api.LabelCreate
	if err := json.Unmarshal(body, &in); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	out := make([]api.Label, 0, len(in))
	for i, item := range in {
		created, err := s.Labels.Create(r.Context(), store.LabelCreate{
			CollectionID: collID,
			Name:         string(item.Name),
			Color:        string(item.Color),
			Description:  derefStr((*string)(item.Description)),
		})
		if err != nil {
			s.handleLabelWriteErr(w, r, errAtAsset(i, err))
			return
		}
		out = append(out, storeToAPILabel(created))
	}
	writeJSON(w, http.StatusCreated, out)
}

// GetCollectionLabelById returns a single label.
func (s APIServer) GetCollectionLabelById(w http.ResponseWriter, r *http.Request, collectionId api.CollectionIdPath, labelId api.LabelIdPath) {
	collID, ok := parseInt64Path(string(collectionId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return
	}
	if _, _, _, ok := s.authorizeCollection(w, r, collID, "stig-manager:collection:read", RoleRestricted); !ok {
		return
	}
	if s.Labels == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	row, err := s.Labels.Get(r.Context(), collID, string(labelId))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "label not found")
			return
		}
		s.logErr(r, "get label", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to get label")
		return
	}
	writeJSON(w, http.StatusOK, storeToAPILabel(row))
}

// PatchCollectionLabelById merges fields onto an existing label.
func (s APIServer) PatchCollectionLabelById(w http.ResponseWriter, r *http.Request, collectionId api.CollectionIdPath, labelId api.LabelIdPath) {
	collID, ok := parseInt64Path(string(collectionId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return
	}
	if _, _, _, ok := s.authorizeCollection(w, r, collID, "stig-manager:collection", RoleManage); !ok {
		return
	}
	if s.Labels == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	var in api.LabelUpdate
	if err := json.Unmarshal(body, &in); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	upd := store.LabelUpdate{}
	if in.Name != nil {
		v := string(*in.Name)
		upd.Name = &v
	}
	if in.Color != nil {
		v := string(*in.Color)
		upd.Color = &v
	}
	if in.Description != nil {
		v := string(*in.Description)
		upd.Description = &v
	}
	row, err := s.Labels.Patch(r.Context(), collID, string(labelId), upd)
	if err != nil {
		s.handleLabelWriteErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, storeToAPILabel(row))
}

// DeleteCollectionLabelById removes a label.
func (s APIServer) DeleteCollectionLabelById(w http.ResponseWriter, r *http.Request, collectionId api.CollectionIdPath, labelId api.LabelIdPath) {
	collID, ok := parseInt64Path(string(collectionId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return
	}
	if _, _, _, ok := s.authorizeCollection(w, r, collID, "stig-manager:collection", RoleManage); !ok {
		return
	}
	if s.Labels == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	if err := s.Labels.Delete(r.Context(), collID, string(labelId)); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "label not found")
			return
		}
		s.logErr(r, "delete label", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to delete label")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetAssetsByCollectionLabelId returns assets mapped to the label.
func (s APIServer) GetAssetsByCollectionLabelId(w http.ResponseWriter, r *http.Request, collectionId api.CollectionIdPath, labelId api.LabelIdPath) {
	collID, ok := parseInt64Path(string(collectionId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return
	}
	if _, _, _, ok := s.authorizeCollection(w, r, collID, "stig-manager:collection:read", RoleRestricted); !ok {
		return
	}
	if s.Labels == nil || s.Assets == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	ids, err := s.Labels.AssetIDs(r.Context(), collID, string(labelId))
	if err != nil {
		s.logErr(r, "list label assets", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to list label assets")
		return
	}
	out := make([]api.AssetProjected, 0, len(ids))
	for _, id := range ids {
		row, err := s.Assets.Get(r.Context(), id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				continue
			}
			s.logErr(r, "get label asset", err)
			writeAuthError(w, http.StatusInternalServerError, "failed to load asset")
			return
		}
		proj, err := s.projectAsset(r.Context(), row, nil)
		if err != nil {
			s.logErr(r, "project label asset", err)
			writeAuthError(w, http.StatusInternalServerError, "failed to project asset")
			return
		}
		out = append(out, proj)
	}
	writeJSON(w, http.StatusOK, out)
}

// PutAssetsByCollectionLabelId replaces a label's asset mappings.
func (s APIServer) PutAssetsByCollectionLabelId(w http.ResponseWriter, r *http.Request, collectionId api.CollectionIdPath, labelId api.LabelIdPath) {
	collID, ok := parseInt64Path(string(collectionId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return
	}
	if _, _, _, ok := s.authorizeCollection(w, r, collID, "stig-manager:collection", RoleManage); !ok {
		return
	}
	if s.Labels == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	var assetIDs []api.AssetId
	if err := json.Unmarshal(body, &assetIDs); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	ids := make([]int64, 0, len(assetIDs))
	for _, raw := range assetIDs {
		id, ok := parseInt64Path(string(raw))
		if !ok {
			writeAuthError(w, http.StatusBadRequest, "invalid assetId: "+string(raw))
			return
		}
		ids = append(ids, id)
	}
	if err := s.Labels.SetAssets(r.Context(), collID, string(labelId), ids); err != nil {
		s.handleLabelWriteErr(w, r, err)
		return
	}
	s.GetAssetsByCollectionLabelId(w, r, collectionId, labelId)
}

// --- helpers -------------------------------------------------------------

func storeToAPILabel(row store.Label) api.Label {
	out := api.Label{
		LabelId: api.LabelId(row.LabelID),
		Name:    api.LabelName(row.Name),
		Color:   api.StringHexColor(row.Color),
		Uses:    row.Uses,
	}
	if row.Description != "" {
		desc := api.String255Nullable(row.Description)
		out.Description = &desc
	}
	return out
}

func (s APIServer) handleLabelWriteErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrDuplicateName):
		writeAuthError(w, http.StatusBadRequest, "duplicate label name")
	case errors.Is(err, store.ErrConflict):
		writeAuthError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, store.ErrNotFound):
		writeAuthError(w, http.StatusNotFound, "label not found")
	default:
		s.logErr(r, "write label", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to write label")
	}
}

// strconv is used (silence unused-import lint when build trims).
var _ = strconv.Itoa
