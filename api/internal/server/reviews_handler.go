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

// GetReviewsByCollection lists Reviews in the given collection,
// optionally filtered by rule, asset, status, or result.
//
// Per-rule ACL filtering (per-grant `acl`) lands in Milestone 9. This
// milestone gates reads on the collection role only: Restricted+
// callers see every Review in the collection.
func (s APIServer) GetReviewsByCollection(
	w http.ResponseWriter, r *http.Request,
	collectionId api.CollectionIdPath, params api.GetReviewsByCollectionParams,
) {
	collID, ok := parseInt64Path(string(collectionId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return
	}
	if _, _, _, ok := s.authorizeCollection(w, r, collID, "stig-manager:collection:read", RoleRestricted); !ok {
		return
	}
	if s.Reviews == nil {
		writeJSON(w, http.StatusOK, []api.ReviewAssetRuleRead{})
		return
	}

	opts := store.ListReviewsOptions{CollectionID: collID}
	if params.RuleId != nil {
		opts.RuleID = string(*params.RuleId)
	}
	if params.Result != nil {
		opts.Result = string(*params.Result)
	}
	if params.Status != nil {
		opts.Status = string(*params.Status)
	}
	if params.AssetId != nil {
		if id, err := strconv.ParseInt(string(*params.AssetId), 10, 64); err == nil && id > 0 {
			opts.AssetID = id
		}
	}

	rows, err := s.Reviews.List(r.Context(), opts)
	if err != nil {
		s.logErr(r, "list reviews", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to list reviews")
		return
	}
	out := make([]api.ReviewAssetRuleRead, 0, len(rows))
	for _, row := range rows {
		out = append(out, reviewToRead(row, nil))
	}
	writeJSON(w, http.StatusOK, out)
}

// GetReviewsByAsset returns Reviews for a single Asset in a Collection.
func (s APIServer) GetReviewsByAsset(
	w http.ResponseWriter, r *http.Request,
	collectionId api.CollectionIdPath, assetId api.AssetIdPath, params api.GetReviewsByAssetParams,
) {
	collID, ok := parseInt64Path(string(collectionId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return
	}
	aID, ok := parseInt64Path(string(assetId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid assetId")
		return
	}
	if _, _, _, ok := s.authorizeCollection(w, r, collID, "stig-manager:collection:read", RoleRestricted); !ok {
		return
	}
	if s.Reviews == nil {
		writeJSON(w, http.StatusOK, []api.ReviewAssetRuleRead{})
		return
	}
	rows, err := s.Reviews.List(r.Context(), store.ListReviewsOptions{
		CollectionID: collID, AssetID: aID,
		Result: deref(params.Result), Status: deref(params.Status),
	})
	if err != nil {
		s.logErr(r, "list reviews by asset", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to list reviews")
		return
	}
	out := make([]api.ReviewAssetRuleRead, 0, len(rows))
	for _, row := range rows {
		out = append(out, reviewToRead(row, nil))
	}
	writeJSON(w, http.StatusOK, out)
}

// GetReviewByAssetRule returns the single Review for (collectionId,
// assetId, ruleId).
func (s APIServer) GetReviewByAssetRule(
	w http.ResponseWriter, r *http.Request,
	collectionId api.CollectionIdPath, assetId api.AssetIdPath, ruleId api.RuleIdPath,
	params api.GetReviewByAssetRuleParams,
) {
	collID, ok := parseInt64Path(string(collectionId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return
	}
	aID, ok := parseInt64Path(string(assetId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid assetId")
		return
	}
	rID := string(ruleId)
	if rID == "" {
		writeAuthError(w, http.StatusBadRequest, "invalid ruleId")
		return
	}
	if _, _, _, ok := s.authorizeCollection(w, r, collID, "stig-manager:collection:read", RoleRestricted); !ok {
		return
	}
	if s.Reviews == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}

	rv, err := s.Reviews.Get(r.Context(), aID, rID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "review not found")
			return
		}
		s.logErr(r, "get review", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to get review")
		return
	}

	var hist []api.ReviewHistory
	if wantsProjection(params.Projection, "history") {
		entries, err := s.Reviews.History(r.Context(), aID, rID)
		if err != nil {
			s.logErr(r, "list review history", err)
			writeAuthError(w, http.StatusInternalServerError, "failed to load review history")
			return
		}
		hist = make([]api.ReviewHistory, 0, len(entries))
		for _, h := range entries {
			hist = append(hist, historyToAPI(h))
		}
	}
	writeJSON(w, http.StatusOK, reviewToRead(rv, hist))
}

// PutReviewByAssetRule sets every field of a Review at once. An
// existing row is replaced; a missing row is created.
func (s APIServer) PutReviewByAssetRule(
	w http.ResponseWriter, r *http.Request,
	collectionId api.CollectionIdPath, assetId api.AssetIdPath, ruleId api.RuleIdPath,
	_ api.PutReviewByAssetRuleParams,
) {
	collID, ok := parseInt64Path(string(collectionId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return
	}
	aID, ok := parseInt64Path(string(assetId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid assetId")
		return
	}
	rID := string(ruleId)
	if rID == "" {
		writeAuthError(w, http.StatusBadRequest, "invalid ruleId")
		return
	}
	_, userID, _, ok := s.authorizeCollection(w, r, collID, "stig-manager:collection", RoleManage)
	if !ok {
		return
	}
	if s.Reviews == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	var in api.ReviewAssetRulePut
	if err := json.Unmarshal(body, &in); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	w8, err := putToStoreWrite(in)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Verify the (asset, rule) pair belongs to this collection so a
	// caller cannot write to another collection by manipulating the
	// path.
	if owner, err := s.Reviews.CollectionForAsset(r.Context(), aID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "asset not found")
			return
		}
		s.logErr(r, "lookup asset collection", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to verify asset")
		return
	} else if owner != collID {
		writeAuthError(w, http.StatusNotFound, "asset not found in collection")
		return
	}

	rv, err := s.Reviews.Put(r.Context(), aID, rID, userID, w8)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeAuthError(w, http.StatusBadRequest, "invalid review payload")
			return
		}
		s.logErr(r, "put review", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to put review")
		return
	}
	writeJSON(w, http.StatusOK, reviewToRead(rv, nil))
}

// PatchReviewByAssetRule merges the provided fields with an existing
// Review.
func (s APIServer) PatchReviewByAssetRule(
	w http.ResponseWriter, r *http.Request,
	collectionId api.CollectionIdPath, assetId api.AssetIdPath, ruleId api.RuleIdPath,
	_ api.PatchReviewByAssetRuleParams,
) {
	collID, ok := parseInt64Path(string(collectionId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return
	}
	aID, ok := parseInt64Path(string(assetId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid assetId")
		return
	}
	rID := string(ruleId)
	if rID == "" {
		writeAuthError(w, http.StatusBadRequest, "invalid ruleId")
		return
	}
	_, userID, _, ok := s.authorizeCollection(w, r, collID, "stig-manager:collection", RoleManage)
	if !ok {
		return
	}
	if s.Reviews == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	var in api.ReviewAssetRulePatch
	if err := json.Unmarshal(body, &in); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	patch, err := patchToStorePatch(in)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, err.Error())
		return
	}

	rv, err := s.Reviews.Patch(r.Context(), aID, rID, userID, patch)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "review not found")
			return
		}
		if errors.Is(err, store.ErrConflict) {
			writeAuthError(w, http.StatusBadRequest, "invalid review payload")
			return
		}
		s.logErr(r, "patch review", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to patch review")
		return
	}
	writeJSON(w, http.StatusOK, reviewToRead(rv, nil))
}

// DeleteReviewByAssetRule removes a Review, snapshotting it to
// review_history first.
func (s APIServer) DeleteReviewByAssetRule(
	w http.ResponseWriter, r *http.Request,
	collectionId api.CollectionIdPath, assetId api.AssetIdPath, ruleId api.RuleIdPath,
	_ api.DeleteReviewByAssetRuleParams,
) {
	collID, ok := parseInt64Path(string(collectionId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return
	}
	aID, ok := parseInt64Path(string(assetId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid assetId")
		return
	}
	rID := string(ruleId)
	if rID == "" {
		writeAuthError(w, http.StatusBadRequest, "invalid ruleId")
		return
	}
	if _, _, _, ok := s.authorizeCollection(w, r, collID, "stig-manager:collection", RoleManage); !ok {
		return
	}
	if s.Reviews == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	if err := s.Reviews.Delete(r.Context(), aID, rID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "review not found")
			return
		}
		s.logErr(r, "delete review", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to delete review")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- helpers -------------------------------------------------------------

func deref[T ~string](p *T) string {
	if p == nil {
		return ""
	}
	return string(*p)
}

func wantsProjection(p *api.ReviewProjectionQuery, name string) bool {
	if p == nil {
		return false
	}
	for _, v := range *p {
		if v == name {
			return true
		}
	}
	return false
}

func reviewToRead(row store.Review, history []api.ReviewHistory) api.ReviewAssetRuleRead {
	out := api.ReviewAssetRuleRead{
		Access:     "rw",
		AutoResult: &row.AutoResult,
		Comment:    row.Comment,
		Detail:     row.Detail,
		Result:     api.ReviewResult(row.Result),
		Status:     reviewStatusRead(row),
		TouchTs:    api.StringDateTime(row.TouchTS),
		Ts:         api.StringDateTime(row.TS),
		UserId:     api.UserId(strconv.FormatInt(row.UserID, 10)),
		Username:   api.Username(row.Username),
	}
	if len(row.Metadata) > 0 && string(row.Metadata) != "null" {
		var m api.Metadata
		if err := json.Unmarshal(row.Metadata, &m); err == nil {
			out.Metadata = &m
		}
	}
	if len(row.ResultEngine) > 0 && string(row.ResultEngine) != "null" {
		var re api.ResultEngine
		if err := json.Unmarshal(row.ResultEngine, &re); err == nil {
			out.ResultEngine = &re
		}
	}
	if history != nil {
		out.History = &history
	}
	return out
}

func reviewStatusRead(row store.Review) api.ReviewStatusRead {
	statusUser := api.UserBasic{
		UserId:   api.UserId(strconv.FormatInt(row.UserID, 10)),
		Username: api.Username(row.Username),
	}
	if row.StatusUserID != nil {
		statusUser.UserId = api.UserId(strconv.FormatInt(*row.StatusUserID, 10))
	}
	read := api.ReviewStatusRead{
		Label: api.ReviewStatusLabel(row.StatusLabel),
		Ts:    api.StringDateTime(row.StatusTS),
		User:  statusUser,
	}
	if row.StatusText != "" {
		t := api.StatusText(row.StatusText)
		read.Text = &t
	}
	return read
}

func historyToAPI(h store.ReviewHistoryEntry) api.ReviewHistory {
	rid := api.RuleId(h.RuleID)
	out := api.ReviewHistory{
		AutoResult: &h.AutoResult,
		Comment:    h.Comment,
		Detail:     h.Detail,
		Result:     api.ReviewResult(h.Result),
		RuleId:     &rid,
		Status: api.ReviewStatusRead{
			Label: api.ReviewStatusLabel(h.StatusLabel),
			Ts:    api.StringDateTime(h.TouchTS),
			User: api.UserBasic{
				UserId:   api.UserId(strconv.FormatInt(h.UserID, 10)),
				Username: api.Username(h.Username),
			},
		},
		TouchTs:  api.StringDateTime(h.TouchTS),
		Ts:       api.StringDateTime(h.TS),
		UserId:   api.UserId(strconv.FormatInt(h.UserID, 10)),
		Username: api.Username(h.Username),
	}
	if h.StatusText != "" {
		t := api.StatusText(h.StatusText)
		out.Status.Text = &t
	}
	if len(h.ResultEngine) > 0 && string(h.ResultEngine) != "null" {
		var re api.ResultEngine
		if err := json.Unmarshal(h.ResultEngine, &re); err == nil {
			out.ResultEngine = &re
		}
	}
	return out
}

// putToStoreWrite converts the OpenAPI PUT payload into a store-layer
// write bundle. Returns an error when the status union is malformed.
func putToStoreWrite(in api.ReviewAssetRulePut) (store.ReviewWrite, error) {
	out := store.ReviewWrite{
		Result: string(in.Result),
	}
	if in.AutoResult != nil {
		out.AutoResult = *in.AutoResult
	}
	if in.Comment != nil {
		out.Comment = string(*in.Comment)
	}
	if in.Detail != nil {
		out.Detail = string(*in.Detail)
	}
	if in.Metadata != nil {
		b, err := json.Marshal(in.Metadata)
		if err != nil {
			return out, errors.New("invalid metadata")
		}
		out.Metadata = b
	}
	if in.ResultEngine != nil {
		b, err := json.Marshal(in.ResultEngine)
		if err != nil {
			return out, errors.New("invalid resultEngine")
		}
		out.ResultEngine = b
	}
	if in.Status != nil {
		label, text, err := unpackStatusWrite(*in.Status)
		if err != nil {
			return out, err
		}
		out.StatusLabel = label
		out.StatusText = text
	}
	return out, nil
}

// patchToStorePatch converts the OpenAPI PATCH payload into a
// store-layer merge bundle. Nil OpenAPI fields stay nil in the store
// patch (= leave-untouched).
func patchToStorePatch(in api.ReviewAssetRulePatch) (store.ReviewPatch, error) {
	out := store.ReviewPatch{}
	if in.Result != nil {
		s := string(*in.Result)
		out.Result = &s
	}
	if in.Comment != nil {
		s := string(*in.Comment)
		out.Comment = &s
	}
	if in.Detail != nil {
		s := string(*in.Detail)
		out.Detail = &s
	}
	if in.Metadata != nil {
		b, err := json.Marshal(in.Metadata)
		if err != nil {
			return out, errors.New("invalid metadata")
		}
		out.Metadata = b
	}
	if in.ResultEngine != nil {
		b, err := json.Marshal(in.ResultEngine)
		if err != nil {
			return out, errors.New("invalid resultEngine")
		}
		out.ResultEngine = b
	}
	if in.Status != nil {
		label, text, err := unpackStatusWrite(*in.Status)
		if err != nil {
			return out, err
		}
		out.StatusLabel = &label
		out.StatusText = &text
	}
	return out, nil
}

// unpackStatusWrite handles the ReviewStatusWrite oneOf: either a bare
// label string ("submitted") or a struct {label, text}.
func unpackStatusWrite(s api.ReviewStatusWrite) (string, string, error) {
	if label, err := s.AsReviewStatusLabel(); err == nil && label != "" {
		return string(label), "", nil
	}
	full, err := s.AsReviewStatusWrite1()
	if err != nil {
		return "", "", errors.New("invalid status payload")
	}
	text := ""
	if full.Text != nil {
		text = string(*full.Text)
	}
	return string(full.Label), text, nil
}
