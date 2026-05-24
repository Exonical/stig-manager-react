//go:build integration

package server_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/Exonical/stig-manager-react/api/internal/auth"
)

// uploadZip constructs a multipart/form-data request body with the
// given zip payload as the importFile part.
func uploadZip(t *testing.T, filename string, payload []byte) (string, []byte) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("importFile", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := io.Copy(part, bytes.NewReader(payload)); err != nil {
		t.Fatalf("copy: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close mw: %v", err)
	}
	return mw.FormDataContentType(), buf.Bytes()
}

// buildZipBytes packs a name→content map into an in-memory zip.
func buildZipBytes(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %q: %v", name, err)
		}
		if _, err := w.Write(body); err != nil {
			t.Fatalf("zip write %q: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

// TestImportBenchmarkZipFlow verifies POST /stigs accepts the DISA
// outer-zip format (a zip containing a STIG-named subfolder with a
// *xccdf.xml file and accessory PDFs/JPGs).
func TestImportBenchmarkZipFlow(t *testing.T) {
	pool := newIntegrationPool(t)

	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(context.Background(), auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	handler := newTestServer(t, withAuth(prov), withPool(pool))

	xml, err := os.ReadFile("../xccdf/testdata/sample.xccdf.xml")
	if err != nil {
		t.Fatalf("read xml fixture: %v", err)
	}

	// 1. Outer zip with a single XCCDF buried beneath accessory files
	//    (DISA "Manual STIG" bundle layout).
	disaZip := buildZipBytes(t, map[string][]byte{
		"U_TEST_OS_V1R1_STIG/U_Readme_SRG_and_STIG.pdf":              []byte("PDF placeholder"),
		"U_TEST_OS_V1R1_STIG/U_TEST_OS_V1R1_Manual-xccdf.xml":        xml,
		"U_TEST_OS_V1R1_STIG/DoD-DISA-logos-as-JPEG.jpg":             []byte("JPG placeholder"),
		"U_TEST_OS_V1R1_STIG/U_TEST_OS_V1R1_Overview.pdf":            []byte("PDF placeholder"),
	})

	ct, body := uploadZip(t, "U_TEST_OS_V1R1_STIG.zip", disaZip)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/stigs", bytes.NewReader(body))
	req.Header.Set("Content-Type", ct)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:stig"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("import disa zip: got %d (body=%s)", rec.Code, rec.Body.String())
	}
	var single map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &single); err != nil {
		t.Fatalf("decode single: %v body=%s", err, rec.Body.String())
	}
	if single["benchmarkId"] != "TEST_OS_STIG" || single["revisionStr"] != "V2R3" {
		t.Fatalf("import disa zip: %+v", single)
	}

	// 2. Nested zip layout (DISA STIG Library quarterly bundle):
	//    outer zip contains a Manifest XML + an inner _xccdf.zip,
	//    which then contains the actual XCCDF XML.
	inner := buildZipBytes(t, map[string][]byte{
		"U_TEST_OS_xccdf.xml": xml,
	})
	outer := buildZipBytes(t, map[string][]byte{
		"U_STIG_Manifest.xml": []byte("<manifest/>"),
		"U_TEST_OS_xccdf.zip": inner,
	})
	ct2, body2 := uploadZip(t, "U_STIG_Library.zip", outer)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/stigs?clobber=true", bytes.NewReader(body2))
	req.Header.Set("Content-Type", ct2)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:stig"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("import nested zip: got %d (body=%s)", rec.Code, rec.Body.String())
	}
	var nested map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &nested); err != nil {
		t.Fatalf("decode nested: %v", err)
	}
	if nested["benchmarkId"] != "TEST_OS_STIG" {
		t.Fatalf("nested import: %+v", nested)
	}

	// 3. A multi-XCCDF zip (two xccdf files at top level) returns an
	//    array under 200. We use clobber=true since the previous
	//    imports already registered TEST_OS_STIG/V2R3.
	multi := buildZipBytes(t, map[string][]byte{
		"a-xccdf.xml": xml,
		"b_xccdf.xml": xml,
	})
	ct3, body3 := uploadZip(t, "multi.zip", multi)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/stigs?clobber=true", bytes.NewReader(body3))
	req.Header.Set("Content-Type", ct3)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:stig"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("import multi zip: got %d (body=%s)", rec.Code, rec.Body.String())
	}
	var arr []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &arr); err != nil {
		t.Fatalf("decode multi: %v body=%s", err, rec.Body.String())
	}
	if len(arr) != 2 {
		t.Fatalf("multi import: want 2 entries, got %d (%+v)", len(arr), arr)
	}

	// 4. Garbage zip (no xccdf inside) → 400.
	junk := buildZipBytes(t, map[string][]byte{"readme.txt": []byte("hello")})
	ct4, body4 := uploadZip(t, "junk.zip", junk)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/stigs", bytes.NewReader(body4))
	req.Header.Set("Content-Type", ct4)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:stig"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("junk zip: got %d (body=%s)", rec.Code, rec.Body.String())
	}
}
