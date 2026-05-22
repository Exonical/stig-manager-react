package xccdf

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// buildZip helps tests build in-memory zips with explicit entry names.
func buildZip(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create %q: %v", name, err)
		}
		if _, err := w.Write(body); err != nil {
			t.Fatalf("write %q: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zw: %v", err)
	}
	return buf.Bytes()
}

func readFixtureXML(t *testing.T) []byte {
	t.Helper()
	f, err := os.Open("testdata/sample.xccdf.xml")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return b
}

func TestIsZip(t *testing.T) {
	if !IsZip([]byte{0x50, 0x4b, 0x03, 0x04, 0x00}) {
		t.Errorf("expected zip prefix to match")
	}
	if IsZip([]byte("<?xml")) {
		t.Errorf("xml should not look like a zip")
	}
	if IsZip(nil) {
		t.Errorf("nil should not be a zip")
	}
}

func TestExtractBenchmarks_RawXML(t *testing.T) {
	payload := readFixtureXML(t)
	out, err := ExtractBenchmarks(payload)
	if err != nil {
		t.Fatalf("extract raw xml: %v", err)
	}
	if len(out) != 1 || out[0].Benchmark.BenchmarkID != "TEST_OS_STIG" {
		t.Fatalf("raw xml result: %+v", out)
	}
}

func TestExtractBenchmarks_DISAOuterZip(t *testing.T) {
	// Mimic the DISA bundle layout: outer zip with the XCCDF inside
	// a STIG-named subdirectory plus assorted PDFs/JPGs.
	zipBytes := buildZip(t, map[string][]byte{
		"U_RHEL_10_V1R1_STIG/U_Readme_SRG_and_STIG.pdf":              []byte("PDF placeholder"),
		"U_RHEL_10_V1R1_STIG/U_RHEL_10_V1R1_Manual-xccdf.xml":        readFixtureXML(t),
		"U_RHEL_10_V1R1_STIG/DoD-DISA-logos-as-JPEG.jpg":             []byte("JPG placeholder"),
		"U_RHEL_10_V1R1_STIG/U_RHEL_10_V1R1_Manual_STIG/STIG.xsl":    []byte("<xsl/>"),
	})

	out, err := ExtractBenchmarks(zipBytes)
	if err != nil {
		t.Fatalf("extract disa zip: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("want 1 benchmark, got %d", len(out))
	}
	if out[0].Benchmark.BenchmarkID != "TEST_OS_STIG" {
		t.Errorf("benchmarkId: got %q", out[0].Benchmark.BenchmarkID)
	}
	if !strings.HasSuffix(out[0].Source, "Manual-xccdf.xml") {
		t.Errorf("source: got %q", out[0].Source)
	}
}

func TestExtractBenchmarks_NestedXccdfZip(t *testing.T) {
	// DISA STIG Library quarterly bundle layout: outer zip contains
	// a Manifest XML and an inner `_xccdf.zip`, which then contains
	// the actual XCCDF XML.
	inner := buildZip(t, map[string][]byte{
		"U_TEST_OS_xccdf.xml": readFixtureXML(t),
	})
	outer := buildZip(t, map[string][]byte{
		"U_STIG_Manifest.xml": []byte("<manifest/>"),
		"U_TEST_OS_xccdf.zip": inner,
	})

	out, err := ExtractBenchmarks(outer)
	if err != nil {
		t.Fatalf("extract nested zip: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("want 1 benchmark, got %d", len(out))
	}
	if out[0].Benchmark.BenchmarkID != "TEST_OS_STIG" {
		t.Errorf("benchmarkId: got %q", out[0].Benchmark.BenchmarkID)
	}
}

func TestExtractBenchmarks_MultipleXccdfFiles(t *testing.T) {
	// A bundle with two XCCDF entries should return both.
	zipBytes := buildZip(t, map[string][]byte{
		"a-xccdf.xml": readFixtureXML(t),
		"b_xccdf.xml": readFixtureXML(t),
	})
	out, err := ExtractBenchmarks(zipBytes)
	if err != nil {
		t.Fatalf("extract multi: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("want 2 benchmarks, got %d", len(out))
	}
}

func TestExtractBenchmarks_NoXccdfFound(t *testing.T) {
	zipBytes := buildZip(t, map[string][]byte{
		"readme.txt": []byte("hello"),
		"data.json":  []byte("{}"),
	})
	_, err := ExtractBenchmarks(zipBytes)
	if err == nil {
		t.Fatalf("expected an error for zip with no xccdf")
	}
	if !strings.Contains(err.Error(), "no XCCDF") {
		t.Errorf("want 'no XCCDF' err, got %v", err)
	}
}

func TestExtractBenchmarks_EmptyInput(t *testing.T) {
	_, err := ExtractBenchmarks(nil)
	if err == nil {
		t.Fatalf("expected error for empty input")
	}
}

func TestExtractBenchmarks_RawInvalidXML(t *testing.T) {
	_, err := ExtractBenchmarks([]byte("<not><a></valid></benchmark>"))
	if err == nil {
		t.Fatalf("expected error for bad xml")
	}
	if !strings.Contains(err.Error(), "parse xccdf") {
		t.Errorf("want wrapped 'parse xccdf' error, got %v", err)
	}
}
