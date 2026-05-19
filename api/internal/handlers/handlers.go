// Package handlers exposes top-level HTTP handlers that sit outside the
// OpenAPI surface (e.g. the unauthenticated liveness probe).
//
// All routes under /api/* are served by the oapi-codegen generated chi
// router; see internal/server/api_server.go for the concrete handler
// implementations.
package handlers

import (
	"encoding/json"
	"net/http"
)

// Health is a simple liveness probe. It is mounted at /health by the
// server package and intentionally lives outside the OpenAPI surface so
// orchestrators can hit it without a token.
func Health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
