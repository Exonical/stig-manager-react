package checklist_test

import (
	"os"
	"testing"

	"github.com/Exonical/stig-manager-react/api/internal/checklist"
)

func TestParseCKL(t *testing.T) {
	f, err := os.Open("testdata/sample.ckl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := checklist.ParseCKL(f)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Format != "ckl" {
		t.Fatalf("format: %q", got.Format)
	}
	if got.BenchmarkID != "RHEL_9_STIG" || got.Revision != "V1R2" {
		t.Fatalf("benchmark: %+v / %+v", got.BenchmarkID, got.Revision)
	}
	if got.Asset.HostName != "host-ckl-1" ||
		got.Asset.FQDN != "host-ckl-1.example.com" ||
		got.Asset.IP != "10.0.0.10" ||
		got.Asset.MAC != "aa:bb:cc:dd:ee:01" {
		t.Fatalf("asset: %+v", got.Asset)
	}
	// 4 VULNs but one has no Rule_ID, so 3 reviews.
	if len(got.Reviews) != 3 {
		t.Fatalf("reviews: got %d want 3 (%+v)", len(got.Reviews), got.Reviews)
	}
	wantResults := map[string]checklist.Result{
		"SV-1000r1_rule": checklist.ResultPass,
		"SV-1001r1_rule": checklist.ResultFail,
		"SV-1002r1_rule": checklist.ResultNotApplicable,
	}
	for _, r := range got.Reviews {
		if want, ok := wantResults[r.RuleID]; !ok {
			t.Fatalf("unexpected rule: %s", r.RuleID)
		} else if want != r.Result {
			t.Fatalf("rule %s: got %s want %s", r.RuleID, r.Result, want)
		}
	}
}

func TestParseCKLB(t *testing.T) {
	f, err := os.Open("testdata/sample.cklb")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := checklist.ParseCKLB(f)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Format != "cklb" {
		t.Fatalf("format: %q", got.Format)
	}
	if got.BenchmarkID != "RHEL_9_STIG" || got.Revision != "V1R2" {
		t.Fatalf("benchmark: %+v / %+v", got.BenchmarkID, got.Revision)
	}
	if got.Asset.FQDN != "host-cklb-1.example.com" {
		t.Fatalf("asset: %+v", got.Asset)
	}
	// 4 rules: one missing both rule_id and rule_id_src is dropped.
	if len(got.Reviews) != 3 {
		t.Fatalf("reviews: got %d want 3 (%+v)", len(got.Reviews), got.Reviews)
	}
	wantResults := map[string]checklist.Result{
		"SV-2000r1_rule": checklist.ResultPass,
		"SV-2001r1_rule": checklist.ResultFail,
		"SV-2002r1_rule": checklist.ResultNotApplicable,
	}
	for _, r := range got.Reviews {
		if want, ok := wantResults[r.RuleID]; !ok {
			t.Fatalf("unexpected rule: %s", r.RuleID)
		} else if want != r.Result {
			t.Fatalf("rule %s: got %s want %s", r.RuleID, r.Result, want)
		}
	}
}

func TestParseXCCDFResults(t *testing.T) {
	f, err := os.Open("testdata/sample.xccdf.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := checklist.ParseXCCDFResults(f)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Format != "xccdf" {
		t.Fatalf("format: %q", got.Format)
	}
	if got.Asset.HostName != "host-xccdf-1" ||
		got.Asset.FQDN != "host-xccdf-1.example.com" ||
		got.Asset.IP != "10.0.0.30" {
		t.Fatalf("asset: %+v", got.Asset)
	}
	if len(got.Reviews) != 4 {
		t.Fatalf("reviews: got %d want 4", len(got.Reviews))
	}
	wantResults := map[string]checklist.Result{
		"SV-3000r1_rule": checklist.ResultPass,
		"SV-3001r1_rule": checklist.ResultFail,
		"SV-3002r1_rule": checklist.ResultNotApplicable,
		"SV-3003r1_rule": checklist.ResultNotChecked,
	}
	for _, r := range got.Reviews {
		if want, ok := wantResults[r.RuleID]; !ok {
			t.Fatalf("unexpected rule: %s", r.RuleID)
		} else if want != r.Result {
			t.Fatalf("rule %s: got %s want %s", r.RuleID, r.Result, want)
		}
		if !r.AutoResult {
			t.Fatalf("xccdf rule %s: AutoResult should be true", r.RuleID)
		}
	}
}
