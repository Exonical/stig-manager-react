package checklist

import (
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

// ParseCKL decodes a STIG Viewer XML checklist into the normalised
// ParsedFile form.
func ParseCKL(src io.Reader) (*ParsedFile, error) {
	var doc cklDoc
	dec := xml.NewDecoder(src)
	// The DISA-published checklists frequently include a non-standard
	// encoding declaration ("UTF-8" with a BOM, or Windows-1252). The
	// stdlib decoder rejects any encoding it doesn't natively know; we
	// stay lenient by accepting whatever the producer claimed and
	// trusting the bytes we received.
	dec.CharsetReader = passthroughCharset
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}

	out := &ParsedFile{
		Format: "ckl",
		Asset:  cklAsset(doc.Asset),
	}

	for _, st := range doc.Stigs.IStig {
		bench, rev := cklBenchmark(st.Info.SIData)
		if out.BenchmarkID == "" {
			out.BenchmarkID = bench
			out.Revision = rev
		}
		for _, v := range st.Vulns {
			pr, ok := cklVuln(v)
			if ok {
				out.Reviews = append(out.Reviews, pr)
			}
		}
	}
	return out, nil
}

// cklDoc mirrors the subset of STIG Viewer's <CHECKLIST> schema we
// need: asset identity + per-rule status/notes.
type cklDoc struct {
	XMLName xml.Name `xml:"CHECKLIST"`
	Asset   cklAssetT `xml:"ASSET"`
	Stigs   struct {
		IStig []cklIStig `xml:"iSTIG"`
	} `xml:"STIGS"`
}

type cklAssetT struct {
	Role          string `xml:"ROLE"`
	AssetType     string `xml:"ASSET_TYPE"`
	HostName      string `xml:"HOST_NAME"`
	HostIP        string `xml:"HOST_IP"`
	HostMAC       string `xml:"HOST_MAC"`
	HostFQDN      string `xml:"HOST_FQDN"`
	TargetComment string `xml:"TARGET_COMMENT"`
}

type cklIStig struct {
	Info struct {
		SIData []cklSIData `xml:"SI_DATA"`
	} `xml:"STIG_INFO"`
	Vulns []cklVulnT `xml:"VULN"`
}

type cklSIData struct {
	Name string `xml:"SID_NAME"`
	Data string `xml:"SID_DATA"`
}

type cklVulnT struct {
	StigData       []cklStigData `xml:"STIG_DATA"`
	Status         string        `xml:"STATUS"`
	FindingDetails string        `xml:"FINDING_DETAILS"`
	Comments       string        `xml:"COMMENTS"`
}

type cklStigData struct {
	Attribute string `xml:"VULN_ATTRIBUTE"`
	Data      string `xml:"ATTRIBUTE_DATA"`
}

func cklAsset(a cklAssetT) AssetIdentity {
	return AssetIdentity{
		HostName: strings.TrimSpace(a.HostName),
		FQDN:     strings.TrimSpace(a.HostFQDN),
		IP:       strings.TrimSpace(a.HostIP),
		MAC:      strings.TrimSpace(a.HostMAC),
		Role:     strings.TrimSpace(a.Role),
	}
}

// cklBenchmark pulls (stigid, version+release) from the SI_DATA name
// dictionary that introduces every iSTIG block.
func cklBenchmark(sis []cklSIData) (string, string) {
	var stigID, version, release string
	for _, si := range sis {
		switch strings.ToLower(si.Name) {
		case "stigid":
			stigID = strings.TrimSpace(si.Data)
		case "version":
			version = strings.TrimSpace(si.Data)
		case "releaseinfo":
			release = strings.TrimSpace(si.Data)
		}
	}
	if version != "" {
		return stigID, "V" + version + "R" + release
	}
	return stigID, release
}

// cklVuln converts a single <VULN> block. Returns ok=false when the
// rule has no Rule_ID (legacy entries that only carry Vuln_Num); those
// rows are silently dropped because we can't tie them to a review.
func cklVuln(v cklVulnT) (ParsedReview, bool) {
	var ruleID, severity string
	for _, sd := range v.StigData {
		switch strings.ToLower(sd.Attribute) {
		case "rule_id":
			ruleID = strings.TrimSpace(sd.Data)
		case "severity":
			severity = strings.TrimSpace(sd.Data)
		}
	}
	if ruleID == "" {
		return ParsedReview{}, false
	}
	return ParsedReview{
		RuleID:   ruleID,
		Result:   cklStatusToResult(v.Status),
		Detail:   v.FindingDetails,
		Comment:  v.Comments,
		Severity: severity,
	}, true
}

// cklStatusToResult mirrors upstream's CKL → review.result mapping.
func cklStatusToResult(s string) Result {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "notafinding":
		return ResultPass
	case "open":
		return ResultFail
	case "not_applicable":
		return ResultNotApplicable
	case "not_reviewed", "":
		return ResultNotChecked
	default:
		return ResultUnknown
	}
}

// passthroughCharset is an xml.CharsetReader that accepts whatever
// encoding the document claims and just hands the bytes back. DISA
// checklists are reliably UTF-8 in practice even when the prologue
// names something else.
func passthroughCharset(_ string, in io.Reader) (io.Reader, error) {
	if in == nil {
		return nil, errors.New("nil reader")
	}
	return in, nil
}
