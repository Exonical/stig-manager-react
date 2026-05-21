// Package config loads runtime configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config captures runtime configuration for the API server.
type Config struct {
	// HTTPAddr is the listen address for the HTTP server (e.g. ":54001").
	HTTPAddr string
	// DatabaseURL is a libpq / pgx connection string for Postgres 18.
	// Optional during scaffold milestones; required from Milestone 4 onward.
	DatabaseURL string
	// AllowedOrigins is the list of CORS origins permitted for the SPA.
	AllowedOrigins []string
	// LogLevel is one of "debug", "info", "warn", "error".
	LogLevel string

	// OIDC carries the access-token validator configuration. When
	// OIDC.Issuer is empty the API runs in an unauthenticated mode
	// suitable for local development (see internal/auth).
	OIDC OIDCConfig
	// Client carries SPA-facing settings the API serves via /js/Env.js.
	Client ClientConfig
	// RateLimit caps incoming request volume per authenticated user
	// (or per remote IP when anonymous).
	RateLimit RateLimitConfig
}

// RateLimitConfig caps per-client request volume. When Enabled is
// false the middleware is a no-op. Rate and Burst follow standard
// token-bucket semantics: the bucket is replenished at Rate tokens
// per second up to a maximum of Burst tokens.
type RateLimitConfig struct {
	Enabled bool
	Rate    float64
	Burst   int
}

// OIDCConfig configures the API-side JWT validator.
type OIDCConfig struct {
	Issuer   string
	Audience string
	Claims   OIDCClaimPaths
}

// OIDCClaimPaths configures which JWT claim each User field is sourced
// from. Each path is dot-separated.
type OIDCClaimPaths struct {
	Username   string
	Name       string
	Email      string
	Privileges string
	Scope      string
	Assertion  string
}

// ClientConfig contains SPA-facing settings served via /js/Env.js. The
// shape mirrors upstream's STIGMAN.Env so the SPA can self-configure at
// runtime.
type ClientConfig struct {
	APIBase       string
	Authority     string
	ClientID      string
	ExtraScopes   string
	ScopePrefix   string
	AudienceValue string
	ResponseMode  string
	StrictPKCE    bool
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		HTTPAddr:    envOr("STIGMAN_HTTP_ADDR", ":54001"),
		DatabaseURL: os.Getenv("STIGMAN_DATABASE_URL"),
		LogLevel:    envOr("STIGMAN_LOG_LEVEL", "info"),
		OIDC: OIDCConfig{
			Issuer:   firstNonEmpty("STIGMAN_OIDC_PROVIDER", "STIGMAN_OIDC_ISSUER"),
			Audience: os.Getenv("STIGMAN_OIDC_AUDIENCE"),
			Claims: OIDCClaimPaths{
				Username:   envOr("STIGMAN_JWT_USERNAME_CLAIM", "preferred_username"),
				Name:       envOr("STIGMAN_JWT_NAME_CLAIM", "name"),
				Email:      envOr("STIGMAN_JWT_EMAIL_CLAIM", "email"),
				Privileges: envOr("STIGMAN_JWT_PRIVILEGES_CLAIM", "realm_access.roles"),
				Scope:      envOr("STIGMAN_JWT_SCOPE_CLAIM", "scope"),
				Assertion:  envOr("STIGMAN_JWT_ASSERTION_CLAIM", "jti"),
			},
		},
		Client: ClientConfig{
			APIBase:       envOr("STIGMAN_CLIENT_API_BASE", "api"),
			Authority:     firstNonEmpty("STIGMAN_CLIENT_OIDC_PROVIDER", "STIGMAN_OIDC_PROVIDER", "STIGMAN_OIDC_ISSUER"),
			ClientID:      envOr("STIGMAN_CLIENT_ID", "stig-manager"),
			ExtraScopes:   os.Getenv("STIGMAN_CLIENT_EXTRA_SCOPES"),
			ScopePrefix:   os.Getenv("STIGMAN_CLIENT_SCOPE_PREFIX"),
			AudienceValue: os.Getenv("STIGMAN_CLIENT_AUDIENCE_VALUE"),
			ResponseMode:  envOr("STIGMAN_CLIENT_RESPONSE_MODE", "fragment"),
			StrictPKCE:    envBool("STIGMAN_CLIENT_STRICT_PKCE", true),
		},
		RateLimit: RateLimitConfig{
			Enabled: envBool("STIGMAN_RATE_LIMIT_ENABLED", true),
			Rate:    envFloat("STIGMAN_RATE_LIMIT_RPS", 50),
			Burst:   envInt("STIGMAN_RATE_LIMIT_BURST", 100),
		},
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

func firstNonEmpty(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

func envFloat(key string, fallback float64) float64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f <= 0 {
		return fallback
	}
	return f
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func envBool(key string, fallback bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch v {
	case "":
		return fallback
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}
