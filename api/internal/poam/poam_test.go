package poam

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func sampleFindings() []Finding {
	return []Finding{
		{
			GroupID:  "V-220706",
			Title:    "The audit system must …",
			Severity: "high",
			Rules: []Rule{{
				RuleID:         "SV-220706r926943",
				Title:          "The audit system must capture every privilege escalation",
				Severity:       "high",
				VulnDiscussion: "Without timely audit, …",
			}},
			Assets: []Asset{
				{AssetID: "1", Name: "host-a"},
				{AssetID: "2", Name: "host-b"},
			},
			Stigs: []Stig{
				{BenchmarkID: "RHEL_9_STIG", RevisionStr: "V1R2", BenchmarkDate: "2024-08-15"},
			},
			CCIs: []CCI{
				{CCI: "CCI-000172"},
				{CCI: "CCI-000130"},
			},
		},
		{
			GroupID:  "V-220888",
			Title:    "Filesystem encryption must be enabled",
			Severity: "medium",
			Rules: []Rule{{
				RuleID:         "SV-220888r926941",
				Title:          "Filesystem encryption must be enabled",
				Severity:       "medium",
				VulnDiscussion: "Disk encryption protects data at rest.",
			}},
			Assets: []Asset{{AssetID: "1", Name: "host-a"}},
			Stigs: []Stig{
				{BenchmarkID: "RHEL_9_STIG", RevisionStr: "V1R2", BenchmarkDate: "2024-08-15"},
			},
		},
	}
}

func openWorkbook(t *testing.T, blob []byte) *excelize.File {
	t.Helper()
	xf, err := excelize.OpenReader(bytes.NewReader(blob))
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	t.Cleanup(func() { _ = xf.Close() })
	return xf
}

func cell(t *testing.T, xf *excelize.File, col, row int) string {
	t.Helper()
	name, err := excelize.CoordinatesToCellName(col, row)
	if err != nil {
		t.Fatalf("CoordinatesToCellName(%d,%d): %v", col, row, err)
	}
	v, err := xf.GetCellValue("POAM", name)
	if err != nil {
		t.Fatalf("GetCellValue(%s): %v", name, err)
	}
	return v
}

func TestWriteEMASS_Headers(t *testing.T) {
	blob, err := Write(nil, FormatEMASS, Defaults{})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	xf := openWorkbook(t, blob)

	if got := cell(t, xf, 1, 1); got != emassHeaders[0] {
		t.Errorf("header[0] = %q, want %q", got, emassHeaders[0])
	}
	last := len(emassHeaders)
	if got := cell(t, xf, last, 1); got != "Assets Affected" {
		t.Errorf("header[last] = %q, want 'Assets Affected'", got)
	}
}

func TestWriteEMASS_RowContent(t *testing.T) {
	def := Defaults{Date: "2026-09-30", Office: "S2", Status: "Ongoing"}
	blob, err := Write(sampleFindings(), FormatEMASS, def)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	xf := openWorkbook(t, blob)

	// Row 2 column 1 — Control Vulnerability Description
	desc := cell(t, xf, 1, 2)
	if !strings.Contains(desc, "Title:\nThe audit system must …") {
		t.Errorf("description missing title; got %q", desc)
	}
	if !strings.Contains(desc, "Description:\nWithout timely audit") {
		t.Errorf("description missing vuln discussion; got %q", desc)
	}

	// Row 2 column 3 — Office/Org
	if got := cell(t, xf, 3, 2); got != "S2" {
		t.Errorf("office = %q, want 'S2'", got)
	}
	// Row 2 column 4 — Security Checks (groupId)
	if got := cell(t, xf, 4, 2); got != "V-220706" {
		t.Errorf("security checks = %q, want 'V-220706'", got)
	}
	// Row 2 column 6 — Scheduled Completion Date
	if got := cell(t, xf, 6, 2); got != "2026-09-30" {
		t.Errorf("scheduled date = %q, want 2026-09-30", got)
	}
	// Row 2 column 10 — Status
	if got := cell(t, xf, 10, 2); got != "Ongoing" {
		t.Errorf("status = %q, want 'Ongoing'", got)
	}
	// Row 2 column 11 — Comments (CCIs newline-joined)
	if got := cell(t, xf, 11, 2); got != "CCI-000172\nCCI-000130" {
		t.Errorf("comments = %q, want 'CCI-000172\\nCCI-000130'", got)
	}
	// Row 2 column 12 — Raw Severity Value
	if got := cell(t, xf, 12, 2); got != "I" {
		t.Errorf("raw severity = %q, want 'I'", got)
	}
	// Row 2 column 15 — Severity Value (title-cased)
	if got := cell(t, xf, 15, 2); got != "High" {
		t.Errorf("severity value = %q, want 'High'", got)
	}
	// Row 2 last column — Assets Affected (newline-joined names)
	assets := cell(t, xf, len(emassHeaders), 2)
	if assets != "host-a\nhost-b" {
		t.Errorf("assets = %q, want 'host-a\\nhost-b'", assets)
	}

	// Row 3 column 12 — medium severity → "II"
	if got := cell(t, xf, 12, 3); got != "II" {
		t.Errorf("row3 raw severity = %q, want 'II'", got)
	}
	if got := cell(t, xf, 15, 3); got != "Moderate" {
		t.Errorf("row3 severity value = %q, want 'Moderate'", got)
	}
}

func TestWriteMCCAST_Headers(t *testing.T) {
	blob, err := Write(nil, FormatMCCAST, Defaults{})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	xf := openWorkbook(t, blob)
	if got := cell(t, xf, 1, 1); got != "Authorization Package" {
		t.Errorf("mccast header[0] = %q", got)
	}
	if got := cell(t, xf, 10, 1); got != "Security Checks" {
		t.Errorf("mccast header[9] = %q", got)
	}
}

func TestWriteMCCAST_RowContent(t *testing.T) {
	def := Defaults{
		Date:            "2026-09-30",
		Status:          "Ongoing",
		MccastPackageID: "PKG-001",
		MccastAuthName:  "Authorisation X",
	}
	blob, err := Write(sampleFindings(), FormatMCCAST, def)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	xf := openWorkbook(t, blob)

	if got := cell(t, xf, 1, 2); got != "Authorisation X" {
		t.Errorf("auth name = %q", got)
	}
	if got := cell(t, xf, 2, 2); got != "The audit system must …" {
		t.Errorf("vuln name = %q", got)
	}
	if got := cell(t, xf, 6, 2); got != "PKG-001" {
		t.Errorf("package id = %q", got)
	}
	if got := cell(t, xf, 7, 2); got != "2026-09-30" {
		t.Errorf("date identified = %q", got)
	}
	if got := cell(t, xf, 10, 2); got != "V-220706" {
		t.Errorf("security checks = %q", got)
	}
}

func TestSuggestedFilename(t *testing.T) {
	got := SuggestedFilename("My Collection / 1", FormatEMASS)
	if !strings.HasPrefix(got, "POAM-EMASS-My_Collection___1_") {
		t.Errorf("filename prefix = %q", got)
	}
	if !strings.HasSuffix(got, ".xlsx") {
		t.Errorf("filename suffix = %q", got)
	}
}
