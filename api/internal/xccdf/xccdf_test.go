package xccdf

import (
	"os"
	"strings"
	"testing"
)

func TestParse_SampleBenchmark(t *testing.T) {
	f, err := os.Open("testdata/sample.xccdf.xml")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	b, err := Parse(f)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if b.BenchmarkID != "TEST_OS_STIG" {
		t.Errorf("BenchmarkID: got %q want TEST_OS_STIG", b.BenchmarkID)
	}
	if b.Title == "" {
		t.Errorf("Title: empty")
	}
	if b.Version != "2" || b.Release != "3" {
		t.Errorf("Version/Release: got %q/%q want 2/3", b.Version, b.Release)
	}
	if got := b.RevisionStr(); got != "V2R3" {
		t.Errorf("RevisionStr: got %q want V2R3", got)
	}
	if b.BenchmarkDate.IsZero() {
		t.Errorf("BenchmarkDate: zero")
	}
	if b.Status != "accepted" {
		t.Errorf("Status: got %q want accepted", b.Status)
	}
	if b.Marking != "Distribution Statement A." {
		t.Errorf("Marking: got %q", b.Marking)
	}
	if len(b.Rules) != 2 {
		t.Fatalf("Rules: got %d want 2", len(b.Rules))
	}
	r1 := b.Rules[0]
	if r1.RuleID != "SV-100001r1_rule" || r1.Severity != "medium" {
		t.Errorf("Rule[0]: %+v", r1)
	}
	if r1.GroupID != "V-100001" || r1.GroupTitle != "SRG-OS-000001" {
		t.Errorf("Rule[0] group: %+v", r1)
	}
	if r1.CheckSystem == "" || r1.CheckContent == "" {
		t.Errorf("Rule[0] check empty: %+v", r1)
	}
	if r1.FixText == "" || r1.FixID == "" {
		t.Errorf("Rule[0] fix empty: %+v", r1)
	}
	if len(r1.CCIs) != 2 || r1.CCIs[0] != "CCI-000366" || r1.CCIs[1] != "CCI-000048" {
		t.Errorf("Rule[0] CCIs: %+v", r1.CCIs)
	}

	r2 := b.Rules[1]
	if r2.Severity != "high" {
		t.Errorf("Rule[1] severity: got %q want high", r2.Severity)
	}
	if len(r2.CCIs) != 1 || r2.CCIs[0] != "CCI-000366" {
		t.Errorf("Rule[1] CCIs: %+v", r2.CCIs)
	}
}

func TestParse_RejectsNonBenchmark(t *testing.T) {
	bogus := `<?xml version="1.0"?><foo></foo>`
	if _, err := Parse(strings.NewReader(bogus)); err == nil {
		t.Errorf("Parse: expected error on non-Benchmark root")
	}
}

func TestCanonicalBenchmarkID(t *testing.T) {
	cases := map[string]string{
		"xccdf_mil.disa.stig_benchmark_RHEL_8_STIG": "RHEL_8_STIG",
		"RHEL_8_STIG":              "RHEL_8_STIG",
		"  RHEL_8_STIG  ":          "RHEL_8_STIG",
	}
	for in, want := range cases {
		if got := canonicalBenchmarkID(in); got != want {
			t.Errorf("canonicalBenchmarkID(%q): got %q want %q", in, got, want)
		}
	}
}

func TestExtractCCIs_Normalises(t *testing.T) {
	idents := []rawIdent{
		{System: "http://cyber.mil/cci", Text: "cci-000048"},
		{System: "http://cyber.mil/cci", Text: "CCI000048"}, // dup, missing dash
		{System: "http://cyber.mil/cci", Text: "  CCI-000366  "},
		{System: "http://cyber.mil/cci", Text: ""},
		{System: "x", Text: "garbage"},
	}
	got := extractCCIs(idents)
	if len(got) != 2 || got[0] != "CCI-000048" || got[1] != "CCI-000366" {
		t.Errorf("extractCCIs: %+v", got)
	}
}


