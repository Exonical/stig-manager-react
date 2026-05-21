package checklist_test

import (
	"bytes"
	"testing"

	"github.com/Exonical/stig-manager-react/api/internal/checklist"
)

// sampleExport returns a fixture ExportData with one STIG and three
// rules covering pass/fail/notapplicable so every status code path is
// exercised.
func sampleExport() checklist.ExportData {
	return checklist.ExportData{
		Asset: checklist.ExportAsset{
			HostName:  "host-export-1",
			FQDN:      "host-export-1.example.com",
			IP:        "10.20.30.40",
			MAC:       "aa:bb:cc:dd:ee:99",
			TargetKey: "asset-99",
		},
		Stigs: []checklist.ExportStig{{
			BenchmarkID: "RHEL_9_STIG",
			RevisionStr: "V1R2",
			Version:     "1",
			Release:     "2",
			Title:       "RHEL 9 STIG",
			Rules: []checklist.ExportRule{
				{
					RuleID:     "SV-1000r1_rule",
					VersionStr: "RHEL-09-000001",
					GroupID:    "V-1000",
					Severity:   "high",
					Title:      "Rule one",
					Result:     "pass",
					Detail:     "all good",
					Comment:    "evidence attached",
					CCIs:       []string{"CCI-000001"},
				},
				{
					RuleID:     "SV-1001r1_rule",
					VersionStr: "RHEL-09-000002",
					GroupID:    "V-1001",
					Severity:   "medium",
					Title:      "Rule two",
					Result:     "fail",
					Detail:     "found a finding",
					CCIs:       []string{"CCI-000002", "CCI-000003"},
				},
				{
					RuleID:     "SV-1002r1_rule",
					VersionStr: "RHEL-09-000003",
					GroupID:    "V-1002",
					Severity:   "low",
					Title:      "Rule three",
					Result:     "notapplicable",
				},
			},
		}},
	}
}

func TestWriteCKLRoundTrip(t *testing.T) {
	data := sampleExport()
	var buf bytes.Buffer
	if err := checklist.WriteCKL(&buf, data); err != nil {
		t.Fatalf("write: %v", err)
	}
	parsed, err := checklist.ParseCKL(&buf)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if parsed.Format != "ckl" {
		t.Fatalf("format: %q", parsed.Format)
	}
	if parsed.BenchmarkID != "RHEL_9_STIG" || parsed.Revision != "V1R2" {
		t.Fatalf("benchmark/revision: %s / %s", parsed.BenchmarkID, parsed.Revision)
	}
	if parsed.Asset.HostName != "host-export-1" ||
		parsed.Asset.FQDN != "host-export-1.example.com" ||
		parsed.Asset.IP != "10.20.30.40" ||
		parsed.Asset.MAC != "aa:bb:cc:dd:ee:99" {
		t.Fatalf("asset: %+v", parsed.Asset)
	}
	want := map[string]checklist.Result{
		"SV-1000r1_rule": checklist.ResultPass,
		"SV-1001r1_rule": checklist.ResultFail,
		"SV-1002r1_rule": checklist.ResultNotApplicable,
	}
	if len(parsed.Reviews) != len(want) {
		t.Fatalf("rule count: got %d want %d", len(parsed.Reviews), len(want))
	}
	for _, r := range parsed.Reviews {
		w, ok := want[r.RuleID]
		if !ok {
			t.Fatalf("unexpected rule: %s", r.RuleID)
		}
		if r.Result != w {
			t.Fatalf("rule %s: got %s want %s", r.RuleID, r.Result, w)
		}
	}
}

func TestWriteCKLBRoundTrip(t *testing.T) {
	data := sampleExport()
	var buf bytes.Buffer
	if err := checklist.WriteCKLB(&buf, data); err != nil {
		t.Fatalf("write: %v", err)
	}
	parsed, err := checklist.ParseCKLB(&buf)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if parsed.Format != "cklb" {
		t.Fatalf("format: %q", parsed.Format)
	}
	if parsed.BenchmarkID != "RHEL_9_STIG" {
		t.Fatalf("benchmark: %s", parsed.BenchmarkID)
	}
	if parsed.Asset.FQDN != "host-export-1.example.com" {
		t.Fatalf("asset: %+v", parsed.Asset)
	}
	want := map[string]checklist.Result{
		"SV-1000r1_rule": checklist.ResultPass,
		"SV-1001r1_rule": checklist.ResultFail,
		"SV-1002r1_rule": checklist.ResultNotApplicable,
	}
	if len(parsed.Reviews) != len(want) {
		t.Fatalf("rule count: got %d want %d", len(parsed.Reviews), len(want))
	}
	for _, r := range parsed.Reviews {
		w, ok := want[r.RuleID]
		if !ok {
			t.Fatalf("unexpected rule: %s", r.RuleID)
		}
		if r.Result != w {
			t.Fatalf("rule %s: got %s want %s", r.RuleID, r.Result, w)
		}
	}
}

func TestWriteXCCDFResultsRoundTrip(t *testing.T) {
	data := sampleExport()
	var buf bytes.Buffer
	if err := checklist.WriteXCCDFResults(&buf, data); err != nil {
		t.Fatalf("write: %v", err)
	}
	parsed, err := checklist.ParseXCCDFResults(&buf)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if parsed.Format != "xccdf" {
		t.Fatalf("format: %q", parsed.Format)
	}
	if parsed.Asset.HostName != "host-export-1" {
		t.Fatalf("asset: %+v", parsed.Asset)
	}
	want := map[string]checklist.Result{
		"SV-1000r1_rule": checklist.ResultPass,
		"SV-1001r1_rule": checklist.ResultFail,
		"SV-1002r1_rule": checklist.ResultNotApplicable,
	}
	if len(parsed.Reviews) != len(want) {
		t.Fatalf("rule count: got %d want %d", len(parsed.Reviews), len(want))
	}
	for _, r := range parsed.Reviews {
		w, ok := want[r.RuleID]
		if !ok {
			t.Fatalf("unexpected rule: %s", r.RuleID)
		}
		if r.Result != w {
			t.Fatalf("rule %s: got %s want %s", r.RuleID, r.Result, w)
		}
		if !r.AutoResult {
			t.Fatalf("rule %s: expected AutoResult true", r.RuleID)
		}
	}
}

func TestWriteCKLEmptyResultIsNotReviewed(t *testing.T) {
	data := checklist.ExportData{
		Stigs: []checklist.ExportStig{{
			BenchmarkID: "TEST_STIG", RevisionStr: "V1R1",
			Rules: []checklist.ExportRule{{RuleID: "SV-9000r1_rule"}},
		}},
	}
	var buf bytes.Buffer
	if err := checklist.WriteCKL(&buf, data); err != nil {
		t.Fatalf("write: %v", err)
	}
	parsed, err := checklist.ParseCKL(&buf)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed.Reviews) != 1 {
		t.Fatalf("rule count: %d", len(parsed.Reviews))
	}
	if parsed.Reviews[0].Result != checklist.ResultNotChecked {
		t.Fatalf("expected ResultNotChecked, got %s", parsed.Reviews[0].Result)
	}
}
