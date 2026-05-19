package server

import (
	"encoding/json"
	"net/http"

	"github.com/Exonical/stig-manager-react/api/internal/api"
	"github.com/Exonical/stig-manager-react/api/internal/auth"
)

// APIServer is the concrete implementation of api.ServerInterface used by
// the running HTTP server. It embeds api.Unimplemented (the generator-
// supplied 501 stub for every operation) and overrides individual methods
// as features land.
//
// Through Milestone 4 the only overrides are the operational endpoints
// (`/api/op/appinfo`, `/api/op/configuration`). Real
// STIG/Collection/Asset handlers arrive in later milestones.
type APIServer struct {
	api.Unimplemented

	Build AppInfoBuild
}

// AppInfoBuild captures build-time stamping for the appinfo endpoint.
type AppInfoBuild struct {
	Version   string
	Commit    string
	BuildDate string
}

// requiredScope guards an operation behind a scope check. Returns true
// if the request is authorised; otherwise writes the appropriate error
// status and returns false. Use:
//
//	if !s.requiredScope(w, r, "stig-manager:op:read") {
//		return
//	}
func (APIServer) requiredScope(w http.ResponseWriter, r *http.Request, scope string) bool {
	user, ok := auth.FromContext(r.Context())
	if !ok {
		writeAuthError(w, http.StatusUnauthorized, "authentication required")
		return false
	}
	if !user.HasScope(scope) {
		writeAuthError(w, http.StatusForbidden, "missing required scope: "+scope)
		return false
	}
	return true
}

// Compile-time assertion that APIServer fully implements api.ServerInterface.
var _ api.ServerInterface = (*APIServer)(nil)

// GetAppInfo returns the running build's version metadata.
//
// The response is intentionally a minimal subset of upstream's AppInfo
// payload until Milestone 4 fleshes it out. The OpenAPI spec gates this
// endpoint behind `stig-manager:op:read`.
func (s APIServer) GetAppInfo(w http.ResponseWriter, r *http.Request, _ api.GetAppInfoParams) {
	if !s.requiredScope(w, r, "stig-manager:op:read") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version":   s.Build.Version,
		"commit":    s.Build.Commit,
		"buildDate": s.Build.BuildDate,
	})
}

// GetConfiguration returns the public runtime configuration needed by
// the SPA. Per the OpenAPI spec this endpoint is `security: []` —
// reachable without a token — so OIDC settings discovered here can be
// used by the SPA to start a login.
//
// The response shape matches upstream's ApiConfiguration. Fields that
// are populated by the database layer (lastMigration) stay zero-valued
// until Milestone 4 wires them up.
func (s APIServer) GetConfiguration(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version":        "v1",
		"classification": "U",
		"commit": map[string]any{
			"sha": s.Build.Commit,
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeAuthError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{
		"error":  http.StatusText(status),
		"detail": msg,
	})
}
