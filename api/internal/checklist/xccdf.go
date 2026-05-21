package checklist

import (
	"encoding/xml"
	"io"
	"strings"
)

// ParseXCCDFResults decodes an XCCDF `<Benchmark>` document containing
// a `<TestResult>` block (as emitted by OpenSCAP, Evaluate-STIG, etc.)
// into a ParsedFile.
func ParseXCCDFResults(src io.Reader) (*ParsedFile, error) {
	var doc xccdfBenchmark
	dec := xml.NewDecoder(src)
	dec.CharsetReader = passthroughCharset
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	out := &ParsedFile{
		Format:      "xccdf",
		BenchmarkID: strings.TrimSpace(doc.ID),
		Revision:    strings.TrimSpace(doc.Version),
	}
	if doc.TestResult.Target != "" {
		out.Asset.HostName = strings.TrimSpace(doc.TestResult.Target)
	}
	if len(doc.TestResult.TargetAddress) > 0 {
		out.Asset.IP = strings.TrimSpace(doc.TestResult.TargetAddress[0])
	}
	for _, f := range doc.TestResult.TargetFacts.Facts {
		switch strings.ToLower(f.Name) {
		case "urn:xccdf:fact:asset:identifier:fqdn":
			out.Asset.FQDN = strings.TrimSpace(f.Value)
		case "urn:xccdf:fact:asset:identifier:mac":
			out.Asset.MAC = strings.TrimSpace(f.Value)
		}
	}
	for _, rr := range doc.TestResult.RuleResults {
		ruleID := normalizeXCCDFRuleID(rr.IDRef)
		if ruleID == "" {
			continue
		}
		out.Reviews = append(out.Reviews, ParsedReview{
			RuleID:     ruleID,
			Result:     xccdfResult(rr.Result),
			Severity:   strings.TrimSpace(rr.Severity),
			AutoResult: true,
		})
	}
	return out, nil
}

type xccdfBenchmark struct {
	XMLName    xml.Name       `xml:"Benchmark"`
	ID         string         `xml:"id,attr"`
	Version    string         `xml:"version"`
	TestResult xccdfTestResult `xml:"TestResult"`
}

type xccdfTestResult struct {
	Target        string         `xml:"target"`
	TargetAddress []string       `xml:"target-address"`
	TargetFacts   xccdfFacts     `xml:"target-facts"`
	RuleResults   []xccdfRR      `xml:"rule-result"`
}

type xccdfFacts struct {
	Facts []xccdfFact `xml:"fact"`
}

type xccdfFact struct {
	Name  string `xml:"name,attr"`
	Value string `xml:",chardata"`
}

type xccdfRR struct {
	IDRef    string `xml:"idref,attr"`
	Severity string `xml:"severity,attr"`
	Result   string `xml:"result"`
}

// normalizeXCCDFRuleID strips the OVAL/XCCDF id prefix
// (xccdf_mil.disa.stig_rule_SV-12345r1_rule → SV-12345r1_rule).
func normalizeXCCDFRuleID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	if i := strings.LastIndex(id, "_SV-"); i >= 0 {
		return id[i+1:]
	}
	// Already in bare SV form.
	if strings.HasPrefix(id, "SV-") {
		return id
	}
	return id
}

// xccdfResult maps the XCCDF rule-result enum to our normalised
// review.result vocabulary.
func xccdfResult(s string) Result {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "pass":
		return ResultPass
	case "fail":
		return ResultFail
	case "notapplicable", "not-applicable":
		return ResultNotApplicable
	case "notchecked", "not-checked":
		return ResultNotChecked
	case "informational":
		return ResultInformational
	case "fixed":
		return ResultFixed
	case "error":
		return ResultError
	case "unknown", "":
		return ResultUnknown
	default:
		return ResultUnknown
	}
}
