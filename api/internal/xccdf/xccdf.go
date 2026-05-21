// Package xccdf decodes DISA STIG XCCDF 1.2 Benchmark XML into the
// internal types used by the data layer. It is intentionally limited
// to the shape we see in published DoD STIGs:
//
//   - A top-level <Benchmark> element (XCCDF 1.2 namespace).
//   - Optional release metadata in a <plain-text id="release-info">.
//   - Groups containing Rules with check / fix children and `ident`
//     elements pointing at CCIs.
//
// Other XCCDF variants (full data-stream files, SCAP 1.2 sources,
// tailoring profiles) are out of scope here and arrive in later
// milestones.
package xccdf

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

// Benchmark is the parsed representation of an XCCDF Benchmark file.
type Benchmark struct {
	BenchmarkID   string
	Title         string
	Description   string
	Source        string
	Version       string
	Release       string
	ReleaseDate   time.Time
	Status        string
	StatusDate    time.Time
	BenchmarkDate time.Time
	Marking       string
	Rules         []Rule
}

// Rule is a single STIG rule.
type Rule struct {
	RuleID       string
	VersionStr   string
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
}

// RevisionStr returns the canonical "V<version>R<release>" form. When
// either component is missing, falls back to whatever is present.
func (b Benchmark) RevisionStr() string {
	v, r := strings.TrimSpace(b.Version), strings.TrimSpace(b.Release)
	switch {
	case v != "" && r != "":
		return "V" + v + "R" + r
	case v != "":
		return "V" + v + "R0"
	case r != "":
		return "V0R" + r
	default:
		return "V0R0"
	}
}

// Parse decodes a Benchmark from an XCCDF XML stream. The reader is
// consumed in full.
func Parse(r io.Reader) (*Benchmark, error) {
	var raw rawBenchmark
	dec := xml.NewDecoder(r)
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("xccdf: decode benchmark: %w", err)
	}
	if raw.XMLName.Local != "Benchmark" {
		return nil, fmt.Errorf("xccdf: expected <Benchmark>, got <%s>", raw.XMLName.Local)
	}
	return raw.normalise()
}

// rawBenchmark mirrors the on-the-wire XML so encoding/xml can decode
// without us having to walk a stream of tokens.
type rawBenchmark struct {
	XMLName     xml.Name      `xml:"Benchmark"`
	ID          string        `xml:"id,attr"`
	Title       string        `xml:"title"`
	Description string        `xml:"description"`
	Version     string        `xml:"version"`
	Status      rawStatus     `xml:"status"`
	Notice      string        `xml:"notice"`
	PlainText   []rawPlain    `xml:"plain-text"`
	References  []rawRef      `xml:"reference"`
	Groups      []rawGroup    `xml:"Group"`
	Profiles    []rawProfile  `xml:"Profile"`
	Models      []rawAttrOnly `xml:"model"`
}

type rawPlain struct {
	ID   string `xml:"id,attr"`
	Text string `xml:",chardata"`
}

type rawRef struct {
	Href   string `xml:"href,attr"`
	Source string `xml:"source"`
}

type rawStatus struct {
	Date string `xml:"date,attr"`
	Text string `xml:",chardata"`
}

type rawProfile struct {
	ID string `xml:"id,attr"`
}

type rawAttrOnly struct {
	System string `xml:"system,attr"`
}

type rawGroup struct {
	ID    string    `xml:"id,attr"`
	Title string    `xml:"title"`
	Rules []rawRule `xml:"Rule"`
}

type rawRule struct {
	ID          string     `xml:"id,attr"`
	Severity    string     `xml:"severity,attr"`
	Weight      string     `xml:"weight,attr"`
	Version     string     `xml:"version"`
	Title       string     `xml:"title"`
	Description string     `xml:"description"`
	Idents      []rawIdent `xml:"ident"`
	FixText     rawFixText `xml:"fixtext"`
	FixRef      rawFix     `xml:"fix"`
	Check       rawCheck   `xml:"check"`
}

type rawIdent struct {
	System string `xml:"system,attr"`
	Text   string `xml:",chardata"`
}

type rawFixText struct {
	FixRef string `xml:"fixref,attr"`
	Text   string `xml:",chardata"`
}

type rawFix struct {
	ID string `xml:"id,attr"`
}

type rawCheck struct {
	System          string             `xml:"system,attr"`
	CheckContentRef rawCheckContentRef `xml:"check-content-ref"`
	CheckContent    string             `xml:"check-content"`
}

type rawCheckContentRef struct {
	Href string `xml:"href,attr"`
	Name string `xml:"name,attr"`
}

var (
	reReleaseInfo  = regexp.MustCompile(`(?i)release:\s*(\S+)`)
	reBenchmarkDt  = regexp.MustCompile(`(?i)benchmark\s*date:\s*([^\n]+?)\s*$`)
	cciPrefixMatch = regexp.MustCompile(`(?i)^CCI-?\d+$`)
)

func (r rawBenchmark) normalise() (*Benchmark, error) {
	if strings.TrimSpace(r.ID) == "" {
		return nil, errors.New("xccdf: Benchmark missing id attribute")
	}
	b := &Benchmark{
		BenchmarkID: canonicalBenchmarkID(r.ID),
		Title:       strings.TrimSpace(r.Title),
		Description: strings.TrimSpace(r.Description),
		Version:     strings.TrimSpace(r.Version),
		Status:      strings.TrimSpace(r.Status.Text),
		StatusDate:  parseDate(r.Status.Date),
	}
	if len(r.References) > 0 {
		b.Source = strings.TrimSpace(r.References[0].Href)
	}
	for _, p := range r.PlainText {
		switch p.ID {
		case "release-info":
			text := strings.TrimSpace(p.Text)
			if m := reReleaseInfo.FindStringSubmatch(text); len(m) == 2 {
				b.Release = strings.TrimSpace(m[1])
			}
			if m := reBenchmarkDt.FindStringSubmatch(text); len(m) == 2 {
				b.BenchmarkDate = parseDate(strings.TrimSpace(m[1]))
			}
		case "marking":
			b.Marking = strings.TrimSpace(p.Text)
		}
	}
	if !b.BenchmarkDate.IsZero() {
		b.ReleaseDate = b.BenchmarkDate
	}

	for _, g := range r.Groups {
		for _, raw := range g.Rules {
			rule := Rule{
				RuleID:       strings.TrimSpace(raw.ID),
				VersionStr:   strings.TrimSpace(raw.Version),
				GroupID:      strings.TrimSpace(g.ID),
				GroupTitle:   strings.TrimSpace(g.Title),
				Severity:     normaliseSeverity(raw.Severity),
				Weight:       strings.TrimSpace(raw.Weight),
				Title:        strings.TrimSpace(raw.Title),
				Description:  strings.TrimSpace(raw.Description),
				CheckSystem:  strings.TrimSpace(raw.Check.System),
				CheckContent: strings.TrimSpace(raw.Check.CheckContent),
				FixID:        strings.TrimSpace(raw.FixRef.ID),
				FixText:      strings.TrimSpace(raw.FixText.Text),
				CCIs:         extractCCIs(raw.Idents),
			}
			if rule.FixID == "" {
				rule.FixID = strings.TrimSpace(raw.FixText.FixRef)
			}
			if rule.RuleID == "" {
				return nil, fmt.Errorf("xccdf: Rule in group %q missing id", g.ID)
			}
			b.Rules = append(b.Rules, rule)
		}
	}
	return b, nil
}

// canonicalBenchmarkID strips the DISA xccdf_mil.disa.stig_benchmark_
// prefix that appears on every published STIG so the database stores
// the short, user-recognisable form ("RHEL_8_STIG" rather than the
// fully-qualified URI form).
func canonicalBenchmarkID(raw string) string {
	const prefix = "xccdf_mil.disa.stig_benchmark_"
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, prefix) {
		return raw[len(prefix):]
	}
	return raw
}

func normaliseSeverity(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "low":
		return "low"
	case "high":
		return "high"
	case "medium":
		return "medium"
	case "":
		return "unknown"
	default:
		return "unknown"
	}
}

func extractCCIs(idents []rawIdent) []string {
	seen := make(map[string]struct{}, len(idents))
	out := make([]string, 0, len(idents))
	for _, id := range idents {
		val := strings.ToUpper(strings.TrimSpace(id.Text))
		if val == "" || !cciPrefixMatch.MatchString(val) {
			continue
		}
		val = strings.ReplaceAll(val, "CCI-", "CCI-")
		if !strings.HasPrefix(val, "CCI-") {
			// Normalise CCI000366 → CCI-000366
			val = "CCI-" + strings.TrimPrefix(val, "CCI")
		}
		if _, dup := seen[val]; dup {
			continue
		}
		seen[val] = struct{}{}
		out = append(out, val)
	}
	return out
}

// parseDate tolerates a few common formats DISA publishes (ISO 8601,
// "DD Mmm YYYY"). On parse failure it returns the zero Time, which
// the caller treats as "unknown".
func parseDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		"2006-01-02",
		"02 Jan 2006",
		"2 Jan 2006",
		"January 2, 2006",
		"01/02/2006",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
