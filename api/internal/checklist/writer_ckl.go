package checklist

import (
	"encoding/xml"
	"fmt"
	"io"
)

// WriteCKL writes data as a DISA STIG Viewer XML checklist (.ckl). The
// output is round-trip compatible with ParseCKL: a checklist written
// with WriteCKL and re-parsed reproduces every (rule, result, detail,
// comment) tuple in the input.
//
// Asset/role defaults (Role=None, AssetType=Computing) follow upstream's
// behaviour when the caller leaves the fields empty.
func WriteCKL(w io.Writer, d ExportData) error {
	doc := buildCKLDoc(d)
	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return fmt.Errorf("encode ckl: %w", err)
	}
	return enc.Flush()
}

type cklOut struct {
	XMLName xml.Name `xml:"CHECKLIST"`
	Asset   cklOutAsset
	Stigs   cklOutStigs
}

type cklOutAsset struct {
	XMLName       xml.Name `xml:"ASSET"`
	Role          string   `xml:"ROLE"`
	AssetType     string   `xml:"ASSET_TYPE"`
	HostName      string   `xml:"HOST_NAME"`
	HostIP        string   `xml:"HOST_IP"`
	HostMAC       string   `xml:"HOST_MAC"`
	HostFQDN      string   `xml:"HOST_FQDN"`
	TargetComment string   `xml:"TARGET_COMMENT"`
	TechArea      string   `xml:"TECH_AREA"`
	TargetKey     string   `xml:"TARGET_KEY"`
	WebOrDB       string   `xml:"WEB_OR_DATABASE"`
	WebDBSite     string   `xml:"WEB_DB_SITE"`
	WebDBInst     string   `xml:"WEB_DB_INSTANCE"`
}

type cklOutStigs struct {
	XMLName xml.Name     `xml:"STIGS"`
	IStig   []cklOutIStig `xml:"iSTIG"`
}

type cklOutIStig struct {
	Info cklOutSTIGInfo
	Vuln []cklOutVuln
}

type cklOutSTIGInfo struct {
	XMLName xml.Name      `xml:"STIG_INFO"`
	SIData  []cklOutSIData
}

type cklOutSIData struct {
	XMLName xml.Name `xml:"SI_DATA"`
	Name    string   `xml:"SID_NAME"`
	Data    string   `xml:"SID_DATA,omitempty"`
}

type cklOutVuln struct {
	XMLName               xml.Name        `xml:"VULN"`
	StigData              []cklOutStigData
	Status                string          `xml:"STATUS"`
	FindingDetails        string          `xml:"FINDING_DETAILS"`
	Comments              string          `xml:"COMMENTS"`
	SeverityOverride      string          `xml:"SEVERITY_OVERRIDE"`
	SeverityJustification string          `xml:"SEVERITY_JUSTIFICATION"`
}

type cklOutStigData struct {
	XMLName   xml.Name `xml:"STIG_DATA"`
	Attribute string   `xml:"VULN_ATTRIBUTE"`
	Data      string   `xml:"ATTRIBUTE_DATA"`
}

func buildCKLDoc(d ExportData) cklOut {
	role := d.Asset.Role
	if role == "" {
		role = "None"
	}
	at := d.Asset.AssetType
	if at == "" {
		at = "Computing"
	}
	out := cklOut{
		Asset: cklOutAsset{
			Role:      role,
			AssetType: at,
			HostName:  d.Asset.HostName,
			HostIP:    d.Asset.IP,
			HostMAC:   d.Asset.MAC,
			HostFQDN:  d.Asset.FQDN,
			TechArea:  d.Asset.TechArea,
			TargetKey: d.Asset.TargetKey,
			WebOrDB:   boolStr(d.Asset.WebOrDB),
			WebDBSite: d.Asset.WebDBSite,
			WebDBInst: d.Asset.WebDBInst,
		},
	}
	for _, st := range d.Stigs {
		ist := cklOutIStig{
			Info: cklOutSTIGInfo{
				SIData: []cklOutSIData{
					{Name: "version", Data: st.Version},
					{Name: "stigid", Data: st.BenchmarkID},
					{Name: "title", Data: st.Title},
					{Name: "description", Data: st.Description},
					// Upstream stores the release-info banner here
					// ("Release: 2 Benchmark Date: ..."). For
					// round-trip compatibility with ParseCKL we emit
					// just the bare release number; ParseCKL then
					// rebuilds RevisionStr as "V<version>R<release>".
					{Name: "releaseinfo", Data: st.Release},
					{Name: "source", Data: st.Source},
				},
			},
		}
		for _, r := range st.Rules {
			v := cklOutVuln{
				Status:         resultToCKLStatus(r.Result),
				FindingDetails: r.Detail,
				Comments:       r.Comment,
			}
			v.StigData = append(v.StigData,
				cklOutStigData{Attribute: "Vuln_Num", Data: r.GroupID},
				cklOutStigData{Attribute: "Severity", Data: r.Severity},
				cklOutStigData{Attribute: "Group_Title", Data: r.GroupTitle},
				cklOutStigData{Attribute: "Rule_ID", Data: r.RuleID},
				cklOutStigData{Attribute: "Rule_Ver", Data: r.VersionStr},
				cklOutStigData{Attribute: "Rule_Title", Data: r.Title},
				cklOutStigData{Attribute: "Vuln_Discuss", Data: r.Description},
				cklOutStigData{Attribute: "Check_Content", Data: r.CheckContent},
				cklOutStigData{Attribute: "Fix_Text", Data: r.FixText},
				cklOutStigData{Attribute: "STIGRef", Data: stigRef(st)},
				cklOutStigData{Attribute: "TargetKey", Data: d.Asset.TargetKey},
				cklOutStigData{Attribute: "STIG_UUID", Data: ""},
			)
			for _, cci := range r.CCIs {
				v.StigData = append(v.StigData, cklOutStigData{Attribute: "CCI_REF", Data: cci})
			}
			ist.Vuln = append(ist.Vuln, v)
		}
		out.Stigs.IStig = append(out.Stigs.IStig, ist)
	}
	return out
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func stigRef(s ExportStig) string {
	if s.Title == "" {
		return s.BenchmarkID + " :: " + s.RevisionStr
	}
	return s.Title + " :: " + s.RevisionStr
}
