// Package handlers exposes HTTP handlers used by the API router.
//
// These are intentionally minimal scaffold endpoints — the full /api/v1
// surface lands in subsequent milestones via oapi-codegen.
package handlers

import (
	"encoding/json"
	"net/http"
)

// AppInfoBuild captures build-time stamping for the appinfo endpoint.
type AppInfoBuild struct {
	Version   string `json:"version"`
	Commit    string `json:"commit,omitempty"`
	BuildDate string `json:"buildDate,omitempty"`
}

// Health is a simple liveness probe.
func Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// AppInfo returns build metadata for the running API.
func AppInfo(build AppInfoBuild) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, build)
	}
}

// AppDataTables returns an empty table list during scaffold milestones.
func AppDataTables(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, []string{})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
