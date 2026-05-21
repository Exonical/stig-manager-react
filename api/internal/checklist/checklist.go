// Package checklist parses STIG-format checklist files into a
// normalized review payload ready for ReviewRepo upsert.
//
// Three formats are supported:
//
//   - CKL  — DISA STIG Viewer XML checklist (legacy).
//   - CKLB — DISA STIG Viewer JSON checklist (current).
//   - XCCDF results — `<Benchmark>` with a `<TestResult>` block, as
//     produced by OpenSCAP / Evaluate-STIG and similar engines.
//
// All three are reduced to a common `ParsedFile` containing the
// asset's identity hints (hostname, IP, MAC, FQDN) and one
// `ParsedReview` per (rule, result) pair. The mapping from each
// format's status enum to `review.result` matches upstream:
//
//	CKL   NotAFinding → pass     CKLB not_a_finding → pass
//	CKL   Open        → fail     CKLB open          → fail
//	CKL   Not_Applicable → notapplicable
//	CKL   Not_Reviewed   → notchecked
package checklist

// Result is the normalised review result enum, matching
// `review.result` in the database (and ReviewResult in the OpenAPI).
type Result string

const (
	ResultPass          Result = "pass"
	ResultFail          Result = "fail"
	ResultNotApplicable Result = "notapplicable"
	ResultNotChecked    Result = "notchecked"
	ResultInformational Result = "informational"
	ResultUnknown       Result = "unknown"
	ResultError         Result = "error"
	ResultFixed         Result = "fixed"
)

// AssetIdentity is the per-file asset metadata captured from a
// checklist's <ASSET>/target/target_data block. Importers use this to
// resolve an existing asset (by FQDN > MAC > IP > host-name, in that
// preference order).
type AssetIdentity struct {
	HostName string
	FQDN     string
	IP       string
	MAC      string
	Role     string
}

// ParsedReview is a single rule-level result extracted from the file.
type ParsedReview struct {
	// RuleID is the STIG Rule_ID (e.g. "SV-12345r1_rule"). Empty when
	// the file only carried the legacy Vuln_Num; importers should drop
	// such rows.
	RuleID  string
	Result  Result
	Detail  string
	Comment string
	// AutoResult is true when the file's source engine populated the
	// result automatically (XCCDF results, certain CKLB engines). The
	// importer surfaces it through review.auto_result.
	AutoResult bool
	// Severity is the rule's severity ("high"/"medium"/"low"); useful
	// for diagnostics but not stored on the review.
	Severity string
}

// ParsedFile holds the asset identity plus zero or more reviews and
// the benchmark+revision the reviews target.
type ParsedFile struct {
	// Format identifies the source format ("ckl"/"cklb"/"xccdf").
	Format string
	// BenchmarkID is the parsed benchmark id (e.g. an SV STIG short
	// name) when present. May be empty for CKL files with no SI_DATA
	// "stigid".
	BenchmarkID string
	// Revision is the parsed revision string (e.g. "V1R5") when
	// present.
	Revision string
	Asset    AssetIdentity
	Reviews  []ParsedReview
}
