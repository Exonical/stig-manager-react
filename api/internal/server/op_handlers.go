package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/Exonical/stig-manager-react/api/internal/api"
	"github.com/Exonical/stig-manager-react/api/internal/state"
)

// requireOpRead is the shared precondition for the read-side op
// endpoints (state, appinfo, appdata).
func (s APIServer) requireOpRead(w http.ResponseWriter, r *http.Request) bool {
	return s.requiredScope(w, r, "stig-manager:op:read")
}

// requireOpWrite is the shared precondition for the write-side op
// endpoints (replace appdata).
func (s APIServer) requireOpWrite(w http.ResponseWriter, r *http.Request) bool {
	return s.requiredScope(w, r, "stig-manager:op")
}

// currentSnapshot probes the database + OIDC provider and returns the
// snapshot that GetState / SSE consumers see.
func (s APIServer) currentSnapshot(ctx context.Context) state.Snapshot {
	now := time.Now().UTC()
	snap := state.Snapshot{
		Since:        now,
		CurrentState: "starting",
	}
	if s.Broker != nil {
		// Preserve "since" across snapshots so the timestamp tracks
		// when the API process first became available rather than the
		// last probe.
		snap.Since = s.Broker.LastSnapshot().Since
	}
	if s.AppInfo != nil {
		probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if err := s.AppInfo.Ping(probeCtx); err == nil {
			snap.Db = true
		}
	}
	if s.AuthEnabled {
		snap.Oidc = true
	}
	switch {
	case s.AppInfo != nil && !snap.Db:
		snap.CurrentState = "unavailable"
	default:
		snap.CurrentState = "available"
	}
	return snap
}

// GetState — GET /op/state. Per-spec security: open.
func (s APIServer) GetState(w http.ResponseWriter, r *http.Request) {
	snap := s.currentSnapshot(r.Context())
	if s.Broker != nil {
		s.Broker.PublishSnapshot(snap)
	}
	resp := api.StateResponse{}
	cs := api.State(snap.CurrentState)
	resp.CurrentState = &cs
	dbVal := snap.Db
	oidcVal := snap.Oidc
	resp.Dependencies = &api.Dependencies{Db: &dbVal, Oidc: &oidcVal}
	if snap.UI != "" {
		ui := snap.UI
		resp.Endpoints = &api.Endpoints{Ui: &ui}
	}
	t := snap.Since
	resp.Since = &t
	writeJSON(w, http.StatusOK, resp)
}

// StreamStateSse — GET /op/state/sse. Server-Sent Events stream.
//
// First frame is always a state.snapshot so a fresh client renders
// instantly; subsequent frames are pushed whenever the broker
// publishes (state change, job run transition, …). A 25-second heart-
// beat keeps reverse proxies from idle-timing the connection.
func (s APIServer) StreamStateSse(w http.ResponseWriter, r *http.Request) {
	if s.Broker == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "state broker not configured")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeAuthError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	// Refresh the snapshot so the inaugural event reflects current
	// dependency state rather than whatever was cached at boot.
	s.Broker.PublishSnapshot(s.currentSnapshot(r.Context()))

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	ctx := r.Context()
	ch, cancel := s.Broker.Subscribe(ctx)
	defer cancel()

	keepAlive := time.NewTicker(25 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if err := writeSSE(w, ev); err != nil {
				return
			}
			flusher.Flush()
		case <-keepAlive.C:
			if _, err := w.Write([]byte(":keepalive\n\n")); err != nil {
				return
			}
			flusher.Flush()
		case <-ctx.Done():
			return
		}
	}
}

func writeSSE(w http.ResponseWriter, ev state.Event) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", ev.Type); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return err
	}
	return nil
}

// GetAppInfo — GET /op/appinfo. Returns a structurally-stable
// summary of the running deployment: build stamps, schema version,
// row counts, Postgres status and request counters. Many sub-fields
// of upstream's AppInfo schema are intentionally left empty in this
// milestone (e.g. per-collection grant matrices) because their
// implementation is non-trivial and not blocking.
func (s APIServer) GetAppInfo(w http.ResponseWriter, r *http.Request, _ api.GetAppInfoParams) {
	if !s.requireOpRead(w, r) {
		return
	}
	resp := map[string]any{
		"version":   s.Build.Version,
		"commit":    s.Build.Commit,
		"buildDate": s.Build.BuildDate,
		"date":      time.Now().UTC().Format(time.RFC3339),
		"schema":    fmt.Sprintf("%d", s.MigrationVersion),
	}

	// Counts surface user / collection / asset / stig / review /
	// job / run totals.
	if s.AppInfo != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if v, err := s.AppInfo.SchemaVersion(ctx); err == nil && v > 0 {
			resp["schema"] = fmt.Sprintf("%d", v)
		}
		if c, err := s.AppInfo.Counts(ctx); err == nil {
			resp["counts"] = map[string]any{
				"users":       c.Users,
				"userGroups":  c.UserGroups,
				"collections": c.Collections,
				"assets":      c.Assets,
				"stigs":       c.Stigs,
				"reviews":     c.Reviews,
				"jobs":        c.Jobs,
				"runs":        c.Runs,
			}
		}
		if pg, err := s.AppInfo.PostgresStatus(ctx); err == nil {
			resp["postgres"] = map[string]any{
				"version":     pg.Version,
				"startTime":   pg.StartTime.UTC().Format(time.RFC3339),
				"uptime":      pg.Uptime.Seconds(),
				"connections": pg.Connections,
				"variables":   pg.Variables,
			}
		}
	}

	// Go runtime instead of node "nodejs" — keys mirror the upstream
	// shape so the SPA's status table renders something useful.
	memStats := runtime.MemStats{}
	runtime.ReadMemStats(&memStats)
	resp["runtime"] = map[string]any{
		"goroutines": runtime.NumGoroutine(),
		"cpus":       runtime.NumCPU(),
		"goVersion":  runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"uptime":     time.Since(serverStartTime).Seconds(),
		"memory": map[string]any{
			"heapAlloc": memStats.HeapAlloc,
			"heapInuse": memStats.HeapInuse,
			"stackSys":  memStats.StackSys,
			"sys":       memStats.Sys,
		},
	}

	if s.RequestCounter != nil {
		rs := s.RequestCounter.Snapshot()
		ops := make(map[string]map[string]any, len(rs.Operations))
		for k, v := range rs.Operations {
			ops[k] = map[string]any{
				"totalRequests": v.TotalRequests,
				"totalDuration": v.TotalDuration,
				"minDuration":   v.MinDuration,
				"maxDuration":   v.MaxDuration,
				"errors":        v.Errors,
			}
		}
		resp["requests"] = map[string]any{
			"totalRequests":        rs.TotalRequests,
			"totalApiRequests":     rs.TotalAPIRequests,
			"totalRequestDuration": rs.TotalDuration,
			"totalErrors":          rs.TotalErrors,
			"operationIds":         ops,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetAppDataTables — GET /op/appdata/tables.
func (s APIServer) GetAppDataTables(w http.ResponseWriter, r *http.Request, _ api.GetAppDataTablesParams) {
	if !s.requireOpRead(w, r) {
		return
	}
	if s.AppInfo == nil {
		writeJSON(w, http.StatusOK, []api.AppDataTable{})
		return
	}
	tables, err := s.AppInfo.Tables(r.Context())
	if err != nil {
		s.logErr(r, "list tables", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to list tables")
		return
	}
	out := make([]api.AppDataTable, 0, len(tables))
	for _, t := range tables {
		name := api.String255(t.Name)
		rowsF := float32(t.Rows)
		dataLenF := float32(t.DataLength)
		out = append(out, api.AppDataTable{
			Name:       &name,
			Rows:       &rowsF,
			DataLength: &dataLenF,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// GetAppData — GET /op/appdata. JSON export of admin tables.
func (s APIServer) GetAppData(w http.ResponseWriter, r *http.Request, _ api.GetAppDataParams) {
	if !s.requireOpRead(w, r) {
		return
	}
	if s.AppData == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "appdata export not configured")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="appdata.json"`)
	w.WriteHeader(http.StatusOK)
	if err := s.AppData.Export(r.Context(), w); err != nil {
		s.logErr(r, "export appdata", err)
	}
}

// ReplaceAppData — POST /op/appdata. Staged for a follow-up
// milestone; returns a 503 so callers can detect the missing
// capability rather than a generic 501.
func (s APIServer) ReplaceAppData(w http.ResponseWriter, r *http.Request, _ api.ReplaceAppDataParams) {
	if !s.requireOpWrite(w, r) {
		return
	}
	writeAuthError(w, http.StatusServiceUnavailable,
		"appdata import is staged for a future milestone (M16+); export via GET /op/appdata is supported today")
}

var serverStartTime = time.Now().UTC()
