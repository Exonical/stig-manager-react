//go:build integration

package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Exonical/stig-manager-react/api/internal/auth"
	"github.com/Exonical/stig-manager-react/api/internal/store"
	"github.com/Exonical/stig-manager-react/api/internal/xccdf"
)

// republishedFixture seeds two revisions of the test STIG with a
// hand-picked rule layout that exercises every branch of the new
// `rules` filter. It returns the http handler, the JWT fixture, the
// collection id, and the asset id.
//
//	Revision 1 (older):
//	  SV-100001r1_rule  ver=TEST-OS-000010  grp=V-100001
//	  SV-100002r1_rule  ver=TEST-OS-000020  grp=V-100002
//
//	Revision 2 (newer = default):
//	  SV-100001r2_rule  ver=TEST-OS-000010  grp=V-100001   (republishes the r1 rule)
//	  SV-300001r2_rule  ver=TEST-OS-000030  grp=V-300001   (new)
//
// Reviews (asset = host-republished):
//	  SV-100001r2_rule  → in_default=true,  mapped=true   (current)
//	  SV-300001r2_rule  → in_default=true,  mapped=true   (current, brand new)
//	  SV-100001r1_rule  → in_default=false, mapped=true   (republished orphan)
//	  SV-100002r1_rule  → in_default=false, mapped=false  (dropped orphan)
func republishedFixture(t *testing.T) (http.Handler, *oidcFixture, *pgxpool.Pool, int64, int64) {
	t.Helper()
	ctx := context.Background()

	pool := newIntegrationPool(t)
	// JWT fixture issues sub="user-1"; seed that user as the Owner.
	userID, collID := seedCollectionWithOwner(t, pool, "user-1", "rachel")

	// Revision 1 comes from the canonical fixture.
	f, err := os.Open("../xccdf/testdata/sample.xccdf.xml")
	if err != nil {
		t.Fatalf("open xccdf fixture: %v", err)
	}
	defer f.Close()
	bench, err := xccdf.Parse(f)
	if err != nil {
		t.Fatalf("parse xccdf fixture: %v", err)
	}
	rev1, err := store.NewSTIGRepo(pool).ImportRevision(ctx, bench, true)
	if err != nil {
		t.Fatalf("import revision: %v", err)
	}
	benchID := bench.BenchmarkID
	_ = rev1

	// Revision 2 — same benchmark, newer date, with one republished
	// rule and one brand-new rule. Inserted directly into the
	// underlying tables so the test stays self-contained instead of
	// relying on a second XML fixture.
	var rev2ID int64
	if err := pool.QueryRow(ctx, `
INSERT INTO stig_revision (benchmark_id, revision_str, version, release, release_date, benchmark_date, status, description, source)
VALUES ($1, 'V2R1', '2', '1', CURRENT_DATE, CURRENT_DATE, 'accepted', '', '')
RETURNING revision_id`, benchID).Scan(&rev2ID); err != nil {
		t.Fatalf("insert rev2: %v", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO stig_rule (revision_id, rule_id, version_str, group_id, title, severity)
VALUES ($1, 'SV-100001r2_rule', 'TEST-OS-000010', 'V-100001', 'republished rule', 'medium'),
       ($1, 'SV-300001r2_rule', 'TEST-OS-000030', 'V-300001', 'brand-new rule',  'low')
`, rev2ID); err != nil {
		t.Fatalf("insert rev2 rules: %v", err)
	}

	// Asset binding — no pinned revision so the default-rev lookup
	// resolves to revision 2 via the benchmark_date ordering.
	assets := store.NewAssetRepo(pool)
	a, err := assets.Create(ctx, store.AssetCreate{
		CollectionID: collID, Name: "host-republished", Noncomputing: false,
	})
	if err != nil {
		t.Fatalf("create asset: %v", err)
	}
	stigs := []string{benchID}
	if _, err := assets.Update(ctx, a.AssetID, store.AssetUpdate{BenchmarkIDs: &stigs}); err != nil {
		t.Fatalf("assign stig: %v", err)
	}

	// Four reviews exercising every quadrant of (in_default,
	// mapped_to_default).
	reviews := store.NewReviewRepo(pool)
	for _, rid := range []string{
		"SV-100001r2_rule", "SV-300001r2_rule",
		"SV-100001r1_rule", "SV-100002r1_rule",
	} {
		if _, err := reviews.Put(ctx, a.AssetID, rid, userID, store.ReviewWrite{
			Result: "pass", Comment: rid, StatusLabel: "saved",
		}); err != nil {
			t.Fatalf("put review %s: %v", rid, err)
		}
	}

	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(ctx, auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	handler := newTestServer(t, withAuth(prov), withPool(pool))
	return handler, fx, pool, collID, a.AssetID
}

// listReviewRuleIDs hits /collections/{cid}/reviews?rules=<filter> and
// returns the rule_ids in deterministic order so the assertions are
// readable.
func listReviewRuleIDs(t *testing.T, handler http.Handler, fx *oidcFixture, collID int64, rulesFilter string) []string {
	t.Helper()
	url := "/api/collections/" + strconv.FormatInt(collID, 10) + "/reviews"
	if rulesFilter != "" {
		url += "?rules=" + rulesFilter
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list reviews (rules=%q): got %d body=%s", rulesFilter, rec.Code, rec.Body.String())
	}
	var raw []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode body (rules=%q): %v", rulesFilter, err)
	}
	out := make([]string, 0, len(raw))
	for _, row := range raw {
		ids, _ := row["ruleIds"].([]any)
		for _, raw := range ids {
			if s, ok := raw.(string); ok {
				out = append(out, s)
			}
		}
	}
	sort.Strings(out)
	return out
}

// TestReviewsRulesFilter walks every Rules enum value against the
// republished-fixture and asserts the rule_ids returned.
func TestReviewsRulesFilter(t *testing.T) {
	handler, fx, _, collID, _ := republishedFixture(t)

	cases := []struct {
		name   string
		filter string
		want   []string
	}{
		{
			name:   "all",
			filter: "all",
			want:   []string{"SV-100001r1_rule", "SV-100001r2_rule", "SV-100002r1_rule", "SV-300001r2_rule"},
		},
		{
			name:   "default",
			filter: "default",
			want:   []string{"SV-100001r2_rule", "SV-300001r2_rule"},
		},
		{
			name:   "not-default",
			filter: "not-default",
			want:   []string{"SV-100001r1_rule", "SV-100002r1_rule"},
		},
		{
			name:   "mapped",
			filter: "mapped",
			want:   []string{"SV-100001r1_rule", "SV-100001r2_rule", "SV-300001r2_rule"},
		},
		{
			name:   "default-mapped",
			filter: "default-mapped",
			want:   []string{"SV-100001r1_rule", "SV-100001r2_rule", "SV-300001r2_rule"},
		},
		{
			name:   "not-mapped",
			filter: "not-mapped",
			want:   []string{"SV-100002r1_rule"},
		},
		{
			name:   "not-default-mapped",
			filter: "not-default-mapped",
			want:   []string{"SV-100001r1_rule"},
		},
		{
			name:   "no filter (empty)",
			filter: "",
			want:   []string{"SV-100001r1_rule", "SV-100001r2_rule", "SV-100002r1_rule", "SV-300001r2_rule"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := listReviewRuleIDs(t, handler, fx, collID, tc.filter)
			if !equalSorted(got, tc.want) {
				t.Fatalf("rules=%q: got %v want %v", tc.filter, got, tc.want)
			}
		})
	}
}

// TestReviewsRulesFilterByAsset confirms the asset-scoped endpoint
// honours the same filter (the SPA's per-asset Reviews workspace
// drives this code path).
func TestReviewsRulesFilterByAsset(t *testing.T) {
	handler, fx, _, collID, assetID := republishedFixture(t)
	url := "/api/collections/" + strconv.FormatInt(collID, 10) +
		"/reviews/" + strconv.FormatInt(assetID, 10) + "?rules=not-default-mapped"
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list by asset: got %d body=%s", rec.Code, rec.Body.String())
	}
	var raw []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(raw) != 1 {
		t.Fatalf("expected 1 republished orphan, got %d (%v)", len(raw), raw)
	}
	ids, _ := raw[0]["ruleIds"].([]any)
	if len(ids) == 0 {
		t.Fatalf("ruleIds missing: %v", raw[0])
	}
	if got, _ := ids[0].(string); got != "SV-100001r1_rule" {
		t.Fatalf("wrong republished rule: %s", got)
	}
}

func equalSorted(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
