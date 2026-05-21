package checklist

// ExportData is the input bundle for the format writers (WriteCKL /
// WriteCKLB / WriteXCCDFResults). It mirrors the store projection but
// is decoupled from the database layer so the checklist package stays
// dependency-free.
type ExportData struct {
	Asset ExportAsset
	Stigs []ExportStig
}

// ExportAsset is the per-asset header captured at the top of each
// checklist file.
type ExportAsset struct {
	HostName     string
	FQDN         string
	IP           string
	MAC          string
	Role         string // None | Workstation | Member Server | Domain Controller
	AssetType    string // Computing | Non-Computing
	TargetKey    string // arbitrary stable identifier; we use asset_id
	TechArea     string
	WebOrDB      bool
	WebDBSite    string
	WebDBInst    string
}

// ExportStig is the per-benchmark slice of an export. Rules SHOULD be
// pre-sorted in the order they appear in the source revision.
type ExportStig struct {
	BenchmarkID string
	RevisionStr string
	Version     string
	Release     string
	Title       string
	Description string
	Source      string
	ReleaseDate string // YYYY-MM-DD, empty when unknown
	Rules       []ExportRule
}

// ExportRule is one rule's content + the current review state. When
// Result is empty the rule is rendered as "Not_Reviewed" in CKL,
// "not_reviewed" in CKLB, and as an unevaluated rule-result in XCCDF.
type ExportRule struct {
	RuleID       string
	VersionStr   string // STIG "Vuln_Num" / version (e.g. "TEST-OS-000010")
	GroupID      string
	GroupTitle   string
	Severity     string
	Weight       string
	Title        string
	Description  string
	CheckSystem  string
	CheckContent string
	FixID        string
	FixText      string
	CCIs         []string

	// Review fields. Result == "" means the rule has no review yet.
	Result      string
	Detail      string
	Comment     string
	AutoResult  bool
	StatusLabel string
	TS          string // RFC3339, empty when no review
	TouchTS     string // RFC3339, empty when no review
	Username    string
}

// resultToCKLStatus maps review.result to CKL <STATUS> values.
func resultToCKLStatus(result string) string {
	switch result {
	case string(ResultPass):
		return "NotAFinding"
	case string(ResultFail):
		return "Open"
	case string(ResultNotApplicable):
		return "Not_Applicable"
	default:
		return "Not_Reviewed"
	}
}

// resultToCKLBStatus maps review.result to CKLB rule.status values.
func resultToCKLBStatus(result string) string {
	switch result {
	case string(ResultPass):
		return "not_a_finding"
	case string(ResultFail):
		return "open"
	case string(ResultNotApplicable):
		return "not_applicable"
	case string(ResultInformational):
		return "informational"
	default:
		return "not_reviewed"
	}
}

// resultToXCCDFResult maps review.result to XCCDF <result> values.
func resultToXCCDFResult(result string) string {
	switch result {
	case string(ResultPass):
		return "pass"
	case string(ResultFail):
		return "fail"
	case string(ResultNotApplicable):
		return "notapplicable"
	case string(ResultInformational):
		return "informational"
	case string(ResultError):
		return "error"
	case string(ResultUnknown):
		return "unknown"
	case string(ResultFixed):
		return "fixed"
	default:
		return "notchecked"
	}
}
