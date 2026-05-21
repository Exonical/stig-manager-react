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

// PostReviewsByAsset bulk-inserts or updates one or more Reviews on a
// single Asset. Each item in the JSON array body is a ReviewAssetPost
// (per the OpenAPI spec).
//
// Per upstream's behaviour, the response carries counts of inserted vs
// updated Reviews plus a list of rejected items (e.g. items missing a
// ruleId). The semantic gates from the spec (collection settings
// status.resetCriteria, per-rule ACLs) land in later milestones; M8
// implements the upsert plumbing only.
func (s APIServer) PostReviewsByAsset(
	w http.ResponseWriter, r *http.Request,
	collectionId api.CollectionIdPath, assetId api.AssetIdPath,
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
	_, userID, _, ok := s.authorizeCollection(w, r, collID, "stig-manager:collection", RoleManage)
	if !ok {
		return
	}
	if s.Reviews == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}

	owner, err := s.Reviews.CollectionForAsset(r.Context(), aID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "asset not found")
			return
		}
		s.logErr(r, "lookup asset collection", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to verify asset")
		return
	}
	if owner != collID {
		writeAuthError(w, http.StatusNotFound, "asset not found in collection")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	var posts []api.ReviewAssetPost
	if err := json.Unmarshal(body, &posts); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	resp := newReviewPostResponse()
	var inserted, updated float32
	for _, p := range posts {
		if p.RuleId == nil || string(*p.RuleId) == "" {
			appendRejected(&resp, nil, "missing ruleId")
			continue
		}
		rID := string(*p.RuleId)
		w8, err := postToStoreWrite(p)
		if err != nil {
			appendRejected(&resp, p.RuleId, err.Error())
			continue
		}
		existed, err := s.Reviews.Exists(r.Context(), aID, rID)
		if err != nil {
			s.logErr(r, "exists review", err)
			appendRejected(&resp, p.RuleId, "internal error")
			continue
		}
		if _, err := s.Reviews.Put(r.Context(), aID, rID, userID, w8); err != nil {
			reason := err.Error()
			if errors.Is(err, store.ErrConflict) {
				reason = "invalid review payload"
			}
			appendRejected(&resp, p.RuleId, reason)
			continue
		}
		if existed {
			updated++
		} else {
			inserted++
		}
	}
	resp.Affected.Inserted = &inserted
	resp.Affected.Updated = &updated
	writeJSON(w, http.StatusOK, resp)
}

// newReviewPostResponse returns a ReviewPostResponse with a non-nil
// (but empty) Rejected slice so JSON output never includes a `null`
// rejected list.
func newReviewPostResponse() api.ReviewPostResponse {
	return api.ReviewPostResponse{
		Rejected: []struct {
			Reason *api.String255 `json:"reason,omitempty"`
			RuleId *api.RuleId    `json:"ruleId,omitempty"`
		}{},
	}
}

func appendRejected(resp *api.ReviewPostResponse, ruleID *api.RuleId, reason string) {
	rs := api.String255(reason)
	resp.Rejected = append(resp.Rejected, struct {
		Reason *api.String255 `json:"reason,omitempty"`
		RuleId *api.RuleId    `json:"ruleId,omitempty"`
	}{Reason: &rs, RuleId: ruleID})
}

// postToStoreWrite converts a single ReviewAssetPost (the JSON shape
// accepted by POST /reviews/{assetId}) into the store-layer write
// bundle used by ReviewRepo.Put.
func postToStoreWrite(p api.ReviewAssetPost) (store.ReviewWrite, error) {
	out := store.ReviewWrite{Result: string(p.Result)}
	if p.AutoResult != nil {
		out.AutoResult = *p.AutoResult
	}
	if p.Comment != nil {
		out.Comment = string(*p.Comment)
	}
	if p.Detail != nil {
		out.Detail = string(*p.Detail)
	}
	if p.Metadata != nil {
		b, err := json.Marshal(p.Metadata)
		if err != nil {
			return out, errors.New("invalid metadata")
		}
		out.Metadata = b
	}
	if p.ResultEngine != nil {
		b, err := json.Marshal(p.ResultEngine)
		if err != nil {
			return out, errors.New("invalid resultEngine")
		}
		out.ResultEngine = b
	}
	if p.Status != nil {
		label, text, err := unpackStatusWrite(*p.Status)
		if err != nil {
			return out, err
		}
		out.StatusLabel = label
		out.StatusText = text
	}
	return out, nil
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

// GetReviewHistoryByCollection returns the cross-asset history for
// every review in a collection that matches the supplied filters.
// Records are returned grouped by asset then rule, ordered newest-
// first per rule — the upstream ReviewHistoryAsset/Rule shape.
func (s APIServer) GetReviewHistoryByCollection(
	w http.ResponseWriter, r *http.Request,
	collectionId api.CollectionIdPath, params api.GetReviewHistoryByCollectionParams,
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
		writeJSON(w, http.StatusOK, []api.ReviewHistoryAsset{})
		return
	}
	opt := store.HistoryByCollectionOptions{CollectionID: collID}
	if params.AssetId != nil {
		if id, err := strconv.ParseInt(string(*params.AssetId), 10, 64); err == nil && id > 0 {
			opt.AssetID = id
		}
	}
	if params.RuleId != nil {
		opt.RuleID = string(*params.RuleId)
	}
	if params.Status != nil {
		opt.Status = string(*params.Status)
	}
	if params.StartDate != nil {
		t := params.StartDate.Time
		opt.StartDate = &t
	}
	if params.EndDate != nil {
		t := params.EndDate.Time
		opt.EndDate = &t
	}

	rows, err := s.Reviews.HistoryByCollection(r.Context(), opt)
	if err != nil {
		s.logErr(r, "list review history by collection", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to list review history")
		return
	}
	writeJSON(w, http.StatusOK, historyEntriesToAssets(rows))
}

// GetReviewHistoryStatsByCollection returns aggregate counts of
// history entries in the collection plus the oldest entry's
// timestamp. When projection=asset is requested, the response also
// includes a per-asset breakdown.
func (s APIServer) GetReviewHistoryStatsByCollection(
	w http.ResponseWriter, r *http.Request,
	collectionId api.CollectionIdPath, params api.GetReviewHistoryStatsByCollectionParams,
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
		writeJSON(w, http.StatusOK, api.ReviewHistoryStats{})
		return
	}

	opt := store.HistoryByCollectionOptions{CollectionID: collID}
	if params.AssetId != nil {
		if id, err := strconv.ParseInt(string(*params.AssetId), 10, 64); err == nil && id > 0 {
			opt.AssetID = id
		}
	}
	if params.RuleId != nil {
		opt.RuleID = string(*params.RuleId)
	}
	if params.Status != nil {
		opt.Status = string(*params.Status)
	}
	if params.StartDate != nil {
		t := params.StartDate.Time
		opt.StartDate = &t
	}
	if params.EndDate != nil {
		t := params.EndDate.Time
		opt.EndDate = &t
	}
	wantAssets := false
	if params.Projection != nil {
		for _, p := range *params.Projection {
			if p == "asset" {
				wantAssets = true
				break
			}
		}
	}

	stats, err := s.Reviews.HistoryStatsByCollection(r.Context(), opt, wantAssets)
	if err != nil {
		s.logErr(r, "stat review history", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to stat review history")
		return
	}
	out := api.ReviewHistoryStats{
		CollectionHistoryEntryCount: stats.CollectionEntries,
		OldestHistoryEntryDate:      api.StringDateTime(stats.OldestEntry),
	}
	if wantAssets {
		assets := make([]api.ReviewHistoryStatsAsset, 0, len(stats.PerAsset))
		for _, a := range stats.PerAsset {
			row := api.ReviewHistoryStatsAsset{
				AssetId:           api.String255(a.AssetName),
				HistoryEntryCount: a.EntryCount,
			}
			if a.OldestEntry != nil {
				ts := a.OldestEntry.UTC().Format("2006-01-02T15:04:05Z07:00")
				row.OldestHistoryEntry = &ts
			}
			assets = append(assets, row)
		}
		out.AssetHistoryEntryCounts = &assets
	}
	writeJSON(w, http.StatusOK, out)
}

// DeleteReviewHistoryByCollection bulk-deletes review_history rows
// older than the requested retention date. Manage role is required
// because this is a destructive operation. Returns the number of
// rows removed.
func (s APIServer) DeleteReviewHistoryByCollection(
	w http.ResponseWriter, r *http.Request,
	collectionId api.CollectionIdPath, params api.DeleteReviewHistoryByCollectionParams,
) {
	collID, ok := parseInt64Path(string(collectionId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return
	}
	if _, _, _, ok := s.authorizeCollection(w, r, collID, "stig-manager:collection", RoleManage); !ok {
		return
	}
	if s.Reviews == nil {
		writeJSON(w, http.StatusOK, api.ReviewHistoryDeleted{HistoryEntriesDeleted: 0})
		return
	}

	var assetID int64
	if params.AssetId != nil {
		if id, err := strconv.ParseInt(string(*params.AssetId), 10, 64); err == nil && id > 0 {
			assetID = id
		}
	}

	deleted, err := s.Reviews.DeleteHistoryByCollection(r.Context(), collID, params.RetentionDate.Time, assetID)
	if err != nil {
		s.logErr(r, "delete review history", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to delete review history")
		return
	}
	writeJSON(w, http.StatusOK, api.ReviewHistoryDeleted{HistoryEntriesDeleted: int(deleted)})
}

// historyEntriesToAssets groups a flat list of history entries (asset
// ASC, rule ASC, touch_ts DESC) into the upstream
// ReviewHistoryAsset/Rule nested shape.
func historyEntriesToAssets(entries []store.ReviewHistoryEntry) []api.ReviewHistoryAsset {
	out := []api.ReviewHistoryAsset{}
	if len(entries) == 0 {
		return out
	}
	// The store query orders by asset_id ASC, rule_id ASC. Group by
	// streaming through the slice.
	var (
		curAsset  *api.ReviewHistoryAsset
		curRule   *api.ReviewHistoryRule
		curAssetID int64
		curRuleID  string
	)
	flushAsset := func() {
		if curAsset != nil {
			out = append(out, *curAsset)
		}
	}
	for _, h := range entries {
		assetIDStr := strconv.FormatInt(h.AssetID, 10)
		if curAsset == nil || h.AssetID != curAssetID {
			flushAsset()
			curAssetID = h.AssetID
			curAsset = &api.ReviewHistoryAsset{
				AssetId:         api.String255(assetIDStr),
				ReviewHistories: []api.ReviewHistoryRule{},
			}
			curRule = nil
			curRuleID = ""
		}
		if curRule == nil || h.RuleID != curRuleID {
			curAsset.ReviewHistories = append(curAsset.ReviewHistories, api.ReviewHistoryRule{
				RuleId:  ruleIDPtr(h.RuleID),
				History: []api.ReviewHistory{},
			})
			curRule = &curAsset.ReviewHistories[len(curAsset.ReviewHistories)-1]
			curRuleID = h.RuleID
		}
		curRule.History = append(curRule.History, historyToAPI(h))
	}
	flushAsset()
	return out
}

func ruleIDPtr(s string) *api.RuleId {
	rid := api.RuleId(s)
	return &rid
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
