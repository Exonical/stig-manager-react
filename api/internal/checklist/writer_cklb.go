package checklist

import (
	"encoding/json"
	"fmt"
	"io"
)

// WriteCKLB writes data as a STIG Viewer 3 JSON checklist (.cklb).
// The output is round-trip compatible with ParseCKLB: a checklist
// written with WriteCKLB and re-parsed reproduces every (rule, result,
// detail, comment) tuple in the input.
func WriteCKLB(w io.Writer, d ExportData) error {
	doc := buildCKLBDoc(d)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return fmt.Errorf("encode cklb: %w", err)
	}
	return nil
}

type cklbOut struct {
	Title      string          `json:"title"`
	StigID     string          `json:"stig_id,omitempty"`
	Active     bool            `json:"active"`
	Mode       int             `json:"mode"`
	HasPath    bool            `json:"has_path"`
	TargetData cklbOutTarget   `json:"target_data"`
	Stigs      []cklbOutStig   `json:"stigs"`
}

type cklbOutTarget struct {
	TargetType     string `json:"target_type"`
	HostName       string `json:"host_name"`
	IP             string `json:"ip_address"`
	MAC            string `json:"mac_address"`
	FQDN           string `json:"fqdn"`
	Comments       string `json:"comments"`
	Role           string `json:"role"`
	IsVirtual      bool   `json:"is_virtual"`
	TechnologyArea string `json:"technology_area"`
	WebOrDB        bool   `json:"web_or_database"`
	WebDBSite      string `json:"web_db_site"`
	WebDBInstance  string `json:"web_db_instance"`
}

type cklbOutStig struct {
	StigName    string         `json:"stig_name"`
	DisplayName string         `json:"display_name"`
	StigID      string         `json:"stig_id"`
	Version     string         `json:"version"`
	Release     string         `json:"release"`
	ReleaseInfo string         `json:"release_info,omitempty"`
	Description string         `json:"description,omitempty"`
	Source      string         `json:"source,omitempty"`
	Rules       []cklbOutRule  `json:"rules"`
}

type cklbOutRule struct {
	GroupID        string   `json:"group_id"`
	GroupTitle     string   `json:"group_title"`
	RuleID         string   `json:"rule_id"`
	RuleIDSrc      string   `json:"rule_id_src,omitempty"`
	RuleVersion    string   `json:"rule_version"`
	Severity       string   `json:"severity"`
	Weight         string   `json:"weight,omitempty"`
	Title          string   `json:"rule_title"`
	Discussion     string   `json:"discussion,omitempty"`
	CheckContent   string   `json:"check_content,omitempty"`
	FixText        string   `json:"fix_text,omitempty"`
	CCIs           []string `json:"ccis"`
	Status         string   `json:"status"`
	FindingDetails string   `json:"finding_details"`
	Comments       string   `json:"comments"`
	AutoResult     bool     `json:"auto_result"`
}

func buildCKLBDoc(d ExportData) cklbOut {
	role := d.Asset.Role
	if role == "" {
		role = "None"
	}
	tt := d.Asset.AssetType
	if tt == "" {
		tt = "Computing"
	}
	out := cklbOut{
		Title:  "STIG Manager Checklist",
		Active: true,
		Mode:   1,
		TargetData: cklbOutTarget{
			TargetType:     tt,
			HostName:       d.Asset.HostName,
			IP:             d.Asset.IP,
			MAC:            d.Asset.MAC,
			FQDN:           d.Asset.FQDN,
			Role:           role,
			TechnologyArea: d.Asset.TechArea,
			WebOrDB:        d.Asset.WebOrDB,
			WebDBSite:      d.Asset.WebDBSite,
			WebDBInstance:  d.Asset.WebDBInst,
		},
	}
	if len(d.Stigs) == 1 {
		out.StigID = d.Stigs[0].BenchmarkID
	}
	for _, st := range d.Stigs {
		os := cklbOutStig{
			StigName:    st.BenchmarkID,
			DisplayName: st.Title,
			StigID:      st.BenchmarkID,
			Version:     st.Version,
			Release:     st.Release,
			ReleaseInfo: st.RevisionStr,
			Description: st.Description,
			Source:      st.Source,
		}
		for _, r := range st.Rules {
			ccis := r.CCIs
			if ccis == nil {
				ccis = []string{}
			}
			os.Rules = append(os.Rules, cklbOutRule{
				GroupID:        r.GroupID,
				GroupTitle:     r.GroupTitle,
				RuleID:         r.RuleID,
				RuleVersion:    r.VersionStr,
				Severity:       r.Severity,
				Weight:         r.Weight,
				Title:          r.Title,
				Discussion:     r.Description,
				CheckContent:   r.CheckContent,
				FixText:        r.FixText,
				CCIs:           ccis,
				Status:         resultToCKLBStatus(r.Result),
				FindingDetails: r.Detail,
				Comments:       r.Comment,
				AutoResult:     r.AutoResult,
			})
		}
		out.Stigs = append(out.Stigs, os)
	}
	return out
}
