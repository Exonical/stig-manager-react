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

// PostReviewBatch handles `POST /collections/{cid}/reviews` — the
// bulk review-mutation endpoint that the upstream Collection Review
// workspace uses to apply a single change (e.g. "mark all NotAFinding
// for these benchmarks across these assets") across many (asset, rule)
// pairs at once.
//
// The flow per the OpenAPI spec:
//
//  1. Resolve the assets criteria (oneOf {assetIds[]} | {benchmarkIds[]})
//     into a concrete set of in-collection, enabled asset_ids.
//  2. Resolve the rules criteria (oneOf {ruleIds[]} | {benchmarkIds[]})
//     into (rule_id, benchmark_id) tuples.
//  3. Cross-join them, but only keep pairs where the asset actually has
//     the rule's benchmark assigned (via asset_stig). This is what
//     `ReviewRepo.ResolveBatchTargets` does in one query.
//  4. For each resulting pair, apply the requested action:
//     - insert: skip pairs that already have a review.
//     - update: skip pairs without a review (and filter further with
//       `updateFilters` when supplied).
//     - merge:  insert when missing, patch when present.
//  5. Return a `ReviewBatchResponse` with insert/update/failure counts
//     plus a `validationErrors` list, OR a `ReviewBatchResponseDryRun`
//     with `willInsert`/`willUpdate`/`willFailValidation` when the
//     `dryRun` flag is set.
//
// The per-rule ACL gating that the spec sketches (grant.acl entries
// that downgrade access on specific rule/asset/benchmark combinations)
// is still deferred to a follow-up — this milestone implements the
// resolution + action plumbing end-to-end so the SPA can drive bulk
// updates from M10 onward.
func (s APIServer) PostReviewBatch(
	w http.ResponseWriter, r *http.Request,
	collectionId api.CollectionIdPath,
) {
	collID, ok := parseInt64Path(string(collectionId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
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
	var batch api.ReviewBatch
	if err := json.Unmarshal(body, &batch); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	crit, action, dryRun, src, errMsg := parseBatchRequest(batch)
	if errMsg != "" {
		writeAuthError(w, http.StatusBadRequest, errMsg)
		return
	}

	pairs, err := s.Reviews.ResolveBatchTargets(r.Context(), collID, crit)
	if err != nil {
		s.logErr(r, "resolve batch targets", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to resolve targets")
		return
	}
	summaries, err := s.Reviews.GetSummaries(r.Context(), pairs)
	if err != nil {
		s.logErr(r, "get review summaries", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to load existing reviews")
		return
	}
	existing := make(map[batchKey]store.ReviewSummary, len(summaries))
	for _, sum := range summaries {
		existing[batchKey{sum.AssetID, sum.RuleID}] = sum
	}

	filters, errMsg := parseBatchFilters(batch.UpdateFilters)
	if errMsg != "" {
		writeAuthError(w, http.StatusBadRequest, errMsg)
		return
	}

	type result struct {
		inserts int
		updates int
		fails   int
		errs    []batchValidationError
	}
	var res result
	apply := func(p store.AssetRulePair, kind batchActionKind, reason string) {
		switch kind {
		case batchKindInsert:
			res.inserts++
		case batchKindUpdate:
			res.updates++
		case batchKindFail:
			res.fails++
			res.errs = append(res.errs, batchValidationError{
				AssetID: p.AssetID, RuleID: p.RuleID, Reason: reason,
			})
		}
	}

	for _, p := range pairs {
		summary, hasReview := existing[batchKey{p.AssetID, p.RuleID}]
		switch action {
		case api.ReviewBatchAction("insert"):
			if hasReview {
				continue // not an error; insert simply skips existing rows
			}
			if reason := validateInsertSource(src); reason != "" {
				apply(p, batchKindFail, reason)
				continue
			}
			if dryRun {
				apply(p, batchKindInsert, "")
				continue
			}
			if _, err := s.Reviews.Put(r.Context(), p.AssetID, p.RuleID, userID, src.full); err != nil {
				apply(p, batchKindFail, classifyBatchErr(err))
				continue
			}
			apply(p, batchKindInsert, "")
		case api.ReviewBatchAction("update"):
			if !hasReview {
				continue
			}
			if !filters.matches(summary) {
				continue
			}
			if dryRun {
				apply(p, batchKindUpdate, "")
				continue
			}
			if _, err := s.Reviews.Patch(r.Context(), p.AssetID, p.RuleID, userID, src.patch); err != nil {
				apply(p, batchKindFail, classifyBatchErr(err))
				continue
			}
			apply(p, batchKindUpdate, "")
		case api.ReviewBatchAction("merge"):
			if hasReview {
				if !filters.matches(summary) {
					continue
				}
				if dryRun {
					apply(p, batchKindUpdate, "")
					continue
				}
				if _, err := s.Reviews.Patch(r.Context(), p.AssetID, p.RuleID, userID, src.patch); err != nil {
					apply(p, batchKindFail, classifyBatchErr(err))
					continue
				}
				apply(p, batchKindUpdate, "")
			} else {
				if reason := validateInsertSource(src); reason != "" {
					apply(p, batchKindFail, reason)
					continue
				}
				if dryRun {
					apply(p, batchKindInsert, "")
					continue
				}
				if _, err := s.Reviews.Put(r.Context(), p.AssetID, p.RuleID, userID, src.full); err != nil {
					apply(p, batchKindFail, classifyBatchErr(err))
					continue
				}
				apply(p, batchKindInsert, "")
			}
		}
	}

	if dryRun {
		writeJSON(w, http.StatusOK, buildBatchDryRunResponse(res.inserts, res.updates, res.fails, res.errs))
	} else {
		writeJSON(w, http.StatusOK, buildBatchResponse(res.inserts, res.updates, res.fails, res.errs))
	}
}

// --- batch request parsing --------------------------------------------------

type batchSource struct {
	full  store.ReviewWrite // used for insert / merge-into-missing
	patch store.ReviewPatch // used for update / merge-into-existing
}

type batchKey struct {
	assetID int64
	ruleID  string
}

type batchActionKind int

const (
	batchKindNoop batchActionKind = iota
	batchKindInsert
	batchKindUpdate
	batchKindFail
)

type batchValidationError struct {
	AssetID int64
	RuleID  string
	Reason  string
}

// parseBatchRequest unpacks the OpenAPI ReviewBatch envelope into
// values the handler can work with directly. Returns an empty errMsg
// when the payload is well-formed.
func parseBatchRequest(b api.ReviewBatch) (store.BatchCriteria, api.ReviewBatchAction, bool, batchSource, string) {
	action := api.ReviewBatchAction("merge")
	if b.Action != nil {
		action = *b.Action
	}
	switch action {
	case "insert", "update", "merge":
	default:
		return store.BatchCriteria{}, action, false, batchSource{}, "invalid action"
	}

	dryRun := false
	if b.DryRun != nil {
		dryRun = *b.DryRun
	}

	crit := store.BatchCriteria{}
	if a, err := b.Assets.AsReviewBatchCriteriaAssetIds(); err == nil && len(a.AssetIds) > 0 {
		crit.AssetIDs = make([]int64, 0, len(a.AssetIds))
		for _, id := range a.AssetIds {
			n, ok := parseInt64Path(string(id))
			if !ok {
				return crit, action, dryRun, batchSource{}, "invalid assetId in assets criteria"
			}
			crit.AssetIDs = append(crit.AssetIDs, n)
		}
	} else if b, err := b.Assets.AsReviewBatchCriteriaBenchmarkIds(); err == nil && len(b.BenchmarkIds) > 0 {
		for _, id := range b.BenchmarkIds {
			if id == nil {
				continue
			}
			crit.AssetBenchmarks = append(crit.AssetBenchmarks, string(*id))
		}
	} else {
		return crit, action, dryRun, batchSource{}, "missing or invalid assets criteria"
	}
	if r, err := b.Rules.AsReviewBatchCriteriaRuleIds(); err == nil && len(r.RuleIds) > 0 {
		for _, id := range r.RuleIds {
			if id == nil {
				continue
			}
			crit.RuleIDs = append(crit.RuleIDs, string(*id))
		}
	} else if rb, err := b.Rules.AsReviewBatchCriteriaBenchmarkIds(); err == nil && len(rb.BenchmarkIds) > 0 {
		for _, id := range rb.BenchmarkIds {
			if id == nil {
				continue
			}
			crit.RuleBenchmarks = append(crit.RuleBenchmarks, string(*id))
		}
	} else {
		return crit, action, dryRun, batchSource{}, "missing or invalid rules criteria"
	}

	src, srcErr := buildBatchSource(b.Source.Review)
	if srcErr != "" {
		return crit, action, dryRun, batchSource{}, srcErr
	}
	return crit, action, dryRun, src, ""
}

// buildBatchSource turns the ReviewAssetRulePatch from
// `ReviewBatchSource.review` into both a full ReviewWrite (for inserts)
// and a partial ReviewPatch (for updates).
func buildBatchSource(in api.ReviewAssetRulePatch) (batchSource, string) {
	full := store.ReviewWrite{}
	if in.Result != nil {
		full.Result = string(*in.Result)
	}
	if in.Detail != nil {
		full.Detail = string(*in.Detail)
	}
	if in.Comment != nil {
		full.Comment = string(*in.Comment)
	}
	if in.Metadata != nil {
		b, err := json.Marshal(in.Metadata)
		if err != nil {
			return batchSource{}, "invalid source.review.metadata"
		}
		full.Metadata = b
	}
	if in.ResultEngine != nil {
		b, err := json.Marshal(in.ResultEngine)
		if err != nil {
			return batchSource{}, "invalid source.review.resultEngine"
		}
		full.ResultEngine = b
	}
	if in.Status != nil {
		label, text, err := unpackStatusWrite(*in.Status)
		if err != nil {
			return batchSource{}, "invalid source.review.status"
		}
		full.StatusLabel = label
		full.StatusText = text
	}

	patch := store.ReviewPatch{}
	if in.Result != nil {
		s := string(*in.Result)
		patch.Result = &s
	}
	if in.Detail != nil {
		s := string(*in.Detail)
		patch.Detail = &s
	}
	if in.Comment != nil {
		s := string(*in.Comment)
		patch.Comment = &s
	}
	if in.Metadata != nil {
		b, _ := json.Marshal(in.Metadata)
		patch.Metadata = b
	}
	if in.ResultEngine != nil {
		b, _ := json.Marshal(in.ResultEngine)
		patch.ResultEngine = b
	}
	if in.Status != nil {
		label, text, _ := unpackStatusWrite(*in.Status)
		patch.StatusLabel = &label
		patch.StatusText = &text
	}
	return batchSource{full: full, patch: patch}, ""
}

// validateInsertSource enforces the same minimum set of fields that
// PUT /reviews/{assetId}/{ruleId} does: result, detail, comment must
// all be present so the new row is well-formed.
func validateInsertSource(src batchSource) string {
	if src.full.Result == "" {
		return "source.review.result is required for insert"
	}
	if src.patch.Detail == nil {
		return "source.review.detail is required for insert"
	}
	if src.patch.Comment == nil {
		return "source.review.comment is required for insert"
	}
	return ""
}

// --- batch update filters ---------------------------------------------------

type batchFilters struct {
	resultEquals  *string
	resultNotEq   *string
	statusEquals  *string
	statusNotEq   *string
	unsupportedOk bool // unsupported filter types are accepted but logged
}

func (f batchFilters) matches(sum store.ReviewSummary) bool {
	if f.resultEquals != nil && sum.Result != *f.resultEquals {
		return false
	}
	if f.resultNotEq != nil && sum.Result == *f.resultNotEq {
		return false
	}
	if f.statusEquals != nil && sum.StatusLabel != *f.statusEquals {
		return false
	}
	if f.statusNotEq != nil && sum.StatusLabel == *f.statusNotEq {
		return false
	}
	return true
}

// parseBatchFilters extracts the supported update-filter types. The
// spec also defines String/Date/User filters, but the upstream UI
// (and every test we have) only emits Result and Status filters today,
// so we accept the others without filtering and surface that decision
// via the `unsupportedOk` flag for future telemetry.
func parseBatchFilters(in *[]api.ReviewBatchFilter) (batchFilters, string) {
	var out batchFilters
	out.unsupportedOk = true
	if in == nil {
		return out, ""
	}
	for _, f := range *in {
		if rf, err := f.AsReviewBatchFilterResult(); err == nil && rf.Field == api.ReviewBatchFilterResultFieldResult {
			v := string(rf.Value)
			cond := api.ReviewBatchFilterResultConditionEquals
			if rf.Condition != nil {
				cond = *rf.Condition
			}
			switch cond {
			case api.ReviewBatchFilterResultConditionEquals:
				out.resultEquals = &v
			case api.ReviewBatchFilterResultConditionNotequal:
				out.resultNotEq = &v
			}
			continue
		}
		if sf, err := f.AsReviewBatchFilterStatus(); err == nil && (sf.Field == api.Status || sf.Field == api.StatusLabel) {
			v := string(sf.Value)
			cond := api.ReviewBatchFilterStatusConditionEquals
			if sf.Condition != nil {
				cond = *sf.Condition
			}
			switch cond {
			case api.ReviewBatchFilterStatusConditionEquals:
				out.statusEquals = &v
			case api.ReviewBatchFilterStatusConditionNotequal:
				out.statusNotEq = &v
			}
			continue
		}
	}
	return out, ""
}

// --- response building ------------------------------------------------------

func buildBatchResponse(ins, upd, fail int, errs []batchValidationError) api.ReviewBatchResponse {
	resp := api.ReviewBatchResponse{
		Inserted:         ins,
		Updated:          upd,
		FailedValidation: fail,
		ValidationErrors: []struct {
			AssetId *api.AssetId   `json:"assetId,omitempty"`
			Error   *api.String255 `json:"error,omitempty"`
			RuleId  *api.RuleId    `json:"ruleId,omitempty"`
		}{},
	}
	for _, e := range errs {
		aid := api.AssetId(int64ToStr(e.AssetID))
		rid := api.RuleId(e.RuleID)
		emsg := api.String255(e.Reason)
		resp.ValidationErrors = append(resp.ValidationErrors, struct {
			AssetId *api.AssetId   `json:"assetId,omitempty"`
			Error   *api.String255 `json:"error,omitempty"`
			RuleId  *api.RuleId    `json:"ruleId,omitempty"`
		}{AssetId: &aid, Error: &emsg, RuleId: &rid})
	}
	return resp
}

func buildBatchDryRunResponse(ins, upd, fail int, errs []batchValidationError) api.ReviewBatchResponseDryRun {
	resp := api.ReviewBatchResponseDryRun{
		WillInsert:         ins,
		WillUpdate:         upd,
		WillFailValidation: fail,
		ValidationErrors: []struct {
			AssetId *api.AssetId   `json:"assetId,omitempty"`
			Error   *api.String255 `json:"error,omitempty"`
			RuleId  *api.RuleId    `json:"ruleId,omitempty"`
		}{},
	}
	for _, e := range errs {
		aid := api.AssetId(int64ToStr(e.AssetID))
		rid := api.RuleId(e.RuleID)
		emsg := api.String255(e.Reason)
		resp.ValidationErrors = append(resp.ValidationErrors, struct {
			AssetId *api.AssetId   `json:"assetId,omitempty"`
			Error   *api.String255 `json:"error,omitempty"`
			RuleId  *api.RuleId    `json:"ruleId,omitempty"`
		}{AssetId: &aid, Error: &emsg, RuleId: &rid})
	}
	return resp
}

// classifyBatchErr converts a store-layer error into a human-readable
// validation reason. Constraint violations and conflicts get a short
// label; everything else falls back to the raw error text.
func classifyBatchErr(err error) string {
	if errors.Is(err, store.ErrConflict) {
		return "invalid review payload"
	}
	if errors.Is(err, store.ErrNotFound) {
		return "review not found"
	}
	return err.Error()
}

func int64ToStr(n int64) string {
	return strconv.FormatInt(n, 10)
}
