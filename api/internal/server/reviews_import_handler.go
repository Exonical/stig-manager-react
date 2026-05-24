package server

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/Exonical/stig-manager-react/api/internal/checklist"
	"github.com/Exonical/stig-manager-react/api/internal/store"
	"github.com/go-chi/chi/v5"
)

// reviewImportMaxBytes caps the multipart payload at 64 MiB. Real-world
// checklist files run < 5 MiB each so this is a sensible defence
// against accidental uploads of a whole STIG library bundle.
const reviewImportMaxBytes = 64 << 20

// fileImportResult is the per-file diagnostic surfaced to the SPA.
type fileImportResult struct {
	Filename     string                 `json:"filename"`
	Format       string                 `json:"format"`
	BenchmarkID  string                 `json:"benchmarkId,omitempty"`
	Revision     string                 `json:"revision,omitempty"`
	AssetID      int64                  `json:"assetId,omitempty"`
	AssetName    string                 `json:"assetName,omitempty"`
	Reviews      int                    `json:"reviews"`
	WillInsert   int                    `json:"willInsert"`
	WillUpdate   int                    `json:"willUpdate"`
	Inserted     int                    `json:"inserted"`
	Updated      int                    `json:"updated"`
	Rejected     []map[string]string    `json:"rejected,omitempty"`
	Error        string                 `json:"error,omitempty"`
}

// reviewImportResponse is the JSON body returned by the import handler.
type reviewImportResponse struct {
	DryRun bool               `json:"dryRun"`
	Files  []fileImportResult `json:"files"`
	Totals struct {
		Reviews    int `json:"reviews"`
		WillInsert int `json:"willInsert"`
		WillUpdate int `json:"willUpdate"`
		Inserted   int `json:"inserted"`
		Updated    int `json:"updated"`
		Rejected   int `json:"rejected"`
	} `json:"totals"`
}

// ImportReviewsByCollection accepts one or more CKL/CKLB/XCCDF result
// files via multipart upload, maps each to an asset already present in
// the collection (by case-insensitive name, FQDN, MAC, or IP), and
// either previews (dryRun=true) or applies the implied reviews via
// ReviewRepo.Put. This endpoint is registered off the generated
// OpenAPI router because the spec models reviews as JSON-only —
// upstream surfaces the same behaviour through a separate jobs
// pipeline that this single-shot endpoint compresses into one request.
func (s APIServer) ImportReviewsByCollection(w http.ResponseWriter, r *http.Request) {
	cidStr := chi.URLParam(r, "collectionId")
	collID, ok := parseInt64Path(cidStr)
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return
	}
	_, userID, _, ok := s.authorizeCollection(w, r, collID, "stig-manager:collection", RoleManage)
	if !ok {
		return
	}
	if s.Reviews == nil || s.Assets == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, reviewImportMaxBytes)
	if err := r.ParseMultipartForm(reviewImportMaxBytes); err != nil {
		writeAuthError(w, http.StatusBadRequest, "multipart parse: "+err.Error())
		return
	}

	dryRun := strings.EqualFold(r.FormValue("dryRun"), "true")

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		writeAuthError(w, http.StatusBadRequest, "no files uploaded under field \"files\"")
		return
	}

	// Pre-load the collection's assets once. Importers commonly upload
	// 10s of files at a time; resolving each one with its own SELECT
	// is wasteful when we can keep an in-memory index keyed on the
	// candidate match fields.
	assets, err := s.Assets.List(r.Context(), store.ListAssetsOptions{CollectionID: collID})
	if err != nil {
		s.logErr(r, "list assets for import", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to load collection assets")
		return
	}
	idx := newAssetIndex(assets)

	resp := reviewImportResponse{DryRun: dryRun, Files: []fileImportResult{}}

	for _, fh := range files {
		res := s.importOneFile(r, fh, idx, userID, dryRun)
		resp.Files = append(resp.Files, res)
		resp.Totals.Reviews += res.Reviews
		resp.Totals.WillInsert += res.WillInsert
		resp.Totals.WillUpdate += res.WillUpdate
		resp.Totals.Inserted += res.Inserted
		resp.Totals.Updated += res.Updated
		resp.Totals.Rejected += len(res.Rejected)
	}

	writeJSON(w, http.StatusOK, resp)
}

// importOneFile parses a single uploaded file, resolves the target
// asset, and applies or previews each contained review.
func (s APIServer) importOneFile(
	r *http.Request, fh *multipart.FileHeader, idx *assetIndex,
	userID int64, dryRun bool,
) fileImportResult {
	res := fileImportResult{Filename: fh.Filename}

	f, err := fh.Open()
	if err != nil {
		res.Error = "open: " + err.Error()
		return res
	}
	defer f.Close()

	body, err := io.ReadAll(io.LimitReader(f, reviewImportMaxBytes))
	if err != nil {
		res.Error = "read: " + err.Error()
		return res
	}

	format, parsed, err := parseChecklistFile(fh.Filename, body)
	if err != nil {
		res.Format = format
		res.Error = "parse: " + err.Error()
		return res
	}
	res.Format = format
	res.BenchmarkID = parsed.BenchmarkID
	res.Revision = parsed.Revision

	asset, ok := idx.Resolve(parsed.Asset, fh.Filename)
	if !ok {
		res.Error = "no asset in this collection matched the file's identity (host/fqdn/ip/mac/filename)"
		return res
	}
	res.AssetID = asset.AssetID
	res.AssetName = asset.Name

	res.Reviews = len(parsed.Reviews)
	for _, pr := range parsed.Reviews {
		if pr.RuleID == "" {
			res.Rejected = append(res.Rejected, map[string]string{
				"ruleId": "",
				"reason": "missing ruleId",
			})
			continue
		}
		existed, err := s.Reviews.Exists(r.Context(), asset.AssetID, pr.RuleID)
		if err != nil {
			res.Rejected = append(res.Rejected, map[string]string{
				"ruleId": pr.RuleID,
				"reason": "exists check failed: " + err.Error(),
			})
			continue
		}
		if dryRun {
			if existed {
				res.WillUpdate++
			} else {
				res.WillInsert++
			}
			continue
		}
		w8 := store.ReviewWrite{
			Result:      string(pr.Result),
			Detail:      pr.Detail,
			Comment:     pr.Comment,
			AutoResult:  pr.AutoResult,
			StatusLabel: "saved",
		}
		if _, err := s.Reviews.Put(r.Context(), asset.AssetID, pr.RuleID, userID, w8); err != nil {
			reason := err.Error()
			if errors.Is(err, store.ErrConflict) {
				reason = "invalid review payload"
			}
			res.Rejected = append(res.Rejected, map[string]string{
				"ruleId": pr.RuleID,
				"reason": reason,
			})
			continue
		}
		if existed {
			res.Updated++
		} else {
			res.Inserted++
		}
	}
	return res
}

// parseChecklistFile dispatches by filename + magic byte to one of the
// existing parsers. Returns the detected format token even when the
// parse itself fails so the SPA can label the per-file diagnostic.
func parseChecklistFile(name string, body []byte) (string, *checklist.ParsedFile, error) {
	ext := strings.ToLower(filepath.Ext(name))
	// Strip an extra .xml from `.xccdf.xml` filenames so the heuristic
	// below still picks the xccdf format up.
	if ext == ".xml" && strings.HasSuffix(strings.ToLower(name), ".xccdf.xml") {
		ext = ".xccdf"
	}

	trim := bytes.TrimLeft(body, " \t\r\n")

	switch ext {
	case ".ckl":
		pf, err := checklist.ParseCKL(bytes.NewReader(body))
		return "ckl", pf, err
	case ".cklb":
		pf, err := checklist.ParseCKLB(bytes.NewReader(body))
		return "cklb", pf, err
	case ".xccdf", ".xml":
		pf, err := checklist.ParseXCCDFResults(bytes.NewReader(body))
		return "xccdf", pf, err
	}

	// Fall back to magic-byte sniffing for files with no extension.
	if len(trim) > 0 && trim[0] == '{' {
		pf, err := checklist.ParseCKLB(bytes.NewReader(body))
		return "cklb", pf, err
	}
	if bytes.HasPrefix(trim, []byte("<?xml")) || bytes.HasPrefix(trim, []byte("<")) {
		if bytes.Contains(body, []byte("<CHECKLIST")) || bytes.Contains(body, []byte("<STIG_INFO")) {
			pf, err := checklist.ParseCKL(bytes.NewReader(body))
			return "ckl", pf, err
		}
		pf, err := checklist.ParseXCCDFResults(bytes.NewReader(body))
		return "xccdf", pf, err
	}

	return "unknown", nil, fmt.Errorf("unrecognised file format: %s", name)
}

// assetIndex is an in-memory lookup over the collection's enabled
// assets. The same Asset row may be reachable through any of name /
// fqdn / mac / ip — we de-duplicate against the underlying AssetID so
// the caller never sees ambiguous matches.
type assetIndex struct {
	byName map[string]*store.Asset
	byFQDN map[string]*store.Asset
	byMAC  map[string]*store.Asset
	byIP   map[string]*store.Asset
}

func newAssetIndex(rows []store.Asset) *assetIndex {
	idx := &assetIndex{
		byName: map[string]*store.Asset{},
		byFQDN: map[string]*store.Asset{},
		byMAC:  map[string]*store.Asset{},
		byIP:   map[string]*store.Asset{},
	}
	for i := range rows {
		a := &rows[i]
		if a.Name != "" {
			idx.byName[strings.ToLower(a.Name)] = a
		}
		if a.FQDN != "" {
			idx.byFQDN[strings.ToLower(a.FQDN)] = a
		}
		if a.MAC != "" {
			idx.byMAC[normalizeMAC(a.MAC)] = a
		}
		if a.IP != "" {
			idx.byIP[strings.ToLower(a.IP)] = a
		}
	}
	return idx
}

// Resolve attempts identity-based asset matching, with filename as a
// final fallback (DISA tooling often names files after the host).
func (i *assetIndex) Resolve(id checklist.AssetIdentity, filename string) (*store.Asset, bool) {
	if id.FQDN != "" {
		if a, ok := i.byFQDN[strings.ToLower(id.FQDN)]; ok {
			return a, true
		}
	}
	if id.HostName != "" {
		if a, ok := i.byName[strings.ToLower(id.HostName)]; ok {
			return a, true
		}
		// Some tools write FQDN into hostname; strip the first label.
		if dot := strings.Index(id.HostName, "."); dot > 0 {
			short := strings.ToLower(id.HostName[:dot])
			if a, ok := i.byName[short]; ok {
				return a, true
			}
		}
	}
	if id.MAC != "" {
		if a, ok := i.byMAC[normalizeMAC(id.MAC)]; ok {
			return a, true
		}
	}
	if id.IP != "" {
		if a, ok := i.byIP[strings.ToLower(id.IP)]; ok {
			return a, true
		}
	}
	if filename != "" {
		base := strings.ToLower(filepath.Base(filename))
		base = strings.TrimSuffix(base, filepath.Ext(base))
		// Strip a trailing ".xccdf" if the filename was foo.xccdf.xml.
		base = strings.TrimSuffix(base, ".xccdf")
		if a, ok := i.byName[base]; ok {
			return a, true
		}
	}
	return nil, false
}

// normalizeMAC strips separators and lowercases so that "aa:bb:cc"
// matches "AA-BB-CC" matches "aabbcc".
func normalizeMAC(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9',
			r >= 'a' && r <= 'f',
			r >= 'A' && r <= 'F':
			if r >= 'A' && r <= 'F' {
				r += 'a' - 'A'
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}


