package auth

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

// Middleware returns a chi-compatible middleware that, when a valid
// bearer token is present, attaches a User to the request context.
//
// It deliberately does NOT 401 on missing/invalid tokens: many routes
// in the OpenAPI spec are declared `security: []` (e.g.
// /api/op/configuration) and need to remain reachable without a token.
// Routes that require a user should chain RequireUser; routes that
// require a specific scope should chain RequireScope.
//
// Invalid tokens are logged at debug level (avoid leaking the token
// payload to info logs) and treated as unauthenticated.
//
// When p is nil (e.g. OIDC isn't configured at startup), Middleware is
// a no-op — every downstream handler sees no User on the context.
func Middleware(p *Provider, logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if p == nil {
				next.ServeHTTP(w, r)
				return
			}
			raw, err := extractBearerToken(r)
			if err != nil {
				if !errors.Is(err, ErrNoToken) {
					logger.Debug("auth: bad authorization header",
						"err", err.Error(),
						"path", r.URL.Path,
					)
				}
				next.ServeHTTP(w, r)
				return
			}
			user, err := p.Verify(r.Context(), raw)
			if err != nil {
				logger.Debug("auth: token rejected",
					"err", err.Error(),
					"path", r.URL.Path,
				)
				next.ServeHTTP(w, r)
				return
			}
			ctx := WithUser(r.Context(), user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireUser is a chi-compatible middleware that 401s when no User is
// present on the request context. Attach it to subtrees that need an
// authenticated principal.
func RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := FromContext(r.Context()); !ok {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireScope returns a chi-compatible middleware that 403s when the
// authenticated user does not hold the required scope. Use it on routes
// the OpenAPI spec gates behind specific scopes.
func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := FromContext(r.Context())
			if !ok {
				writeError(w, http.StatusUnauthorized, "authentication required")
				return
			}
			if !user.HasScope(scope) {
				writeError(w, http.StatusForbidden, "missing required scope: "+scope)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":  http.StatusText(status),
		"detail": msg,
	})
}
