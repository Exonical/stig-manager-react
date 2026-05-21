// Package server wires the HTTP router for the STIG Manager API.
package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Exonical/stig-manager-react/api/internal/api"
	"github.com/Exonical/stig-manager-react/api/internal/auth"
	"github.com/Exonical/stig-manager-react/api/internal/config"
	"github.com/Exonical/stig-manager-react/api/internal/handlers"
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
}

// Server holds the HTTP router and its dependencies.
type Server struct {
	opts   Options
	router http.Handler
}

// New constructs a Server with the given options.
func New(opts Options) *Server {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   opts.Config.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Requested-With"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Attach the authenticated user (if any) to the request context.
	// Per-handler scope checks gate individual operations; configuration
	// and Env.js stay reachable without a token.
	r.Use(auth.Middleware(opts.AuthProvider, opts.Logger))

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

	apiServer := APIServer{
		Build: AppInfoBuild{
			Version:   opts.Version,
			Commit:    opts.Commit,
			BuildDate: opts.BuildDate,
		},
		Logger:           opts.Logger,
		MigrationVersion: opts.MigrationVersion,
	}
	if opts.Pool != nil {
		apiServer.Users = store.NewUserRepo(opts.Pool)
		apiServer.Collections = store.NewCollectionRepo(opts.Pool)
		apiServer.Stigs = store.NewSTIGRepo(opts.Pool)
		apiServer.Assets = store.NewAssetRepo(opts.Pool)
		apiServer.Labels = store.NewLabelRepo(opts.Pool)
		apiServer.Grants = store.NewGrantRepo(opts.Pool)
	}

	// Register the generated handlers directly onto the root chi router
	// at /api/* so the existing middleware stack (RequestID, CORS, etc.)
	// applies. This matches upstream's URL layout: paths in the OpenAPI
	// spec are relative to /api.
	api.HandlerFromMuxWithBaseURL(apiServer, r, "/api")

	return &Server{opts: opts, router: r}
}

// Router exposes the configured http.Handler.
func (s *Server) Router() http.Handler { return s.router }

// BuildAuthProvider returns a configured *auth.Provider, or nil when
// cfg.OIDC.Issuer is empty (unauthenticated dev mode). Errors are
// returned only when discovery against a configured issuer fails.
func BuildAuthProvider(ctx context.Context, cfg *config.Config) (*auth.Provider, error) {
	if cfg == nil || cfg.OIDC.Issuer == "" {
		return nil, nil
	}
	return auth.NewProvider(ctx, auth.Config{
		Issuer:   cfg.OIDC.Issuer,
		Audience: cfg.OIDC.Audience,
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
