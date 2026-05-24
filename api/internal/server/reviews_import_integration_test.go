//go:build integration

package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Exonical/stig-manager-react/api/internal/auth"
	"github.com/Exonical/stig-manager-react/api/internal/store"
)

// seedAssetForReviewsNamed mirrors seedAssetForReviews but with a
// caller-supplied asset name so the import path can match against an
// existing collection asset by its case-insensitive hostname.
func seedAssetForReviewsNamed(t *testing.T, pool *pgxpool.Pool, subject, username, assetName string) (collectionID, assetID int64) {
	t.Helper()
	_, collectionID = seedCollectionWithOwner(t, pool, subject, username)
	assets := store.NewAssetRepo(pool)
	a, err := assets.Create(context.Background(), store.AssetCreate{
		CollectionID: collectionID,
		Name:         assetName,
		Noncomputing: false,
	})
	if err != nil {
		t.Fatalf("create asset: %v", err)
	}
	return collectionID, a.AssetID
}

// buildImportMultipart returns a multipart/form-data body containing
// every (filename, payload) pair plus the dryRun field.
func buildImportMultipart(t *testing.T, files map[string][]byte, dryRun bool) (string, []byte) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for name, payload := range files {
		w, err := mw.CreateFormFile("files", name)
		if err != nil {
			t.Fatalf("multipart field: %v", err)
		}
		if _, err := io.Copy(w, bytes.NewReader(payload)); err != nil {
			t.Fatalf("multipart copy: %v", err)
		}
	}
	if dryRun {
		_ = mw.WriteField("dryRun", "true")
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("multipart close: %v", err)
	}
	return mw.FormDataContentType(), buf.Bytes()
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return b
}

// TestImportReviewsByCollection_CKL covers the dry-run + apply path
// for a CKL fixture, including the second-call updated bookkeeping.
func TestImportReviewsByCollection_CKL(t *testing.T) {
	pool := newIntegrationPool(t)
	collID, _ := seedAssetForReviewsNamed(t, pool, "user-1", "alice", "host-ckl-1")
	collStr := strconv.FormatInt(collID, 10)

	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(context.Background(), auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	handler := newTestServer(t, withAuth(prov), withPool(pool))

	payload := readFile(t, "../checklist/testdata/sample.ckl")

	// Dry-run first.
	ct, body := buildImportMultipart(t, map[string][]byte{"host-ckl-1.ckl": payload}, true)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/collections/"+collStr+"/reviews/import", bytes.NewReader(body))
	req.Header.Set("Content-Type", ct)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("dry-run: got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["dryRun"] != true {
		t.Fatalf("dry-run flag: %v", resp["dryRun"])
	}
	totals, _ := resp["totals"].(map[string]any)
	if got := totals["willInsert"]; got.(float64) <= 0 {
		t.Fatalf("dry-run willInsert: %v", got)
	}
	if got := totals["inserted"]; got.(float64) != 0 {
		t.Fatalf("dry-run inserted: %v", got)
	}
	if got := totals["updated"]; got.(float64) != 0 {
		t.Fatalf("dry-run updated: %v", got)
	}

	// Apply.
	ct, body = buildImportMultipart(t, map[string][]byte{"host-ckl-1.ckl": payload}, false)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/collections/"+collStr+"/reviews/import", bytes.NewReader(body))
	req.Header.Set("Content-Type", ct)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("apply: got %d body=%s", rec.Code, rec.Body.String())
	}
	resp = map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	totals, _ = resp["totals"].(map[string]any)
	if got := totals["inserted"]; got.(float64) <= 0 {
		t.Fatalf("apply inserted: %v", got)
	}

	// Re-apply: should become updates.
	ct, body = buildImportMultipart(t, map[string][]byte{"host-ckl-1.ckl": payload}, false)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/collections/"+collStr+"/reviews/import", bytes.NewReader(body))
	req.Header.Set("Content-Type", ct)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("re-apply: got %d body=%s", rec.Code, rec.Body.String())
	}
	resp = map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	totals, _ = resp["totals"].(map[string]any)
	if got := totals["updated"]; got.(float64) <= 0 {
		t.Fatalf("re-apply updated: %v", got)
	}
	if got := totals["inserted"]; got.(float64) != 0 {
		t.Fatalf("re-apply inserted (should be 0): %v", got)
	}
}

// TestImportReviewsByCollection_CKLB validates the JSON CKLB format.
func TestImportReviewsByCollection_CKLB(t *testing.T) {
	pool := newIntegrationPool(t)
	collID, _ := seedAssetForReviewsNamed(t, pool, "user-1", "alice", "host-cklb-1")
	collStr := strconv.FormatInt(collID, 10)

	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(context.Background(), auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	handler := newTestServer(t, withAuth(prov), withPool(pool))

	payload := readFile(t, "../checklist/testdata/sample.cklb")
	ct, body := buildImportMultipart(t, map[string][]byte{"host-cklb-1.cklb": payload}, false)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/collections/"+collStr+"/reviews/import", bytes.NewReader(body))
	req.Header.Set("Content-Type", ct)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("apply: got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	files, _ := resp["files"].([]any)
	if len(files) != 1 {
		t.Fatalf("files: %d", len(files))
	}
	row, _ := files[0].(map[string]any)
	if row["format"] != "cklb" {
		t.Fatalf("format: %v", row["format"])
	}
	if row["assetName"] != "host-cklb-1" {
		t.Fatalf("assetName: %v", row["assetName"])
	}
}

// TestImportReviewsByCollection_XCCDF validates the XCCDF results
// format and asset resolution by FQDN.
func TestImportReviewsByCollection_XCCDF(t *testing.T) {
	pool := newIntegrationPool(t)
	collID, _ := seedAssetForReviewsNamed(t, pool, "user-1", "alice", "host-xccdf-1")
	collStr := strconv.FormatInt(collID, 10)

	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(context.Background(), auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	handler := newTestServer(t, withAuth(prov), withPool(pool))

	payload := readFile(t, "../checklist/testdata/sample.xccdf.xml")
	ct, body := buildImportMultipart(t, map[string][]byte{"host-xccdf-1.xccdf.xml": payload}, false)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/collections/"+collStr+"/reviews/import", bytes.NewReader(body))
	req.Header.Set("Content-Type", ct)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("apply: got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	files, _ := resp["files"].([]any)
	row, _ := files[0].(map[string]any)
	if row["format"] != "xccdf" {
		t.Fatalf("format: %v", row["format"])
	}
}

// TestImportReviewsByCollection_UnknownAsset asserts that a file
// targeting a hostname that has no asset in the collection lands in
// the per-file diagnostic without aborting the whole request.
func TestImportReviewsByCollection_UnknownAsset(t *testing.T) {
	pool := newIntegrationPool(t)
	collID, _ := seedAssetForReviewsNamed(t, pool, "user-1", "alice", "some-other-host")
	collStr := strconv.FormatInt(collID, 10)

	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(context.Background(), auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	handler := newTestServer(t, withAuth(prov), withPool(pool))

	payload := readFile(t, "../checklist/testdata/sample.ckl")
	ct, body := buildImportMultipart(t, map[string][]byte{"unmatched.ckl": payload}, false)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/collections/"+collStr+"/reviews/import", bytes.NewReader(body))
	req.Header.Set("Content-Type", ct)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("apply: got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	totals, _ := resp["totals"].(map[string]any)
	if got := totals["inserted"]; got.(float64) != 0 {
		t.Fatalf("inserted should be 0 for unmatched: %v", got)
	}
	files, _ := resp["files"].([]any)
	row, _ := files[0].(map[string]any)
	if row["error"] == nil || row["error"] == "" {
		t.Fatalf("expected per-file error: %v", row)
	}
}

// TestImportReviewsByCollection_AuthErrors covers the auth gates:
// missing scope and read-only role both yield 4xx.
func TestImportReviewsByCollection_AuthErrors(t *testing.T) {
	pool := newIntegrationPool(t)
	collID, _ := seedAssetForReviewsNamed(t, pool, "user-1", "alice", "host-ckl-1")
	collStr := strconv.FormatInt(collID, 10)

	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(context.Background(), auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	handler := newTestServer(t, withAuth(prov), withPool(pool))

	payload := readFile(t, "../checklist/testdata/sample.ckl")
	ct, body := buildImportMultipart(t, map[string][]byte{"host-ckl-1.ckl": payload}, false)

	// No auth → 401.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/collections/"+collStr+"/reviews/import", bytes.NewReader(body))
	req.Header.Set("Content-Type", ct)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-auth: got %d body=%s", rec.Code, rec.Body.String())
	}

	// Read-only scope but write attempt → 403.
	ct2, body2 := buildImportMultipart(t, map[string][]byte{"host-ckl-1.ckl": payload}, false)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/collections/"+collStr+"/reviews/import", bytes.NewReader(body2))
	req.Header.Set("Content-Type", ct2)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("read-only scope: got %d body=%s", rec.Code, rec.Body.String())
	}
}
