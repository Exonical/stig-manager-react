//go:build integration

package store_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Exonical/stig-manager-react/api/internal/store"
	"github.com/Exonical/stig-manager-react/api/internal/xccdf"
)

// readSampleBenchmark loads the parser fixture that ships with the
// xccdf package. The integration suite reuses it so the store-layer
// tests exercise the same shape we expect at runtime.
func readSampleBenchmark(t *testing.T) *xccdf.Benchmark {
	t.Helper()
	f, err := os.Open("../xccdf/testdata/sample.xccdf.xml")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()
	b, err := xccdf.Parse(f)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return b
}

// TestSTIGRepo_ImportListGet covers the happy path for importing a
// STIG, listing the library, and reading a single STIG back. It also
// verifies the duplicate-revision behaviour around the `clobber` flag.
func TestSTIGRepo_ImportListGet(t *testing.T) {
	pool := newPGPool(t)
	ctx := context.Background()
	repo := store.NewSTIGRepo(pool)

	bench := readSampleBenchmark(t)

	rev, err := repo.ImportRevision(ctx, bench, false)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	if rev.BenchmarkID != "TEST_OS_STIG" || rev.RevisionStr != "V2R3" {
		t.Fatalf("revision summary: %+v", rev)
	}
	if rev.RuleCount != 2 {
		t.Fatalf("rule count: got %d want 2", rev.RuleCount)
	}

	// Re-importing the same revision without clobber must return
	// ErrDuplicateName — the API translates this to 400.
	if _, err := repo.ImportRevision(ctx, bench, false); !errors.Is(err, store.ErrDuplicateName) {
		t.Fatalf("second import (no clobber): got %v want ErrDuplicateName", err)
	}

	// With clobber, the revision and its rules are replaced. The
	// row count must remain 2 (matching the fixture).
	if _, err := repo.ImportRevision(ctx, bench, true); err != nil {
		t.Fatalf("clobber import: %v", err)
	}

	list, err := repo.List(ctx, store.ListSTIGsOptions{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list: got %d entries want 1", len(list))
	}
	if list[0].BenchmarkID != "TEST_OS_STIG" || list[0].RuleCount != 2 {
		t.Fatalf("list[0]: %+v", list[0])
	}
	if list[0].LastRevisionStr != "V2R3" {
		t.Fatalf("list[0].LastRevisionStr: got %q want V2R3", list[0].LastRevisionStr)
	}

	// Title filter is case-insensitive substring.
	filtered, err := repo.List(ctx, store.ListSTIGsOptions{TitleContains: "operating system"})
	if err != nil {
		t.Fatalf("filtered list: %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("filtered list: got %d want 1", len(filtered))
	}
	none, err := repo.List(ctx, store.ListSTIGsOptions{TitleContains: "nothing"})
	if err != nil {
		t.Fatalf("none list: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("none list: got %d want 0", len(none))
	}

	got, err := repo.Get(ctx, "TEST_OS_STIG")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.LastRevisionStr != "V2R3" || got.RuleCount != 2 {
		t.Fatalf("get: %+v", got)
	}
	if len(got.RevisionStrs) != 1 || got.RevisionStrs[0] != "V2R3" {
		t.Fatalf("get.RevisionStrs: %+v", got.RevisionStrs)
	}

	if _, err := repo.Get(ctx, "MISSING"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("get missing: got %v want ErrNotFound", err)
	}
}

// TestSTIGRepo_GetRuleAndCci exercises the rule + cci lookup endpoints,
// including the cross-link projection that returns the STIGs which
// reference a given CCI.
func TestSTIGRepo_GetRuleAndCci(t *testing.T) {
	pool := newPGPool(t)
	ctx := context.Background()
	repo := store.NewSTIGRepo(pool)

	bench := readSampleBenchmark(t)
	if _, err := repo.ImportRevision(ctx, bench, false); err != nil {
		t.Fatalf("import: %v", err)
	}

	rule, err := repo.GetRuleByRuleID(ctx, "SV-100001r1_rule")
	if err != nil {
		t.Fatalf("get rule: %v", err)
	}
	if rule.Severity != "medium" {
		t.Fatalf("rule severity: got %q want medium", rule.Severity)
	}
	if rule.BenchmarkID != "TEST_OS_STIG" || rule.RevisionStr != "V2R3" {
		t.Fatalf("rule provenance: %+v", rule)
	}
	if !strings.Contains(rule.CheckContent, "pwquality") {
		t.Fatalf("rule check content: %q", rule.CheckContent)
	}
	if len(rule.CCIs) != 2 {
		t.Fatalf("rule ccis: %+v", rule.CCIs)
	}

	if _, err := repo.GetRuleByRuleID(ctx, "SV-missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing rule: got %v want ErrNotFound", err)
	}

	cci, err := repo.GetCCI(ctx, "CCI-000366")
	if err != nil {
		t.Fatalf("get cci: %v", err)
	}
	if cci.CCI != "CCI-000366" {
		t.Fatalf("cci.CCI: %q", cci.CCI)
	}
	if len(cci.Stigs) != 1 || cci.Stigs[0].BenchmarkID != "TEST_OS_STIG" {
		t.Fatalf("cci.Stigs: %+v", cci.Stigs)
	}

	if _, err := repo.GetCCI(ctx, "CCI-999999"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing cci: got %v want ErrNotFound", err)
	}
}
