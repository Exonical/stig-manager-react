// Package poam serialises STIG findings into the two
// xlsx-based POA&M templates the upstream API supports:
//
//   - EMASS: the eMASS Plan of Action & Milestones template
//   - MCCAST: the MCCAST POA&M Vulnerabilities template
//
// Both formats are produced from the same Finding slice so the
// downstream HTTP handler can pick the format at request time.
package poam

import (
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

// Finding is one row of the POA&M spreadsheet. It corresponds to
// either a Group (groupId aggregator) or a Rule (ruleId aggregator)
// rolled up across every Asset in a Collection that has at least
// one "fail" / "Open" review on it.
type Finding struct {
	// GroupID is the V-number ("V-220706") when the spreadsheet
	// aggregates by group; empty when aggregator=ruleId.
	GroupID string
	// RuleID is the SV-number ("SV-258023r926943") when the spreadsheet
	// aggregates by rule; empty when aggregator=groupId.
	RuleID string
	// Title is the group title (groupId aggregator) or the rule title
	// (ruleId aggregator). Used verbatim in the "Description" column
	// alongside the first rule's vuln discussion.
	Title string
	// Severity is the highest severity across all rules covered by the
	// finding ("low", "medium", "high"). "unknown" indicates the
	// underlying STIG didn't classify the rule.
	Severity string
	// Rules carries the (ruleId, title, severity, vulnDiscussion) tuples
	// that contributed to this finding. The serialiser uses the first
	// entry for the Description column.
	Rules []Rule
	// Assets is the deduplicated set of (assetId, name) pairs whose
	// most-recent review on this finding was "fail". The serialiser
	// joins names with newlines into the "Assets Affected" column.
	Assets []Asset
	// Stigs is the deduplicated set of (benchmarkId, revisionStr,
	// benchmarkDate) tuples contributing rules to this finding.
	Stigs []Stig
	// CCIs is the deduplicated set of Control Correlation Identifiers
	// tied to this finding's rules.
	CCIs []CCI
}

// Rule is the per-rule tuple a Finding carries for serialisation.
type Rule struct {
	RuleID         string
	Title          string
	Severity       string
	VulnDiscussion string
}

// Asset is the per-asset tuple a Finding carries.
type Asset struct {
	AssetID string
	Name    string
}

// Stig is the per-benchmark tuple a Finding carries.
type Stig struct {
	BenchmarkID   string
	RevisionStr   string
	BenchmarkDate string
}

// CCI is the per-CCI tuple a Finding carries.
type CCI struct {
	CCI        string
	Definition string
	// APAcronym is the NIST 800-53 control acronym (e.g. "AC-2(1)")
	// once the upstream CCI XML import lands. For now this is empty
	// and the serialiser falls back to the CCI identifier alone.
	APAcronym string
	// Control is the parent control identifier (e.g. "AC-2") for the
	// same reason; empty until the CCI parent-control table is
	// populated.
	Control string
}

// Defaults captures the URL-query defaults that flow into the
// EMASS / MCCAST templates' columns. Every field is optional;
// empty strings produce empty cells.
type Defaults struct {
	Date            string
	Office          string
	Status          string
	MccastPackageID string
	MccastAuthName  string
}

// Format chooses which xlsx layout to emit.
type Format string

const (
	// FormatEMASS is the eMASS Plan of Action & Milestones template
	// (default; ~22 columns).
	FormatEMASS Format = "EMASS"
	// FormatMCCAST is the MCCAST POA&M Vulnerabilities template
	// (~17 columns; different control field semantics).
	FormatMCCAST Format = "MCCAST"
)

// EMASS column headers. Order matches the upstream xlsx template
// (`api/source/utils/poam-template.xlsx`); we render the headers
// directly into row 1 of the workbook rather than substituting into
// a binary template so the implementation has zero on-disk asset
// dependencies.
var emassHeaders = []string{
	"Control Vulnerability Description",
	"Security Control Number (NC/NA controls only)",
	"Office/Org",
	"Security Checks",
	"Resources Required",
	"Scheduled Completion Date",
	"Milestone with Completion Dates",
	"Milestone Changes",
	"Source Identifying Vulnerability",
	"Status",
	"Comments",
	"Raw Severity Value",
	"Mitigations (in-house and Inherited)",
	"Predisposing Conditions",
	"Severity Value",
	"Relevance of Threat",
	"Threat Description",
	"Likelihood",
	"Impact",
	"Impact Description",
	"Residual Risk Level",
	"Recommendations",
	"Resulting Residual Risk after Proposed Mitigations",
	"Assets Affected",
}

// MCCAST column headers. Order matches `poam-template-mccast.xlsx`.
var mccastHeaders = []string{
	"Authorization Package",
	"Vulnerability Name",
	"Vulnerability Data ID",
	"STIG Info",
	"Status",
	"Authorization Package ID",
	"Date Identified",
	"Mitigation Start Date",
	"Mitigation End Date",
	"Security Checks",
	"Security Control Number",
	"Resulting Risk Level",
	"Weakness Description",
	"Mitigations",
	"Comments",
	"Assets Affected",
	"Modified Attack Vector",
	"Modified Attack Complexity",
	"Modified Privileges Required",
	"Modified User Interaction",
	"Modified Scope",
	"Modified Confidentiality",
	"Modified Integrity",
	"Modified Availability",
}

// rawSeverity maps the upstream "low/medium/high" severity vocabulary
// onto the EMASS raw severity codes ("I", "II", "III"). Unknown / mixed
// severities collapse to "Mixed".
func rawSeverity(sev string) string {
	switch sev {
	case "low":
		return "III"
	case "medium":
		return "II"
	case "high":
		return "I"
	default:
		return "Mixed"
	}
}

// titleSeverity capitalises "low" / "medium" / "high" for the
// downstream Severity Value columns. "medium" expands to "Moderate"
// to match the upstream serialiser exactly.
func titleSeverity(sev string) string {
	switch sev {
	case "medium":
		return "Moderate"
	case "low":
		return "Low"
	case "high":
		return "High"
	default:
		return strings.Title(sev) //nolint:staticcheck // upstream parity
	}
}

// emassDescription renders the "Control Vulnerability Description"
// cell — a multi-line string containing the (first) rule's title and
// vuln discussion.
func emassDescription(f Finding) string {
	title := f.Title
	disc := ""
	if len(f.Rules) > 0 {
		if title == "" {
			title = f.Rules[0].Title
		}
		disc = f.Rules[0].VulnDiscussion
	}
	return "Title:\n" + title + "\n\nDescription:\n" + disc
}

// joinAssets renders the "Assets Affected" cell — one asset name per
// line, alphabetical-ish (the store layer already orders by name).
func joinAssets(assets []Asset) string {
	names := make([]string, 0, len(assets))
	for _, a := range assets {
		if a.Name == "" {
			continue
		}
		names = append(names, a.Name)
	}
	return strings.Join(names, "\n")
}

// joinCCIComments renders the "Comments" cell for EMASS — CCI IDs in
// the upstream "CCI-XXXXXX" form, one per line.
func joinCCIComments(ccis []CCI) string {
	out := make([]string, 0, len(ccis))
	for _, c := range ccis {
		if c.CCI == "" {
			continue
		}
		// CCIs in our store are stored as the bare "CCI-XXXXXX"
		// identifier; emit them verbatim so re-importing the xlsx
		// keeps round-trip equality.
		out = append(out, c.CCI)
	}
	return strings.Join(out, "\n")
}

// joinControls renders the "Security Control Number" cell — the NIST
// 800-53 control acronym for each CCI, deduplicated. When the CCI
// import didn't carry an acronym (current state) the column is left
// blank rather than echoing the CCI ID, since the column is a control
// reference not a CCI reference.
func joinControls(ccis []CCI) string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(ccis))
	for _, c := range ccis {
		if c.APAcronym == "" {
			continue
		}
		if _, ok := seen[c.APAcronym]; ok {
			continue
		}
		seen[c.APAcronym] = struct{}{}
		out = append(out, c.APAcronym)
	}
	return strings.Join(out, "\n")
}

// joinStigInfo renders the "Source Identifying Vulnerability" cell —
// each contributing STIG's identifier + revision + benchmark date.
func joinStigInfo(stigs []Stig) string {
	out := make([]string, 0, len(stigs))
	for _, s := range stigs {
		entry := s.BenchmarkID
		if s.RevisionStr != "" {
			entry += "\n" + s.RevisionStr
		}
		if s.BenchmarkDate != "" {
			entry += "\nBenchmark Date: " + s.BenchmarkDate
		}
		out = append(out, entry)
	}
	return strings.Join(out, "\n\n")
}

// securityChecks renders the "Security Checks" cell — the rule
// identifier (SV-…) when aggregating by ruleId, or the group
// identifier (V-…) when aggregating by groupId.
func securityChecks(f Finding) string {
	if f.RuleID != "" {
		return f.RuleID
	}
	return f.GroupID
}

// Write produces an xlsx workbook containing one row per finding in
// the format-specific layout. The first row is always the header
// row; data rows start at row 2.
//
// The returned bytes are a complete .xlsx blob and can be streamed
// directly to an HTTP response (Content-Type
// application/vnd.openxmlformats-officedocument.spreadsheetml.sheet).
func Write(findings []Finding, format Format, def Defaults) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	sheet := "POAM"
	// excelize creates a default "Sheet1"; rename it instead of
	// adding a second one so the workbook has a single visible tab.
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return nil, err
	}

	var headers []string
	switch format {
	case FormatMCCAST:
		headers = mccastHeaders
	default:
		headers = emassHeaders
	}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellValue(sheet, cell, h); err != nil {
			return nil, err
		}
	}

	for i, finding := range findings {
		row := i + 2
		cells := buildRow(finding, format, def)
		for col, val := range cells {
			cell, _ := excelize.CoordinatesToCellName(col+1, row)
			if err := f.SetCellValue(sheet, cell, val); err != nil {
				return nil, err
			}
		}
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// buildRow produces the per-row string slice for the given format.
// Cell positions are 0-indexed; the writer translates to A1 notation.
func buildRow(f Finding, format Format, def Defaults) []string {
	switch format {
	case FormatMCCAST:
		return buildMCCASTRow(f, def)
	default:
		return buildEMASSRow(f, def)
	}
}

func buildEMASSRow(f Finding, def Defaults) []string {
	return []string{
		emassDescription(f),
		joinControls(f.CCIs),
		def.Office,
		securityChecks(f),
		"",
		def.Date,
		"Resolve this finding. " + def.Date,
		"Resolve this finding. " + def.Date,
		joinStigInfo(f.Stigs),
		def.Status,
		joinCCIComments(f.CCIs),
		rawSeverity(f.Severity),
		"",
		"",
		titleSeverity(f.Severity),
		"",
		"",
		"",
		"",
		"",
		titleSeverity(f.Severity),
		"",
		titleSeverity(f.Severity),
		joinAssets(f.Assets),
	}
}

func buildMCCASTRow(f Finding, def Defaults) []string {
	title := f.Title
	if title == "" && len(f.Rules) > 0 {
		title = f.Rules[0].Title
	}
	disc := ""
	if len(f.Rules) > 0 {
		disc = f.Rules[0].VulnDiscussion
	}
	dateID := ""
	if len(f.Stigs) > 0 {
		dateID = f.Stigs[0].BenchmarkDate
	}
	// The MCCAST "Security Control Number" column reuses the upstream
	// convention of `DoD RMF-<pkg>-<apAcronym>-CNSSI 1253` per CCI.
	controlParts := make([]string, 0, len(f.CCIs))
	for _, c := range f.CCIs {
		ap := strings.ReplaceAll(c.APAcronym, ".", " ")
		controlParts = append(controlParts,
			"DoD RMF-"+def.MccastPackageID+"-"+ap+"-CNSSI 1253",
		)
	}
	return []string{
		def.MccastAuthName,
		title,
		dateID,
		"STIG Finding",
		def.Status,
		def.MccastPackageID,
		def.Date,
		"",
		"",
		securityChecks(f),
		strings.Join(controlParts, "\n"),
		titleSeverity(f.Severity),
		disc,
		"",
		joinCCIComments(f.CCIs),
		joinAssets(f.Assets),
		"", "", "", "", "", "", "", "", // modified-CVSS columns left blank
	}
}

// SuggestedFilename returns a Content-Disposition friendly filename
// for a POA&M export. Mirrors the upstream
// "POAM-<format>-<collection>_<YYYYMMDD-HHMMSS>.xlsx" convention.
func SuggestedFilename(collectionName string, format Format) string {
	safe := sanitise(collectionName)
	if safe == "" {
		safe = "collection"
	}
	stamp := time.Now().UTC().Format("20060102-150405")
	return "POAM-" + string(format) + "-" + safe + "_" + stamp + ".xlsx"
}

// sanitise lowercases anything that isn't safe in a filename to '_'.
func sanitise(s string) string {
	b := make([]byte, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '.', r == '-', r == '_':
			b = append(b, byte(r))
		default:
			b = append(b, '_')
		}
	}
	return string(b)
}
