// Package server wires the HTTP router for the STIG Manager API.
package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/Exonical/stig-manager-react/api/internal/api"
	"github.com/Exonical/stig-manager-react/api/internal/config"
	"github.com/Exonical/stig-manager-react/api/internal/handlers"
)

// Options describes inputs needed to construct a Server.
type Options struct {
	Logger    *slog.Logger
	Version   string
	Commit    string
	BuildDate string
	Config    *config.Config
}

// Server holds the HTTP router and its dependencies.
type Server struct {
	opts   Options
	router http.Handler
}

// New constructs a Server with the given options.
func New(opts Options) *Server {
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

	// Liveness probe lives outside the OpenAPI surface so orchestrators
	// can hit it without a token. The generated router handles everything
	// under /api.
	r.Get("/health", handlers.Health)

	apiServer := APIServer{
		Build: AppInfoBuild{
			Version:   opts.Version,
			Commit:    opts.Commit,
			BuildDate: opts.BuildDate,
		},
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
