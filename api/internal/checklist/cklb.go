package checklist

import (
	"encoding/json"
	"io"
	"strings"
)

// ParseCKLB decodes a STIG Viewer JSON (.cklb) checklist into a
// ParsedFile.
func ParseCKLB(src io.Reader) (*ParsedFile, error) {
	var doc cklbDoc
	if err := json.NewDecoder(src).Decode(&doc); err != nil {
		return nil, err
	}
	out := &ParsedFile{
		Format: "cklb",
		Asset: AssetIdentity{
			HostName: strings.TrimSpace(doc.TargetData.HostName),
			FQDN:     strings.TrimSpace(doc.TargetData.FQDN),
			IP:       strings.TrimSpace(doc.TargetData.IP),
			MAC:      strings.TrimSpace(doc.TargetData.MAC),
			Role:     strings.TrimSpace(doc.TargetData.Role),
		},
	}
	for _, stig := range doc.Stigs {
		if out.BenchmarkID == "" {
			out.BenchmarkID = strings.TrimSpace(stig.StigID)
			out.Revision = cklbRevision(stig.Version, stig.Release, stig.ReleaseInfo)
		}
		for _, r := range stig.Rules {
			ruleID := strings.TrimSpace(r.RuleID)
			if ruleID == "" {
				ruleID = strings.TrimSpace(r.RuleIDSrc)
			}
			if ruleID == "" {
				continue
			}
			out.Reviews = append(out.Reviews, ParsedReview{
				RuleID:   ruleID,
				Result:   cklbStatusToResult(r.Status),
				Detail:   r.FindingDetails,
				Comment:  r.Comments,
				Severity: strings.TrimSpace(r.Severity),
			})
		}
	}
	return out, nil
}

type cklbDoc struct {
	Title      string         `json:"title"`
	StigID     string         `json:"stig_id"`
	TargetData cklbTargetData `json:"target_data"`
	Stigs      []cklbStig     `json:"stigs"`
}

type cklbTargetData struct {
	HostName string `json:"host_name"`
	FQDN     string `json:"fqdn"`
	IP       string `json:"ip_address"`
	MAC      string `json:"mac_address"`
	Role     string `json:"role"`
}

type cklbStig struct {
	StigID      string     `json:"stig_id"`
	Version     string     `json:"version"`
	Release     string     `json:"release"`
	ReleaseInfo string     `json:"release_info"`
	Rules       []cklbRule `json:"rules"`
}

type cklbRule struct {
	GroupID        string `json:"group_id"`
	RuleID         string `json:"rule_id"`
	RuleIDSrc      string `json:"rule_id_src"`
	Severity       string `json:"severity"`
	Status         string `json:"status"`
	FindingDetails string `json:"finding_details"`
	Comments       string `json:"comments"`
}

func cklbRevision(version, release, releaseInfo string) string {
	v := strings.TrimSpace(version)
	r := strings.TrimSpace(release)
	if v == "" && r == "" {
		return strings.TrimSpace(releaseInfo)
	}
	if v == "" {
		v = "0"
	}
	if r == "" {
		r = "0"
	}
	return "V" + v + "R" + r
}

func cklbStatusToResult(s string) Result {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "not_a_finding":
		return ResultPass
	case "open":
		return ResultFail
	case "not_applicable":
		return ResultNotApplicable
	case "not_reviewed", "":
		return ResultNotChecked
	case "informational":
		return ResultInformational
	default:
		return ResultUnknown
	}
}
