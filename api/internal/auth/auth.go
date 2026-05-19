// Package auth implements OIDC access-token validation, request-scoped
// user extraction, and per-route authorization helpers for the STIG
// Manager API.
//
// The shape of User and the configurable JWT claim paths match upstream
// STIG Manager (NUWCDIVNPT/stig-manager) so deployments can keep using
// the same Keycloak / Okta / Entra ID setup. See
// https://stig-manager.readthedocs.io/en/latest/admin-guide/oauth/.
package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

// User describes the authenticated principal extracted from a validated
// access token. Claim paths are configurable per deployment; the fields
// here reflect upstream's canonical names.
type User struct {
	// Subject is the OIDC `sub` claim — a stable, opaque identifier.
	Subject string
	// Username is sourced from the configured username claim (default
	// `preferred_username`).
	Username string
	// Name is the display name (default claim `name`).
	Name string
	// Email is the user's email address (default claim `email`).
	Email string
	// Privileges is the list of role strings sourced from the configured
	// privileges claim (default `realm_access.roles`). Upstream uses
	// privileges such as "admin", "create_collection", "user".
	Privileges []string
	// Scopes is the list of OAuth scopes sourced from the configured
	// scope claim (default `scope`, a space-separated string). Scopes
	// gate endpoint access and follow upstream's hierarchical match
	// semantics (see HasScope).
	Scopes []string
	// Assertion is a per-token identifier used to detect replay (default
	// claim `jti`).
	Assertion string
	// Raw is the full claim set, useful for one-off lookups.
	Raw map[string]any
}

// HasPrivilege returns true if the user has the named privilege.
func (u *User) HasPrivilege(p string) bool {
	if u == nil {
		return false
	}
	for _, have := range u.Privileges {
		if have == p {
			return true
		}
	}
	return false
}

// HasScope reports whether the user holds a scope that satisfies the
// requested one, using upstream's hierarchical semantics:
//
//   - An exact match satisfies the requirement.
//   - A "write" scope (no `:read` suffix) implies the matching `:read`
//     scope: holding `stig-manager:collection` satisfies a requirement
//     of `stig-manager:collection:read`.
//   - A broader prefix DOES NOT imply a narrower scope on its own; the
//     hierarchy is exactly one level deep (resource → resource:read).
//
// Examples:
//
//	user has               required                       result
//	stig-manager:stig      stig-manager:stig              true
//	stig-manager:stig      stig-manager:stig:read         true
//	stig-manager:stig:read stig-manager:stig              false
//	stig-manager:stig:read stig-manager:stig:read         true
func (u *User) HasScope(required string) bool {
	if u == nil || required == "" {
		return false
	}
	readSuffix := ":read"
	requiredRead := strings.HasSuffix(required, readSuffix)
	requiredBase := strings.TrimSuffix(required, readSuffix)
	for _, have := range u.Scopes {
		if have == required {
			return true
		}
		// A write-level scope satisfies a :read requirement.
		if requiredRead && have == requiredBase {
			return true
		}
	}
	return false
}

type contextKey int

const userContextKey contextKey = iota

// WithUser returns ctx with u attached.
func WithUser(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, userContextKey, u)
}

// FromContext returns the user attached to ctx by Middleware, if any.
func FromContext(ctx context.Context) (*User, bool) {
	u, ok := ctx.Value(userContextKey).(*User)
	return u, ok && u != nil
}

// MustFromContext panics if no user is present. Use only after
// RequireUser has run.
func MustFromContext(ctx context.Context) *User {
	u, ok := FromContext(ctx)
	if !ok {
		panic("auth: user missing from context (did RequireUser run?)")
	}
	return u
}

// ErrNoToken is returned by extractBearerToken when no Authorization
// header is present.
var ErrNoToken = errors.New("auth: no bearer token")

// extractBearerToken pulls the access token from the Authorization
// header. Returns ErrNoToken if the header is absent, or an error if
// it is malformed.
func extractBearerToken(r *http.Request) (string, error) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", ErrNoToken
	}
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", errors.New("auth: malformed Authorization header")
	}
	return strings.TrimSpace(h[len(prefix):]), nil
}
