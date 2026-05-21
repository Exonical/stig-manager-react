package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Exonical/stig-manager-react/api/internal/api"
	"github.com/Exonical/stig-manager-react/api/internal/store"
)

// GetAssets lists Assets in the specified Collection. The collectionId
// query parameter is required (the OpenAPI spec marks it so) and is
// used for grant resolution. Per-asset filters supported in this
// milestone: name, benchmarkId, labelId (single value).
func (s APIServer) GetAssets(w http.ResponseWriter, r *http.Request, params api.GetAssetsParams) {
	collID, ok := parseInt64Path(string(params.CollectionId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return
	}
	if _, _, _, ok := s.authorizeCollection(w, r, collID, "stig-manager:collection:read", RoleRestricted); !ok {
		return
	}
	if s.Assets == nil {
		writeJSON(w, http.StatusOK, []api.AssetProjected{})
		return
	}
	opts := store.ListAssetsOptions{CollectionID: collID}
	if params.Name != nil {
		opts.NameContains = string(*params.Name)
	}
	if params.BenchmarkId != nil {
		opts.BenchmarkID = string(*params.BenchmarkId)
	}
	if params.LabelId != nil && len(*params.LabelId) > 0 {
		opts.LabelID = string((*params.LabelId)[0])
	}
	rows, err := s.Assets.List(r.Context(), opts)
	if err != nil {
		s.logErr(r, "list assets", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to list assets")
		return
	}
	out := make([]api.AssetProjected, 0, len(rows))
	for _, row := range rows {
		proj, err := s.projectAsset(r.Context(), row, params.Projection)
		if err != nil {
			s.logErr(r, "decode asset", err)
			writeAuthError(w, http.StatusInternalServerError, "failed to decode asset")
			return
		}
		out = append(out, proj)
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateAsset creates a single Asset inside the Collection identified
// by the request body's `collectionId` field.
func (s APIServer) CreateAsset(w http.ResponseWriter, r *http.Request, params api.CreateAssetParams) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	var in api.AssetCreateOrReplace
	if err := json.Unmarshal(body, &in); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	collID, ok := parseInt64Path(string(in.CollectionId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return
	}
	if _, _, _, ok := s.authorizeCollection(w, r, collID, "stig-manager:collection", RoleManage); !ok {
		return
	}
	if s.Assets == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}

	created, err := s.createOneAsset(r.Context(), collID, in)
	if err != nil {
		s.handleAssetWriteErr(w, r, err)
		return
	}
	out, err := s.projectAsset(r.Context(), created, params.Projection)
	if err != nil {
		s.logErr(r, "decode asset", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to decode asset")
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// CreateAssets is the collection-scoped bulk-create endpoint
// (POST /collections/{cid}/assets). Accepts either an
// AssetCreateOrReplace (single) or an array of AssetBatchItem.
func (s APIServer) CreateAssets(w http.ResponseWriter, r *http.Request, collectionId api.CollectionIdPath, params api.CreateAssetsParams) {
	collID, ok := parseInt64Path(string(collectionId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return
	}
	if _, _, _, ok := s.authorizeCollection(w, r, collID, "stig-manager:collection", RoleManage); !ok {
		return
	}
	if s.Assets == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var rows []store.Asset
	trimmed := strings.TrimLeftFunc(string(body), func(r rune) bool {
		return r == ' ' || r == '\n' || r == '\t' || r == '\r'
	})
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var batch api.AssetCreateBatch
		if err := json.Unmarshal(body, &batch); err != nil {
			writeAuthError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		for i, item := range batch {
			asset, err := s.createOneAsset(r.Context(), collID, batchToCreate(collectionId, item))
			if err != nil {
				s.handleAssetWriteErr(w, r, errAtAsset(i, err))
				return
			}
			rows = append(rows, asset)
		}
	} else {
		var in api.AssetCreateOrReplace
		if err := json.Unmarshal(body, &in); err != nil {
			writeAuthError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		// Override the body's collectionId with the path's.
		in.CollectionId = collectionId
		asset, err := s.createOneAsset(r.Context(), collID, in)
		if err != nil {
			s.handleAssetWriteErr(w, r, err)
			return
		}
		rows = []store.Asset{asset}
	}

	out := make([]api.AssetProjected, 0, len(rows))
	for _, a := range rows {
		proj, err := s.projectAsset(r.Context(), a, params.Projection)
		if err != nil {
			s.logErr(r, "decode asset", err)
			writeAuthError(w, http.StatusInternalServerError, "failed to decode asset")
			return
		}
		out = append(out, proj)
	}
	writeJSON(w, http.StatusCreated, out)
}

// GetAsset returns a single Asset projection.
func (s APIServer) GetAsset(w http.ResponseWriter, r *http.Request, assetId api.AssetIdPath, params api.GetAssetParams) {
	id, ok := parseInt64Path(string(assetId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid assetId")
		return
	}
	if s.Assets == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	row, err := s.Assets.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "asset not found")
			return
		}
		s.logErr(r, "get asset", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to get asset")
		return
	}
	if _, _, _, ok := s.authorizeCollection(w, r, row.CollectionID, "stig-manager:collection:read", RoleRestricted); !ok {
		return
	}
	out, err := s.projectAsset(r.Context(), row, params.Projection)
	if err != nil {
		s.logErr(r, "decode asset", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to decode asset")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// UpdateAsset merges the supplied fields onto an existing Asset.
func (s APIServer) UpdateAsset(w http.ResponseWriter, r *http.Request, assetId api.AssetIdPath, params api.UpdateAssetParams) {
	id, ok := parseInt64Path(string(assetId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid assetId")
		return
	}
	if s.Assets == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	row, err := s.Assets.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "asset not found")
			return
		}
		s.logErr(r, "get asset", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to get asset")
		return
	}
	if _, _, _, ok := s.authorizeCollection(w, r, row.CollectionID, "stig-manager:collection", RoleManage); !ok {
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	var in api.AssetUpdate
	if err := json.Unmarshal(body, &in); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	upd := assetUpdateToStore(in)
	if in.LabelNames != nil {
		ids, err := s.resolveLabelNames(r.Context(), row.CollectionID, *in.LabelNames)
		if err != nil {
			s.handleAssetWriteErr(w, r, err)
			return
		}
		upd.LabelIDs = &ids
	}
	if in.Stigs != nil {
		bids := make([]string, 0, len(*in.Stigs))
		for _, b := range *in.Stigs {
			if b != nil {
				bids = append(bids, string(*b))
			}
		}
		upd.BenchmarkIDs = &bids
	}

	updated, err := s.Assets.Update(r.Context(), id, upd)
	if err != nil {
		s.handleAssetWriteErr(w, r, err)
		return
	}
	out, err := s.projectAsset(r.Context(), updated, params.Projection)
	if err != nil {
		s.logErr(r, "decode asset", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to decode asset")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// DeleteAsset soft-deletes an Asset.
func (s APIServer) DeleteAsset(w http.ResponseWriter, r *http.Request, assetId api.AssetIdPath, _ api.DeleteAssetParams) {
	id, ok := parseInt64Path(string(assetId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid assetId")
		return
	}
	if s.Assets == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	row, err := s.Assets.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "asset not found")
			return
		}
		s.logErr(r, "get asset", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to get asset")
		return
	}
	if _, _, _, ok := s.authorizeCollection(w, r, row.CollectionID, "stig-manager:collection", RoleManage); !ok {
		return
	}
	if err := s.Assets.Delete(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "asset not found")
			return
		}
		s.logErr(r, "delete asset", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to delete asset")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetStigsByAsset returns the STIGs currently mapped to the asset.
func (s APIServer) GetStigsByAsset(w http.ResponseWriter, r *http.Request, assetId api.AssetIdPath) {
	id, ok := parseInt64Path(string(assetId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid assetId")
		return
	}
	if s.Assets == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	row, err := s.Assets.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "asset not found")
			return
		}
		s.logErr(r, "get asset", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to get asset")
		return
	}
	if _, _, _, ok := s.authorizeCollection(w, r, row.CollectionID, "stig-manager:collection:read", RoleRestricted); !ok {
		return
	}
	stigs, err := s.Assets.AssignedStigs(r.Context(), id)
	if err != nil {
		s.logErr(r, "list asset stigs", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to list asset stigs")
		return
	}
	out := make([]api.AssetStigResponse, 0, len(stigs))
	for _, st := range stigs {
		bid := api.BenchmarkId(st.BenchmarkID)
		resp := api.AssetStigResponse{
			BenchmarkId: &bid,
			RevisionStr: api.RevisionStrRaw(st.RevisionStr),
		}
		if st.RevisionDate != nil {
			resp.RevisionDate = api.RevisionDate{Time: *st.RevisionDate}
		}
		rc := api.RuleCount(st.RuleCount)
		resp.RuleCount = &rc
		out = append(out, resp)
	}
	writeJSON(w, http.StatusOK, out)
}

// --- helpers -------------------------------------------------------------

// projectAsset converts a store.Asset to api.AssetProjected, optionally
// hydrating Stigs depending on the requested projection
// (?projection=stigs). Stats / recent-activity projections land in a
// later milestone.
func (s APIServer) projectAsset(ctx context.Context, row store.Asset, projection *api.AssetProjectionQuery) (api.AssetProjected, error) {
	collIDStr := api.CollectionId(strconv.FormatInt(row.CollectionID, 10))
	out := api.AssetProjected{
		AssetId: api.AssetId(strconv.FormatInt(row.AssetID, 10)),
		Collection: api.CollectionBasic{
			CollectionId: &collIDStr,
		},
		Name:         api.AssetName(row.Name),
		Noncomputing: row.Noncomputing,
		LabelIds:     []api.LabelId{},
	}
	if row.Description != "" {
		v := api.String255Nullable(row.Description)
		out.Description = &v
	}
	if row.FQDN != "" {
		v := api.String255Nullable(row.FQDN)
		out.Fqdn = &v
	}
	if row.IP != "" {
		v := api.String255Nullable(row.IP)
		out.Ip = &v
	}
	if row.MAC != "" {
		v := api.String255Nullable(row.MAC)
		out.Mac = &v
	}
	if len(row.Metadata) > 0 {
		meta := api.Metadata{}
		if err := json.Unmarshal(row.Metadata, &meta); err != nil {
			return api.AssetProjected{}, err
		}
		out.Metadata = &meta
	}
	if s.Assets != nil {
		ids, err := s.Assets.AssignedLabelIDs(ctx, row.AssetID)
		if err != nil {
			return api.AssetProjected{}, err
		}
		out.LabelIds = make([]api.LabelId, len(ids))
		for i, id := range ids {
			out.LabelIds[i] = api.LabelId(id)
		}
	}
	if projectionContains(projection, "stigs") && s.Assets != nil {
		stigs, err := s.Assets.AssignedStigs(ctx, row.AssetID)
		if err != nil {
			return api.AssetProjected{}, err
		}
		conv := make([]api.CollectionStig, 0, len(stigs))
		for _, st := range stigs {
			bid := api.BenchmarkId(st.BenchmarkID)
			cs := api.CollectionStig{BenchmarkId: &bid, RevisionStr: api.RevisionStr(st.RevisionStr)}
			conv = append(conv, cs)
		}
		out.Stigs = &conv
	}
	return out, nil
}

// createOneAsset shells out the create flow used by both
// CreateAsset and the single-object variant of CreateAssets.
func (s APIServer) createOneAsset(ctx context.Context, collID int64, in api.AssetCreateOrReplace) (store.Asset, error) {
	metadata, err := json.Marshal(in.Metadata)
	if err != nil {
		return store.Asset{}, err
	}
	labelIDs, err := s.resolveLabelNamesPtr(ctx, collID, in.LabelNames)
	if err != nil {
		return store.Asset{}, err
	}
	bids := make([]string, 0, len(in.Stigs))
	for _, b := range in.Stigs {
		bids = append(bids, string(b))
	}
	return s.Assets.Create(ctx, store.AssetCreate{
		CollectionID: collID,
		Name:         string(in.Name),
		FQDN:         derefStr((*string)(in.Fqdn)),
		IP:           derefStr((*string)(in.Ip)),
		MAC:          derefStr((*string)(in.Mac)),
		Description:  derefStr((*string)(in.Description)),
		Noncomputing: bool(in.Noncomputing),
		Metadata:     metadata,
		BenchmarkIDs: bids,
		LabelIDs:     labelIDs,
	})
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (s APIServer) resolveLabelNamesPtr(ctx context.Context, collID int64, names *[]api.LabelName) ([]string, error) {
	if names == nil || len(*names) == 0 {
		return nil, nil
	}
	return s.resolveLabelNames(ctx, collID, *names)
}

// resolveLabelNames maps a slice of label names to the corresponding
// label ids for the given collection. Names that do not exist are
// silently dropped (matching upstream's lenient behaviour); strict
// validation lives on the dedicated label-assignment endpoint.
func (s APIServer) resolveLabelNames(ctx context.Context, collID int64, names []api.LabelName) ([]string, error) {
	if s.Labels == nil || len(names) == 0 {
		return nil, nil
	}
	rows, err := s.Labels.List(ctx, collID)
	if err != nil {
		return nil, err
	}
	idByLowerName := make(map[string]string, len(rows))
	for _, l := range rows {
		idByLowerName[strings.ToLower(l.Name)] = l.LabelID
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		if id, ok := idByLowerName[strings.ToLower(string(n))]; ok {
			out = append(out, id)
		}
	}
	return out, nil
}

func assetUpdateToStore(in api.AssetUpdate) store.AssetUpdate {
	out := store.AssetUpdate{}
	if in.Name != nil {
		v := string(*in.Name)
		out.Name = &v
	}
	if in.Fqdn != nil {
		v := string(*in.Fqdn)
		out.FQDN = &v
	}
	if in.Ip != nil {
		v := string(*in.Ip)
		out.IP = &v
	}
	if in.Mac != nil {
		v := string(*in.Mac)
		out.MAC = &v
	}
	if in.Description != nil {
		v := string(*in.Description)
		out.Description = &v
	}
	if in.Noncomputing != nil {
		v := bool(*in.Noncomputing)
		out.Noncomputing = &v
	}
	if in.Metadata != nil {
		raw, err := json.Marshal(*in.Metadata)
		if err == nil {
			out.Metadata = raw
		}
	}
	if in.CollectionId != nil {
		id, ok := parseInt64Path(string(*in.CollectionId))
		if ok {
			out.CollectionID = &id
		}
	}
	return out
}

func batchToCreate(coll api.CollectionIdPath, b api.AssetBatchItem) api.AssetCreateOrReplace {
	return api.AssetCreateOrReplace{
		CollectionId: coll,
		Name:         b.Name,
		Description:  b.Description,
		Fqdn:         b.Fqdn,
		Ip:           b.Ip,
		LabelNames:   b.LabelNames,
		Mac:          b.Mac,
		Metadata:     b.Metadata,
		Noncomputing: b.Noncomputing,
		Stigs:        b.Stigs,
	}
}

func errAtAsset(i int, err error) error {
	return &assetIndexErr{Index: i, Err: err}
}

type assetIndexErr struct {
	Index int
	Err   error
}

func (a *assetIndexErr) Error() string {
	return "assets[" + strconv.Itoa(a.Index) + "]: " + a.Err.Error()
}
func (a *assetIndexErr) Unwrap() error { return a.Err }

func (s APIServer) handleAssetWriteErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrDuplicateName):
		writeAuthError(w, http.StatusBadRequest, "duplicate asset name")
	case errors.Is(err, store.ErrConflict):
		writeAuthError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, store.ErrNotFound):
		writeAuthError(w, http.StatusNotFound, "asset not found")
	default:
		s.logErr(r, "write asset", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to write asset")
	}
}
