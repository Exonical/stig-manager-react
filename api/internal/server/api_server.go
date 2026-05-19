package server

import (
	"encoding/json"
	"net/http"

	"github.com/Exonical/stig-manager-react/api/internal/api"
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

// Compile-time assertion that APIServer fully implements api.ServerInterface.
var _ api.ServerInterface = (*APIServer)(nil)

// GetAppInfo returns the running build's version metadata.
//
// The response is intentionally a minimal subset of upstream's AppInfo
// payload until Milestone 4 fleshes it out.
func (s APIServer) GetAppInfo(w http.ResponseWriter, _ *http.Request, _ api.GetAppInfoParams) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version":   s.Build.Version,
		"commit":    s.Build.Commit,
		"buildDate": s.Build.BuildDate,
	})
}

// GetConfiguration returns the public runtime configuration needed by the
// SPA (OIDC settings, feature flags). During scaffold milestones it only
// reports the API version and an empty feature map.
func (APIServer) GetConfiguration(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version":  "v1",
		"features": map[string]bool{},
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
