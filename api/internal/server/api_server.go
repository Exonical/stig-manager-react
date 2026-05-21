package server

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/Exonical/stig-manager-react/api/internal/api"
	"github.com/Exonical/stig-manager-react/api/internal/auth"
	"github.com/Exonical/stig-manager-react/api/internal/jobs"
	"github.com/Exonical/stig-manager-react/api/internal/state"
	"github.com/Exonical/stig-manager-react/api/internal/store"
)

// APIServer is the concrete implementation of api.ServerInterface used by
// the running HTTP server. It embeds api.Unimplemented (the generator-
// supplied 501 stub for every operation) and overrides individual methods
// as features land.
//
// As of Milestone 4 the overrides are: the operational endpoints
// (`/api/op/appinfo`, `/api/op/configuration`) and a first slice of
// the collections surface (list, get, create).
type APIServer struct {
	api.Unimplemented

	Build AppInfoBuild

	// Logger is used for unexpected handler errors. Optional; falls
	// back to slog.Default when nil.
	Logger *slog.Logger

	// MigrationVersion is the version of the most recently applied
	// database migration at server start-up. Exposed via
	// /api/op/configuration. Zero when migrations were not run.
	MigrationVersion int64

	// Users persists / refreshes the app_user row for each
	// authenticated principal. Optional: when nil the create
	// endpoint returns 503.
	Users *store.UserRepo
	// Collections is the data-layer entry point for the collections
	// endpoints. Optional with the same fall-back semantics as Users.
	Collections *store.CollectionRepo
	// Stigs is the data-layer entry point for the STIG library
	// endpoints (`/stigs`, `/stigs/{benchmarkId}`,
	// `/stigs/rules/{ruleId}`, `/stigs/ccis/{cci}`). Optional with the
	// same fall-back semantics as the other repos.
	Stigs *store.STIGRepo
	// Assets is the data-layer entry point for `/assets/*` and
	// `/collections/{cid}/assets/*` endpoints.
	Assets *store.AssetRepo
	// Labels is the data-layer entry point for
	// `/collections/{cid}/labels/*` endpoints.
	Labels *store.LabelRepo
	// Grants is the data-layer entry point for
	// `/collections/{cid}/grants/*` endpoints; also resolves the
	// effective role for the requesting user when authorising
	// per-collection writes.
	Grants *store.GrantRepo
	// Reviews is the data-layer entry point for
	// `/collections/{cid}/reviews/*` endpoints (single-asset evaluator
	// workspace + append-only history).
	Reviews *store.ReviewRepo
	// Metrics aggregates compliance metrics for
	// `/collections/{cid}/metrics/summary*` endpoints.
	Metrics *store.MetricsRepo
	// Checklists assembles export payloads for the CKL / CKLB /
	// XCCDF endpoints and the collection-level checklist summary.
	Checklists *store.ChecklistRepo
	// Poam aggregates failing reviews into POA&M xlsx findings.
	Poam *store.PoamRepo
	// UserGroups is the data-layer entry point for `/user-groups/*`
	// endpoints. Optional with the same fall-back semantics as the
	// other repos.
	UserGroups *store.UserGroupRepo
	// Jobs persists scheduled jobs, runs and run-output rows that
	// power the `/jobs/*` endpoints.  Optional with the same fall-
	// back semantics as the other repos.
	Jobs *store.JobRepo
	// JobRunner executes the in-process task registry for an
	// immediate run.  Nil when jobs are not wired (immediate-run
	// responses fall back to 503).
	JobRunner *jobs.Runner
	// SynchronousRuns forces RunImmediateJob to wait for the runner
	// before returning. Useful for tests; production should leave
	// this false so the request returns 202 immediately.
	SynchronousRuns bool

	// AppInfo answers the lightweight queries that back
	// GetAppInfo, GetAppDataTables and GetState (table sizes, row
	// counts, schema version, db ping).  Optional; nil disables the
	// db-derived fields.
	AppInfo *store.AppInfoRepo
	// AppData implements the JSON export consumed by GetAppData.
	AppData *store.AppDataRepo
	// Broker is the publish/subscribe fan-out that powers
	// /op/state/sse.  Nil disables the SSE endpoint.
	Broker *state.Broker
	// RequestCounter accumulates per-route request statistics for
	// the /op/appinfo "requests" section.
	RequestCounter *RequestCounter
	// AuthEnabled is true when an OIDC provider was configured at
	// boot.  Surfaces via /op/state.dependencies.oidc.
	AuthEnabled bool
}

func (s APIServer) logErr(r *http.Request, op string, err error) {
	logger := s.Logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.ErrorContext(r.Context(), op, "err", err, "path", r.URL.Path)
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

// GetConfiguration returns the public runtime configuration needed by
// the SPA. Per the OpenAPI spec this endpoint is `security: []` —
// reachable without a token — so OIDC settings discovered here can be
// used by the SPA to start a login.
//
// The response shape matches upstream's ApiConfiguration; lastMigration
// reflects the most-recently-applied goose version observed at server
// start.
func (s APIServer) GetConfiguration(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version":        "v1",
		"classification": "U",
		"commit": map[string]any{
			"sha": s.Build.Commit,
		},
		"lastMigration": s.MigrationVersion,
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
