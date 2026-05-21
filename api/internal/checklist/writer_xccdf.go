package checklist

import (
	"encoding/xml"
	"fmt"
	"io"
)

// WriteXCCDFResults writes data as an XCCDF Benchmark document with a
// <TestResult> block. The output is round-trip compatible with
// ParseXCCDFResults for the rule-id + result pairs.
//
// One file represents one (asset, benchmark) pair; if data carries
// multiple STIGs only the first is emitted.
func WriteXCCDFResults(w io.Writer, d ExportData) error {
	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}
	doc := buildXCCDFDoc(d)
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return fmt.Errorf("encode xccdf: %w", err)
	}
	return enc.Flush()
}

type xccdfOutBenchmark struct {
	XMLName    xml.Name       `xml:"Benchmark"`
	Xmlns      string         `xml:"xmlns,attr"`
	ID         string         `xml:"id,attr"`
	Title      string         `xml:"title,omitempty"`
	Description string        `xml:"description,omitempty"`
	Version    string         `xml:"version,omitempty"`
	Rules      []xccdfOutRule `xml:"Rule,omitempty"`
	TestResult xccdfOutTestResult
}

type xccdfOutRule struct {
	XMLName     xml.Name `xml:"Rule"`
	ID          string   `xml:"id,attr"`
	Severity    string   `xml:"severity,attr,omitempty"`
	Weight      string   `xml:"weight,attr,omitempty"`
	Version     string   `xml:"version,omitempty"`
	Title       string   `xml:"title,omitempty"`
	Description string   `xml:"description,omitempty"`
}

type xccdfOutTestResult struct {
	XMLName       xml.Name           `xml:"TestResult"`
	ID            string             `xml:"id,attr"`
	StartTime     string             `xml:"start-time,attr,omitempty"`
	EndTime       string             `xml:"end-time,attr,omitempty"`
	Target        string             `xml:"target,omitempty"`
	TargetAddress []string           `xml:"target-address,omitempty"`
	TargetFacts   *xccdfOutFacts     `xml:"target-facts,omitempty"`
	RuleResults   []xccdfOutRR
}

type xccdfOutFacts struct {
	Facts []xccdfOutFact `xml:"fact"`
}

type xccdfOutFact struct {
	XMLName xml.Name `xml:"fact"`
	Name    string   `xml:"name,attr"`
	Type    string   `xml:"type,attr,omitempty"`
	Value   string   `xml:",chardata"`
}

type xccdfOutRR struct {
	XMLName   xml.Name `xml:"rule-result"`
	IDRef     string   `xml:"idref,attr"`
	Severity  string   `xml:"severity,attr,omitempty"`
	Time      string   `xml:"time,attr,omitempty"`
	Result    string   `xml:"result"`
}

func buildXCCDFDoc(d ExportData) xccdfOutBenchmark {
	var stig ExportStig
	if len(d.Stigs) > 0 {
		stig = d.Stigs[0]
	}
	doc := xccdfOutBenchmark{
		Xmlns:       "http://checklists.nist.gov/xccdf/1.2",
		ID:          "xccdf_mil.disa.stig_benchmark_" + stig.BenchmarkID,
		Title:       stig.Title,
		Description: stig.Description,
		Version:     stig.RevisionStr,
		TestResult: xccdfOutTestResult{
			ID:            "xccdf_mil.disa.stig_testresult_" + stig.BenchmarkID,
			Target:        d.Asset.HostName,
			TargetAddress: nonEmpty(d.Asset.IP),
		},
	}
	facts := &xccdfOutFacts{}
	if d.Asset.FQDN != "" {
		facts.Facts = append(facts.Facts, xccdfOutFact{
			Name:  "urn:xccdf:fact:asset:identifier:fqdn",
			Type:  "string",
			Value: d.Asset.FQDN,
		})
	}
	if d.Asset.MAC != "" {
		facts.Facts = append(facts.Facts, xccdfOutFact{
			Name:  "urn:xccdf:fact:asset:identifier:mac",
			Type:  "string",
			Value: d.Asset.MAC,
		})
	}
	if len(facts.Facts) > 0 {
		doc.TestResult.TargetFacts = facts
	}

	for _, r := range stig.Rules {
		doc.Rules = append(doc.Rules, xccdfOutRule{
			ID:          r.RuleID,
			Severity:    r.Severity,
			Weight:      r.Weight,
			Version:     r.VersionStr,
			Title:       r.Title,
			Description: r.Description,
		})
		doc.TestResult.RuleResults = append(doc.TestResult.RuleResults, xccdfOutRR{
			IDRef:    r.RuleID,
			Severity: r.Severity,
			Time:     r.TS,
			Result:   resultToXCCDFResult(r.Result),
		})
	}
	return doc
}

func nonEmpty(s string) []string {
	if s == "" {
		return nil
	}
	return []string{s}
}
