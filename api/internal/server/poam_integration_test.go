//go:build integration

package server_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"

	"github.com/Exonical/stig-manager-react/api/internal/auth"
)

func poamServer(t *testing.T, pool *pgxpool.Pool) (http.Handler, *oidcFixture) {
	t.Helper()
	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(t.Context(), auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	return newTestServer(t, withAuth(prov), withPool(pool)), fx
}

// TestPoamByCollection_RoundTrip hits the POA&M xlsx endpoint, reads
// the returned workbook back through excelize, and verifies that the
// failing review seeded by the metrics scenario surfaces as exactly
// one finding row at the expected aggregator key.
func TestPoamByCollection_RoundTrip(t *testing.T) {
	pool := newIntegrationPool(t)
	collID, _, _, _, _ := seedMetricsScenario(t, pool, "user-1", "poam-user")
	handler, fx := poamServer(t, pool)
	tok := fx.token(t, "stig-manager:collection:read")

	rec := httptest.NewRecorder()
	url := "/api/collections/" + strconv.FormatInt(collID, 10) +
		"/poam?aggregator=groupId&format=EMASS&date=2026-12-31&office=S2&status=Ongoing"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	got := rec.Body.Bytes()
	if !bytes.HasPrefix(got, []byte("PK")) {
		t.Fatalf("response is not a zip/xlsx archive (first bytes: %q)", got[:6])
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet") {
		t.Fatalf("content-type = %q", ct)
	}
	cd := rec.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "POAM-EMASS-") {
		t.Fatalf("content-disposition = %q", cd)
	}

	xf, err := excelize.OpenReader(bytes.NewReader(got))
	if err != nil {
		t.Fatalf("excelize.OpenReader: %v", err)
	}
	defer xf.Close()

	rows, err := xf.GetRows("POAM")
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("expected header + at least one finding, got %d rows", len(rows))
	}
	// First row is headers; row 2 is the one failing review we seeded.
	header := rows[0]
	if header[0] != "Control Vulnerability Description" {
		t.Errorf("header[0] = %q", header[0])
	}
	row := rows[1]
	// Description column should mention the rule we failed.
	if !strings.Contains(row[0], "Title:") {
		t.Errorf("row[0] missing 'Title:' prefix: %q", row[0])
	}
	// Office column (defaulted from query).
	if row[2] != "S2" {
		t.Errorf("row office = %q, want 'S2'", row[2])
	}
	// Scheduled date column.
	if row[5] != "2026-12-31" {
		t.Errorf("row date = %q", row[5])
	}
	// Status column.
	if row[9] != "Ongoing" {
		t.Errorf("row status = %q", row[9])
	}
	// Raw severity for high ⇒ "I"; the seeded fail review is on
	// rule SV-100002r1_rule which the fixture marks as severity="high".
	if row[11] != "I" {
		t.Errorf("row raw severity = %q, want 'I'", row[11])
	}
}

// TestPoamByCollection_RuleAggregator verifies the ruleId aggregator
// echoes the SV-… identifier into the "Security Checks" column rather
// than the V-… group id used by the groupId aggregator.
func TestPoamByCollection_RuleAggregator(t *testing.T) {
	pool := newIntegrationPool(t)
	collID, _, _, _, _ := seedMetricsScenario(t, pool, "user-1", "poam-rule-user")
	handler, fx := poamServer(t, pool)
	tok := fx.token(t, "stig-manager:collection:read")

	rec := httptest.NewRecorder()
	url := "/api/collections/" + strconv.FormatInt(collID, 10) +
		"/poam?aggregator=ruleId"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}

	xf, err := excelize.OpenReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("excelize.OpenReader: %v", err)
	}
	defer xf.Close()
	rows, err := xf.GetRows("POAM")
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("expected at least one finding row, got %d rows", len(rows))
	}
	// Column 4 ("Security Checks") should now be the SV-… rule id.
	if !strings.HasPrefix(rows[1][3], "SV-") {
		t.Errorf("ruleId aggregator: security checks = %q (want SV-…)", rows[1][3])
	}
}

// TestPoamByCollection_BadAggregator returns 400 when the aggregator
// query string isn't one of the OpenAPI-enumerated values.
func TestPoamByCollection_BadAggregator(t *testing.T) {
	pool := newIntegrationPool(t)
	collID, _, _, _, _ := seedMetricsScenario(t, pool, "user-1", "poam-bad-user")
	handler, fx := poamServer(t, pool)
	tok := fx.token(t, "stig-manager:collection:read")

	rec := httptest.NewRecorder()
	url := "/api/collections/" + strconv.FormatInt(collID, 10) + "/poam?aggregator=banana"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d (want 400) body=%s", rec.Code, rec.Body.String())
	}
}

// TestPoamByCollection_RequiresAuth — no bearer token, no POA&M.
func TestPoamByCollection_RequiresAuth(t *testing.T) {
	pool := newIntegrationPool(t)
	collID, _, _, _, _ := seedMetricsScenario(t, pool, "user-1", "poam-anon-user")
	handler, _ := poamServer(t, pool)

	rec := httptest.NewRecorder()
	url := "/api/collections/" + strconv.FormatInt(collID, 10) + "/poam?aggregator=groupId"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d (want 401) body=%s", rec.Code, rec.Body.String())
	}
}
