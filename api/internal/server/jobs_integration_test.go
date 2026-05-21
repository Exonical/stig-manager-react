//go:build integration

package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Exonical/stig-manager-react/api/internal/auth"
	"github.com/Exonical/stig-manager-react/api/internal/server"
)

// withSyncJobs flips RunImmediateJob into blocking-mode so the
// integration test can assert terminal state after the response.
func withSyncJobs() serverOpt {
	return func(o *server.Options) { o.SynchronousRuns = true }
}

func jobsServer(t *testing.T, pool *pgxpool.Pool) (http.Handler, *oidcFixture) {
	t.Helper()
	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(t.Context(), auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	return newTestServer(t, withAuth(prov), withPool(pool), withSyncJobs()), fx
}

// TestJobsTasksSeeded verifies that the built-in task registry was
// installed by server.New and is reachable via GET /jobs/tasks.
func TestJobsTasksSeeded(t *testing.T) {
	pool := newIntegrationPool(t)
	handler, fx := jobsServer(t, pool)
	tokRead := fx.token(t, "stig-manager:op:read")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/jobs/tasks", nil)
	req.Header.Set("Authorization", "Bearer "+tokRead)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}
	var tasks []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &tasks); err != nil {
		t.Fatalf("decode: %v", err)
	}
	have := map[string]bool{}
	for _, tk := range tasks {
		if name, ok := tk["name"].(string); ok {
			have[name] = true
		}
	}
	for _, want := range []string{"noop", "ping", "fail"} {
		if !have[want] {
			t.Errorf("missing built-in task %q in response: %s", want, rec.Body.String())
		}
	}
}

// TestJobsAuth verifies unauthenticated and under-scoped callers get
// 401 / 403 on the /jobs surface.
func TestJobsAuth(t *testing.T) {
	pool := newIntegrationPool(t)
	handler, fx := jobsServer(t, pool)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/jobs", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: got %d body=%s", rec.Code, rec.Body.String())
	}

	tokWrong := fx.token(t, "stig-manager:collection:read")
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/jobs", nil)
	req.Header.Set("Authorization", "Bearer "+tokWrong)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("wrong scope: got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestJobsCRUDAndRun walks the full lifecycle of a job: create a job
// linked to the noop task, list it, run it synchronously, then tail
// the resulting output and confirm the run completed.
func TestJobsCRUDAndRun(t *testing.T) {
	pool := newIntegrationPool(t)
	handler, fx := jobsServer(t, pool)
	tokWrite := fx.token(t, "stig-manager:op")
	tokRead := fx.token(t, "stig-manager:op:read")

	// look up the noop task id by name via the public endpoint
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/jobs/tasks", nil)
	req.Header.Set("Authorization", "Bearer "+tokRead)
	handler.ServeHTTP(rec, req)
	var tasks []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &tasks); err != nil {
		t.Fatalf("decode tasks: %v", err)
	}
	var noopID string
	for _, tk := range tasks {
		if tk["name"] == "noop" {
			if id, ok := tk["taskId"].(string); ok {
				noopID = id
			}
		}
	}
	if noopID == "" {
		t.Fatalf("noop task not seeded: %s", rec.Body.String())
	}

	// CREATE job
	createBody := map[string]any{
		"name":        "smoke-job",
		"description": "smoke test job",
		"tasks":       []string{noopID},
	}
	enc, _ := json.Marshal(createBody)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewReader(enc))
	req.Header.Set("Authorization", "Bearer "+tokWrite)
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: got %d body=%s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	jobID, ok := created["jobId"].(string)
	if !ok || jobID == "" {
		t.Fatalf("create.jobId missing: %v", created)
	}

	// LIST jobs
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/jobs", nil)
	req.Header.Set("Authorization", "Bearer "+tokRead)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: got %d body=%s", rec.Code, rec.Body.String())
	}
	var listed []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &listed)
	if len(listed) != 1 || listed[0]["jobId"] != jobID {
		t.Fatalf("list mismatch: %s", rec.Body.String())
	}

	// PATCH description
	patchBody := map[string]any{"description": "patched"}
	enc, _ = json.Marshal(patchBody)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api/jobs/"+jobID, bytes.NewReader(enc))
	req.Header.Set("Authorization", "Bearer "+tokWrite)
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: got %d body=%s", rec.Code, rec.Body.String())
	}
	var patched map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &patched)
	if patched["description"] != "patched" {
		t.Errorf("patch description: got %v want 'patched'", patched["description"])
	}

	// RUN
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/jobs/"+jobID+"/runs", nil)
	req.Header.Set("Authorization", "Bearer "+tokWrite)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("run: got %d body=%s", rec.Code, rec.Body.String())
	}
	var runResp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &runResp)
	runID, ok := runResp["runId"].(string)
	if !ok || runID == "" {
		t.Fatalf("run.runId missing: %v", runResp)
	}

	// GET run — should be completed because SynchronousRuns=true
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/jobs/runs/"+runID, nil)
	req.Header.Set("Authorization", "Bearer "+tokRead)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get run: got %d body=%s", rec.Code, rec.Body.String())
	}
	var run map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &run)
	if state, _ := run["state"].(string); state != "completed" {
		t.Errorf("run state: got %v want 'completed' body=%s", state, rec.Body.String())
	}

	// LIST output
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/jobs/runs/"+runID+"/output", nil)
	req.Header.Set("Authorization", "Bearer "+tokRead)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get output: got %d body=%s", rec.Code, rec.Body.String())
	}
	var out []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out) == 0 {
		t.Fatalf("expected output rows; got %s", rec.Body.String())
	}
	var sawNoop, sawSystem bool
	for _, row := range out {
		if row["task"] == "noop" {
			sawNoop = true
		}
		if row["type"] == "system" {
			sawSystem = true
		}
	}
	if !sawNoop {
		t.Errorf("expected at least one row with task=noop: %s", rec.Body.String())
	}
	if !sawSystem {
		t.Errorf("expected at least one system row: %s", rec.Body.String())
	}

	// LIST runs for the job
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/jobs/"+jobID+"/runs", nil)
	req.Header.Set("Authorization", "Bearer "+tokRead)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list runs: got %d body=%s", rec.Code, rec.Body.String())
	}
	var runs []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &runs)
	if len(runs) != 1 || runs[0]["runId"] != runID {
		t.Fatalf("list runs mismatch: %s", rec.Body.String())
	}

	// DELETE run
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/jobs/runs/"+runID, nil)
	req.Header.Set("Authorization", "Bearer "+tokWrite)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete run: got %d body=%s", rec.Code, rec.Body.String())
	}

	// DELETE job
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/jobs/"+jobID, nil)
	req.Header.Set("Authorization", "Bearer "+tokWrite)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete job: got %d body=%s", rec.Code, rec.Body.String())
	}

	// GET job after delete → 404
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/jobs/"+jobID, nil)
	req.Header.Set("Authorization", "Bearer "+tokRead)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete: got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestJobsFailingTask verifies a run terminating in the failed state
// when its first task returns an error.
func TestJobsFailingTask(t *testing.T) {
	pool := newIntegrationPool(t)
	handler, fx := jobsServer(t, pool)
	tokWrite := fx.token(t, "stig-manager:op")
	tokRead := fx.token(t, "stig-manager:op:read")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/jobs/tasks", nil)
	req.Header.Set("Authorization", "Bearer "+tokRead)
	handler.ServeHTTP(rec, req)
	var tasks []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &tasks)
	var failID string
	for _, tk := range tasks {
		if tk["name"] == "fail" {
			failID, _ = tk["taskId"].(string)
		}
	}
	if failID == "" {
		t.Fatalf("fail task not seeded: %s", rec.Body.String())
	}

	createBody := map[string]any{
		"name":  "expected-failure",
		"tasks": []string{failID},
	}
	enc, _ := json.Marshal(createBody)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewReader(enc))
	req.Header.Set("Authorization", "Bearer "+tokWrite)
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: got %d body=%s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	jobID, _ := created["jobId"].(string)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/jobs/"+jobID+"/runs", nil)
	req.Header.Set("Authorization", "Bearer "+tokWrite)
	handler.ServeHTTP(rec, req)
	var runResp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &runResp)
	runID, _ := runResp["runId"].(string)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/jobs/runs/"+runID, nil)
	req.Header.Set("Authorization", "Bearer "+tokRead)
	handler.ServeHTTP(rec, req)
	var run map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &run)
	if state, _ := run["state"].(string); state != "failed" {
		t.Errorf("run state: got %v want 'failed' body=%s", state, rec.Body.String())
	}

	// after-seq tailing should return only later rows
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/jobs/runs/"+runID+"/output?after-seq="+strconv.Itoa(0), nil)
	req.Header.Set("Authorization", "Bearer "+tokRead)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("output: got %d body=%s", rec.Code, rec.Body.String())
	}
	var out []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out) == 0 {
		t.Fatalf("expected output rows; got %s", rec.Body.String())
	}
	var sawStderr bool
	for _, row := range out {
		if row["type"] == "stderr" {
			sawStderr = true
		}
	}
	if !sawStderr {
		t.Errorf("expected a stderr row from the fail task: %s", rec.Body.String())
	}
}
