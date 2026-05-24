package server

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/Exonical/stig-manager-react/api/internal/api"
	"github.com/Exonical/stig-manager-react/api/internal/store"
	"github.com/Exonical/stig-manager-react/api/internal/xccdf"
)

// importBenchmarkMaxBytes caps the size of an uploaded STIG XCCDF file
// (or DISA zip bundle). Published DISA STIG zips are typically 1-5 MiB;
// 64 MiB leaves comfortable head-room without exposing the API to
// memory-exhaustion uploads.
const importBenchmarkMaxBytes = 64 << 20

// GetSTIGs lists imported STIG benchmarks. Supports a case-insensitive
// `title` substring filter from the OpenAPI spec.
func (s APIServer) GetSTIGs(w http.ResponseWriter, r *http.Request, params api.GetSTIGsParams) {
	if !s.requiredScope(w, r, "stig-manager:stig:read") {
		return
	}
	if s.Stigs == nil {
		writeJSON(w, http.StatusOK, []api.STIG{})
		return
	}
	opts := store.ListSTIGsOptions{}
	if params.Title != nil {
		opts.TitleContains = strings.TrimSpace(*params.Title)
	}
	rows, err := s.Stigs.List(r.Context(), opts)
	if err != nil {
		s.logErr(r, "list stigs", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to list stigs")
		return
	}
	withRevisions := projectionContains(params.Projection, "revisions")
	out := make([]api.STIG, 0, len(rows))
	for _, row := range rows {
		out = append(out, storeToAPIStig(row, withRevisions))
	}
	writeJSON(w, http.StatusOK, out)
}

// GetStigById returns the projection for a single STIG.
func (s APIServer) GetStigById(w http.ResponseWriter, r *http.Request, benchmarkId string, _ api.GetStigByIdParams) {
	if !s.requiredScope(w, r, "stig-manager:stig:read") {
		return
	}
	if s.Stigs == nil {
		writeAuthError(w, http.StatusNotFound, "stig not found")
		return
	}
	row, err := s.Stigs.Get(r.Context(), benchmarkId)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "stig not found")
			return
		}
		s.logErr(r, "get stig", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to get stig")
		return
	}
	writeJSON(w, http.StatusOK, storeToAPIStig(*row, true))
}

// ImportBenchmark accepts an XCCDF Benchmark file via multipart upload
// and writes the parsed revision into the database. The clobber query
// parameter controls whether an existing (benchmark, revisionStr) row
// is replaced or rejected.
func (s APIServer) ImportBenchmark(w http.ResponseWriter, r *http.Request, params api.ImportBenchmarkParams) {
	if !s.requiredScope(w, r, "stig-manager:stig") {
		return
	}
	if s.Stigs == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}

	if err := r.ParseMultipartForm(importBenchmarkMaxBytes); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid multipart body: "+err.Error())
		return
	}
	file, _, err := r.FormFile("importFile")
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "missing importFile part: "+err.Error())
		return
	}
	defer file.Close()

	limited := io.LimitReader(file, importBenchmarkMaxBytes)
	buf, err := io.ReadAll(limited)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "failed to read upload: "+err.Error())
		return
	}

	// Accept raw XCCDF XML, a zip containing one or more *xccdf.xml
	// files (the standard DISA STIG bundle layout), or a zip that
	// itself contains a nested *xccdf.zip (the DISA STIG Library
	// quarterly bundle).
	extracted, err := xccdf.ExtractBenchmarks(buf)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid upload: "+err.Error())
		return
	}

	clobber := params.Clobber != nil && *params.Clobber

	benches := make([]*xccdf.Benchmark, 0, len(extracted))
	for _, ex := range extracted {
		benches = append(benches, ex.Benchmark)
	}

	// All benchmarks are committed in a single transaction so a late-
	// stage failure (e.g. duplicate revision without clobber) cannot
	// leave the database in a half-imported state.
	revs, err := s.Stigs.ImportRevisions(r.Context(), benches, clobber)
	if err != nil {
		if errors.Is(err, store.ErrDuplicateName) {
			writeAuthError(w, http.StatusBadRequest,
				"one or more revisions already exist; pass ?clobber=true to overwrite")
			return
		}
		s.logErr(r, "import revisions", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to import upload")
		return
	}

	imports := make([]api.RevisionPost, 0, len(revs))
	for _, rev := range revs {
		action := api.RevisionPostAction("inserted")
		bid := api.BenchmarkId(rev.BenchmarkID)
		marking := api.RevisionMarkingNullable(rev.Marking)
		one := api.RevisionPost{
			Action:      action,
			BenchmarkId: &bid,
			RevisionStr: api.RevisionStr(rev.RevisionStr),
		}
		if rev.Marking != "" {
			one.Marking = &marking
		}
		imports = append(imports, one)
	}

	// Single-XCCDF uploads keep the legacy RevisionPost shape so
	// existing clients aren't broken; multi-XCCDF zips fan out to an
	// array under the same status code.
	if len(imports) == 1 {
		writeJSON(w, http.StatusOK, imports[0])
		return
	}
	writeJSON(w, http.StatusOK, imports)
}

// GetRuleByRuleId returns the projection for a Rule by its ruleId.
func (s APIServer) GetRuleByRuleId(w http.ResponseWriter, r *http.Request, ruleId string, _ api.GetRuleByRuleIdParams) {
	if !s.requiredScope(w, r, "stig-manager:stig:read") {
		return
	}
	if s.Stigs == nil {
		writeAuthError(w, http.StatusNotFound, "rule not found")
		return
	}
	row, err := s.Stigs.GetRuleByRuleID(r.Context(), ruleId)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "rule not found")
			return
		}
		s.logErr(r, "get rule", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to get rule")
		return
	}
	writeJSON(w, http.StatusOK, ruleToAPI(*row))
}

// GetCci returns a single CCI projection plus the STIGs that reference
// it. Missing CCIs yield 404.
func (s APIServer) GetCci(w http.ResponseWriter, r *http.Request, cci string, _ api.GetCciParams) {
	if !s.requiredScope(w, r, "stig-manager:stig:read") {
		return
	}
	if s.Stigs == nil {
		writeAuthError(w, http.StatusNotFound, "cci not found")
		return
	}
	row, err := s.Stigs.GetCCI(r.Context(), normaliseCciParam(cci))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "cci not found")
			return
		}
		s.logErr(r, "get cci", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to get cci")
		return
	}
	writeJSON(w, http.StatusOK, cciToAPI(*row))
}

// storeToAPIStig projects a store.STIG into the OpenAPI shape.
func storeToAPIStig(row store.STIG, withRevisions bool) api.STIG {
	bid := api.BenchmarkId(row.BenchmarkID)
	out := api.STIG{
		BenchmarkId:     &bid,
		Title:           api.StigTitle(row.Title),
		LastRevisionStr: row.LastRevisionStr,
		CollectionIds:   []api.CollectionId{},
		RuleCount:       intPtr(row.RuleCount),
	}
	if row.LastRevisionDate != nil {
		nullable := api.StringDateNullable{Time: *row.LastRevisionDate}
		out.LastRevisionDate = &nullable
	}
	if row.Marking != "" {
		marking := api.RevisionMarkingNullable(row.Marking)
		out.Marking = &marking
	}
	if row.Status != "" {
		st := api.StatusText(row.Status)
		out.Status = &st
	}
	if withRevisions && len(row.RevisionStrs) > 0 {
		raw := make([]api.RevisionStrRaw, 0, len(row.RevisionStrs))
		for _, rs := range row.RevisionStrs {
			raw = append(raw, api.RevisionStrRaw(rs))
		}
		out.RevisionStrs = &raw
	}
	return out
}

// ruleToAPI projects a store.RuleProjection into the OpenAPI
// RuleProjected schema returned by GET /stigs/rules/{ruleId}.
func ruleToAPI(row store.RuleProjection) api.RuleProjected {
	rid := api.RuleId(row.RuleID)
	title := api.RuleTitle(row.Title)
	version := api.VersionString(row.VersionStr)
	groupID := api.GroupId(row.RevisionStr)
	groupTitle := api.GroupTitle("")

	out := api.RuleProjected{
		RuleId:     &rid,
		Severity:   row.Severity,
		Title:      title,
		Version:    version,
		GroupId:    &groupID,
		GroupTitle: &groupTitle,
	}

	if row.CheckContent != "" || row.CheckSystem != "" {
		out.Check = &api.Check{
			Content: strPtr(row.CheckContent),
			System:  strPtr(row.CheckSystem),
		}
	}
	if row.FixText != "" || row.FixID != "" {
		out.Fix = &api.Fix{
			Text:   strPtr(row.FixText),
			Fixref: strPtr(row.FixID),
		}
	}
	if len(row.CCIs) > 0 {
		basics := make([]api.CciBasic, 0, len(row.CCIs))
		for _, cci := range row.CCIs {
			basics = append(basics, api.CciBasic{Cci: api.CciString(cci)})
		}
		out.Ccis = &basics
	}
	if row.Description != "" {
		// Embed the raw VulnDiscussion blob under detail.vulnDiscussion;
		// fine-grained parsing of the DISA pseudo-XML lives behind the
		// review-content milestone.
		desc := row.Description
		out.Detail = &struct {
			Documentable             *string `json:"documentable,omitempty"`
			FalseNegatives           *string `json:"falseNegatives,omitempty"`
			FalsePositives           *string `json:"falsePositives,omitempty"`
			MitigationControl        *string `json:"mitigationControl,omitempty"`
			Mitigations              *string `json:"mitigations,omitempty"`
			PotentialImpacts         *string `json:"potentialImpacts,omitempty"`
			Responsibility           *string `json:"responsibility,omitempty"`
			SeverityOverrideGuidance *string `json:"severityOverrideGuidance,omitempty"`
			ThirdPartyTools          *string `json:"thirdPartyTools,omitempty"`
			VulnDiscussion           *string `json:"vulnDiscussion,omitempty"`
			Weight                   *string `json:"weight,omitempty"`
		}{VulnDiscussion: &desc}
	}
	if row.BenchmarkID != "" {
		bid := api.BenchmarkId(row.BenchmarkID)
		out.Stigs = &[]api.RevisionBasic{{
			BenchmarkId: &bid,
			RevisionStr: api.RevisionStr(row.RevisionStr),
		}}
	}
	return out
}

// cciToAPI projects a store.CCI into the OpenAPI Cci.
func cciToAPI(row store.CCI) api.Cci {
	cciStr := api.CciString(row.CCI)
	def := api.DefinitionString(row.Definition)
	out := api.Cci{
		Cci:        &cciStr,
		Definition: &def,
	}
	if row.Type != "" {
		t := api.CciType(row.Type)
		out.Type = &t
	}
	if row.Status != "" {
		st := api.CciStatus(row.Status)
		out.Status = &st
	}
	if row.PublishDate != nil {
		pd := api.StringDateTime(*row.PublishDate)
		out.Publishdate = &pd
	}
	if len(row.Stigs) > 0 {
		basics := make([]api.RevisionBasic, 0, len(row.Stigs))
		for _, ref := range row.Stigs {
			bid := api.BenchmarkId(ref.BenchmarkID)
			basics = append(basics, api.RevisionBasic{
				BenchmarkId: &bid,
				RevisionStr: api.RevisionStr(ref.RevisionStr),
			})
		}
		out.Stigs = &basics
	}
	return out
}

// normaliseCciParam accepts either "CCI-000366" or "000366" and returns
// the canonical "CCI-000366" form used in storage. The OpenAPI spec
// declares the path parameter as `^[0-9]{6}$`, so DISA's own format
// (six digits, no prefix) is the spec-conformant variant; we accept
// both for ergonomic curl usage.
func normaliseCciParam(in string) string {
	in = strings.ToUpper(strings.TrimSpace(in))
	if strings.HasPrefix(in, "CCI-") {
		return in
	}
	return "CCI-" + in
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func intPtr(n int) *int {
	return &n
}

// projectionContains is true when the OpenAPI projection slice contains
// the requested value (case-insensitive).
func projectionContains(p *api.StigProjectionQuery, want string) bool {
	if p == nil {
		return false
	}
	want = strings.ToLower(want)
	for _, v := range *p {
		if strings.ToLower(v) == want {
			return true
		}
	}
	return false
}


