// Package server wires the HTTP router for the STIG Manager API.
package server

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

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
	r.Use(middleware.Timeout(60_000_000_000)) // 60s

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   opts.Config.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Requested-With"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Get("/health", handlers.Health)

	r.Route("/api/v1", func(api chi.Router) {
		api.Route("/op", func(op chi.Router) {
			op.Get("/appinfo", handlers.AppInfo(handlers.AppInfoBuild{
				Version:   opts.Version,
				Commit:    opts.Commit,
				BuildDate: opts.BuildDate,
			}))
			op.Get("/appdata/tables", handlers.AppDataTables)
		})
	})

	return &Server{opts: opts, router: r}
}

// Router exposes the configured http.Handler.
func (s *Server) Router() http.Handler { return s.router }
