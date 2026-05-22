// Package server wires the HTTP router for the STIG Manager API.
package server

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Exonical/stig-manager-react/api/internal/api"
	"github.com/Exonical/stig-manager-react/api/internal/audit"
	"github.com/Exonical/stig-manager-react/api/internal/auth"
	"github.com/Exonical/stig-manager-react/api/internal/config"
	"github.com/Exonical/stig-manager-react/api/internal/handlers"
	"github.com/Exonical/stig-manager-react/api/internal/jobs"
	"github.com/Exonical/stig-manager-react/api/internal/ratelimit"
	"github.com/Exonical/stig-manager-react/api/internal/state"
	"github.com/Exonical/stig-manager-react/api/internal/store"
)

// Options describes inputs needed to construct a Server.
type Options struct {
	Logger    *slog.Logger
	Version   string
	Commit    string
	BuildDate string
	Config    *config.Config
	// AuthProvider validates access tokens. When nil the server runs
	// without authentication; protected endpoints will 401.
	AuthProvider *auth.Provider
	// Pool is the Postgres connection pool. Optional: when nil, the
	// database-backed handlers degrade gracefully (collections list
	// returns [], get/create return 404/503).
	Pool *pgxpool.Pool
	// MigrationVersion is the version of the most recently applied
	// goose migration, surfaced via /api/op/configuration.
	MigrationVersion int64
	// SynchronousRuns flips RunImmediateJob into a blocking mode that
	// drives the runner to completion before responding. Only used by
	// the integration tests; production leaves this false so the
	// endpoint returns 202 immediately.
	SynchronousRuns bool
}

// Server holds the HTTP router and its dependencies.
type Server struct {
	opts      Options
	router    http.Handler
	Scheduler *jobs.Scheduler
}

// New constructs a Server with the given options.
func New(opts Options) *Server {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	var auditRepo *store.AuditRepo
	if opts.Pool != nil {
		auditRepo = store.NewAuditRepo(opts.Pool)
	}

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   opts.Config.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Requested-With"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	requestCounter := NewRequestCounter()
	r.Use(requestCounter.Middleware)

	// SSE handlers need to run untouched by the request timeout —
	// otherwise long-lived streams get killed at 60s. Apply the
	// timeout only to non-SSE paths.
	r.Use(func(next http.Handler) http.Handler {
		timed := middleware.Timeout(60 * time.Second)(next)
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.URL.Path == "/api/op/state/sse" {
				next.ServeHTTP(w, req)
				return
			}
			timed.ServeHTTP(w, req)
		})
	})

	// Attach the authenticated user (if any) to the request context.
	// Per-handler scope checks gate individual operations; configuration
	// and Env.js stay reachable without a token.
	r.Use(auth.Middleware(opts.AuthProvider, opts.Logger))

	// Per-client rate limiting runs after auth so authenticated
	// callers are bucketed by their stable Subject claim rather than
	// the source IP. Long-lived SSE streams and the public health
	// endpoint are exempt.
	if opts.Config != nil && opts.Config.RateLimit.Enabled {
		rl := ratelimit.New(ratelimit.Config{
			RequestsPerSecond: opts.Config.RateLimit.Rate,
			Burst:             opts.Config.RateLimit.Burst,
			Enabled:           true,
		})
		r.Use(rl.Middleware("/api/op/state/sse", "/health"))
	}

	// Audit middleware records every mutating /api/* request after the
	// rate limiter has accepted it. Reads, /js/Env.js, and /health are
	// skipped inside the middleware. When no Pool is configured the
	// middleware degrades to a no-op (recorder is nil).
	r.Use(audit.Middleware(auditRepo, audit.Config{Logger: opts.Logger}))

	// Liveness probe lives outside the OpenAPI surface so orchestrators
	// can hit it without a token. The generated router handles
	// everything under /api.
	r.Get("/health", handlers.Health)

	// /js/Env.js is upstream's mechanism for delivering SPA-side OIDC
	// settings without rebuilding the bundle.
	r.Get("/js/Env.js", EnvScript{
		Cfg:     opts.Config,
		Version: opts.Version,
		Commit:  opts.Commit,
	}.ServeHTTP)

	broker := state.NewBroker()

	apiServer := APIServer{
		Build: AppInfoBuild{
			Version:   opts.Version,
			Commit:    opts.Commit,
			BuildDate: opts.BuildDate,
		},
		Logger:           opts.Logger,
		MigrationVersion: opts.MigrationVersion,
		SynchronousRuns:  opts.SynchronousRuns,
		Broker:           broker,
		RequestCounter:   requestCounter,
		AuthEnabled:      opts.AuthProvider != nil,
	}
	if opts.Pool != nil {
		apiServer.Audit = auditRepo
		apiServer.Users = store.NewUserRepo(opts.Pool)
		apiServer.Collections = store.NewCollectionRepo(opts.Pool)
		apiServer.Stigs = store.NewSTIGRepo(opts.Pool)
		apiServer.Assets = store.NewAssetRepo(opts.Pool)
		apiServer.Labels = store.NewLabelRepo(opts.Pool)
		apiServer.Grants = store.NewGrantRepo(opts.Pool)
		apiServer.Reviews = store.NewReviewRepo(opts.Pool)
		apiServer.Metrics = store.NewMetricsRepo(opts.Pool)
		apiServer.Checklists = store.NewChecklistRepo(opts.Pool)
		apiServer.Poam = store.NewPoamRepo(opts.Pool)
		apiServer.UserGroups = store.NewUserGroupRepo(opts.Pool)
		apiServer.Jobs = store.NewJobRepo(opts.Pool)
		apiServer.AppInfo = store.NewAppInfoRepo(opts.Pool)
		apiServer.AppData = store.NewAppDataRepo(opts.Pool)

		// Seed the built-in task registry into job_task and wire the
		// runner.  Seeding is idempotent so it is safe to run on every
		// boot; errors are logged but do not block startup, mirroring
		// the conservative bootstrap convention used by the other
		// repos.
		registry := jobs.NewBuiltinRegistry()
		if err := registry.Seed(context.Background(), apiServer.Jobs); err != nil {
			opts.Logger.Error("seed job task registry", "err", err)
		}
		apiServer.JobRunner = jobs.NewRunner(apiServer.Jobs, registry, opts.Logger)
		apiServer.JobRunner.SetListener(brokerJobListener{broker: broker})
	}

	// Wire the cron loop that dispatches scheduled jobs. The
	// scheduler is only constructed; the caller (cmd/stigman) starts
	// the goroutine alongside the http listener and cancels its
	// context on shutdown.
	var scheduler *jobs.Scheduler
	if opts.Pool != nil && opts.Config != nil && opts.Config.Scheduler.Enabled {
		s, err := jobs.NewScheduler(apiServer.Jobs, apiServer.JobRunner, jobs.SchedulerConfig{
			Tick:   opts.Config.Scheduler.TickFreq,
			Logger: opts.Logger,
		})
		if err != nil {
			opts.Logger.Error("scheduler init", "err", err)
		} else {
			scheduler = s
		}
	}

	// /api/op/audit-log is an admin-only read endpoint that surfaces
	// the audit_log rows captured by the audit middleware. It is not
	// part of the upstream OpenAPI surface (the table itself is
	// stig-manager-react-specific), so we wire it directly on the
	// root chi router before the generated handlers are mounted.
	r.Get("/api/op/audit-log", apiServer.GetAuditLog)

	// Register the generated handlers directly onto the root chi router
	// at /api/* so the existing middleware stack (RequestID, CORS, etc.)
	// applies. This matches upstream's URL layout: paths in the OpenAPI
	// spec are relative to /api.
	api.HandlerFromMuxWithBaseURL(apiServer, r, "/api")

	return &Server{opts: opts, router: r, Scheduler: scheduler}
}

// Router exposes the configured http.Handler.
func (s *Server) Router() http.Handler { return s.router }

// brokerJobListener bridges the jobs.Listener interface to the SSE
// broker so /op/state/sse subscribers see "job.run" events whenever a
// run transitions state.
type brokerJobListener struct{ broker *state.Broker }

func (b brokerJobListener) JobRunTransition(runID uuid.UUID, jobID int64, jobName, runState, message string) {
	if b.broker == nil {
		return
	}
	b.broker.Publish(state.Event{
		Type: state.EventJobRun,
		Data: state.JobRunEvent{
			RunID:   runID.String(),
			JobID:   strconv.FormatInt(jobID, 10),
			State:   runState,
			Message: message,
		},
	})
}

// BuildAuthProvider returns a configured *auth.Provider, or nil when
// cfg.OIDC.Issuer is empty (unauthenticated dev mode). Errors are
// returned only when discovery against a configured issuer fails.
func BuildAuthProvider(ctx context.Context, cfg *config.Config) (*auth.Provider, error) {
	if cfg == nil || cfg.OIDC.Issuer == "" {
		return nil, nil
	}
	return auth.NewProvider(ctx, auth.Config{
		Issuer:       cfg.OIDC.Issuer,
		DiscoveryURL: cfg.OIDC.DiscoveryURL,
		Audience:     cfg.OIDC.Audience,
		Claims: auth.ClaimPaths{
			Username:   cfg.OIDC.Claims.Username,
			Name:       cfg.OIDC.Claims.Name,
			Email:      cfg.OIDC.Claims.Email,
			Privileges: cfg.OIDC.Claims.Privileges,
			Scope:      cfg.OIDC.Claims.Scope,
			Assertion:  cfg.OIDC.Claims.Assertion,
		},
	})
}
