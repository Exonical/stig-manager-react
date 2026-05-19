//go:build integration

package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	pgtc "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Exonical/stig-manager-react/api/internal/auth"
	"github.com/Exonical/stig-manager-react/api/internal/migrations"
	"github.com/Exonical/stig-manager-react/api/internal/server"
	"github.com/Exonical/stig-manager-react/api/internal/store"
)

func newIntegrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	container, err := pgtc.Run(ctx,
		"postgres:18",
		pgtc.WithDatabase("stigman"),
		pgtc.WithUsername("stigman"),
		pgtc.WithPassword("stigman"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(90*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	pool, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := migrations.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}

// TestCollectionsHTTPFlow walks the create/list/get HTTP surface
// against a real Postgres and a signed JWT. The auth fixture defined
// in server_test.go is reused so the auth + scope path is exercised
// in addition to the SQL layer.
func TestCollectionsHTTPFlow(t *testing.T) {
	pool := newIntegrationPool(t)
	ctx := context.Background()

	// Seed an app_user to put in the grant.
	owner, err := store.NewUserRepo(pool).Upsert(ctx, "sub-it", "alice", "Alice", "alice@example.com", nil)
	if err != nil {
		t.Fatalf("upsert owner: %v", err)
	}

	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(context.Background(), auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	handler := newTestServer(t, withAuth(prov), withPool(pool))

	// list (empty) returns [].
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/collections", nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list empty: got %d (body=%s)", rec.Code, rec.Body.String())
	}
	var listEmpty []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &listEmpty); err != nil {
		t.Fatalf("list empty json: %v", err)
	}
	if len(listEmpty) != 0 {
		t.Fatalf("list empty: got %d items", len(listEmpty))
	}

	// create returns 201 with the created shape.
	body := map[string]any{
		"name":        "Demo",
		"description": "integration",
		"grants": []map[string]any{
			{"userId": strconv.FormatInt(owner.UserID, 10), "roleId": 4},
		},
	}
	rawBody, _ := json.Marshal(body)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/collections", bytes.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: got %d (body=%s)", rec.Code, rec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("create json: %v", err)
	}
	id, _ := created["collectionId"].(string)
	if id == "" {
		t.Fatalf("create: missing collectionId in %v", created)
	}
	if created["name"] != "Demo" {
		t.Fatalf("create.name: got %v want Demo", created["name"])
	}

	// create with wrong scope (only :read) → 403.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/collections", bytes.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("create wrong scope: got %d want 403", rec.Code)
	}

	// list now returns one item.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/collections", nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list one: got %d", rec.Code)
	}
	var listOne []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &listOne); err != nil {
		t.Fatalf("list one json: %v", err)
	}
	if len(listOne) != 1 || listOne[0]["collectionId"] != id {
		t.Fatalf("list one: %v", listOne)
	}

	// get by id returns 200 + same shape.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/collections/"+id, nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: got %d (body=%s)", rec.Code, rec.Body.String())
	}
	var fetched map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("get json: %v", err)
	}
	if fetched["collectionId"] != id {
		t.Fatalf("get: id mismatch (%v vs %v)", fetched["collectionId"], id)
	}

	// get missing → 404.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/collections/99999", nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get missing: got %d want 404", rec.Code)
	}

	// duplicate name → 400.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/collections", bytes.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:collection"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create duplicate: got %d want 400", rec.Code)
	}
}

// TestConfigurationReportsMigrationVersion confirms /api/op/configuration
// surfaces the migration version applied by the API at start.
func TestConfigurationReportsMigrationVersion(t *testing.T) {
	pool := newIntegrationPool(t)
	ctx := context.Background()
	v, err := migrations.Version(ctx, pool)
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	handler := newTestServer(t, withPool(pool), withMigrationVersion(v))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/op/configuration", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("config: got %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("config json: %v", err)
	}
	lm, ok := body["lastMigration"].(float64)
	if !ok {
		t.Fatalf("lastMigration: missing or wrong type: %v", body)
	}
	if int64(lm) != v {
		t.Fatalf("lastMigration: got %v want %d", lm, v)
	}
}

func withPool(p *pgxpool.Pool) serverOpt {
	return func(o *server.Options) { o.Pool = p }
}

func withMigrationVersion(v int64) serverOpt {
	return func(o *server.Options) { o.MigrationVersion = v }
}
