// Package config loads runtime configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// Config captures runtime configuration for the API server.
type Config struct {
	// HTTPAddr is the listen address for the HTTP server (e.g. ":54001").
	HTTPAddr string
	// DatabaseURL is a libpq / pgx connection string for Postgres 18.
	// Optional during scaffold milestones; required from Milestone 2 onward.
	DatabaseURL string
	// AllowedOrigins is the list of CORS origins permitted for the SPA.
	AllowedOrigins []string
	// LogLevel is one of "debug", "info", "warn", "error".
	LogLevel string
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		HTTPAddr:    envOr("STIGMAN_HTTP_ADDR", ":54001"),
		DatabaseURL: os.Getenv("STIGMAN_DATABASE_URL"),
		LogLevel:    envOr("STIGMAN_LOG_LEVEL", "info"),
	}

	origins := envOr("STIGMAN_ALLOWED_ORIGINS",
		"http://localhost:54000,http://127.0.0.1:54000")
	for _, o := range strings.Split(origins, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			cfg.AllowedOrigins = append(cfg.AllowedOrigins, o)
		}
	}

	if cfg.HTTPAddr == "" {
		return nil, errors.New("STIGMAN_HTTP_ADDR must not be empty")
	}
	switch cfg.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return nil, fmt.Errorf("invalid STIGMAN_LOG_LEVEL %q", cfg.LogLevel)
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
