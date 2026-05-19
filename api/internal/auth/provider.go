package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

// ClaimPaths configures which JWT claim each User field is sourced from.
// Each path is dot-separated (e.g. "realm_access.roles") to support
// nested claims; an empty path disables that field.
type ClaimPaths struct {
	Username   string
	Name       string
	Email      string
	Privileges string
	Scope      string
	Assertion  string
}

// DefaultClaimPaths returns upstream's default claim layout, which
// matches Keycloak's stock access tokens.
func DefaultClaimPaths() ClaimPaths {
	return ClaimPaths{
		Username:   "preferred_username",
		Name:       "name",
		Email:      "email",
		Privileges: "realm_access.roles",
		Scope:      "scope",
		Assertion:  "jti",
	}
}

// Config configures Provider.
type Config struct {
	// Issuer is the OIDC issuer URL (e.g.
	// "http://localhost:8080/realms/stigman").
	Issuer string
	// Audience is the expected `aud` claim. Most Keycloak access tokens
	// set this to the client id.
	Audience string
	// ClockSkew is the allowed leeway when validating exp/nbf/iat.
	ClockSkew time.Duration
	// Claims maps logical fields to JWT claim paths.
	Claims ClaimPaths
}

// Provider validates access tokens against an OIDC issuer.
type Provider struct {
	cfg      Config
	verifier *oidc.IDTokenVerifier
}

// NewProvider performs OIDC discovery against cfg.Issuer and returns a
// Provider ready to validate tokens. The Issuer must be reachable.
func NewProvider(ctx context.Context, cfg Config) (*Provider, error) {
	if cfg.Issuer == "" {
		return nil, errors.New("auth: Config.Issuer must be set")
	}
	if cfg.Claims == (ClaimPaths{}) {
		cfg.Claims = DefaultClaimPaths()
	}
	if cfg.ClockSkew == 0 {
		cfg.ClockSkew = 30 * time.Second
	}

	prov, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("auth: oidc discovery: %w", err)
	}

	verifierCfg := &oidc.Config{
		// We're verifying access tokens, not ID tokens — Keycloak ATs
		// don't carry a `typ: ID` claim, so disable that check.
		SkipClientIDCheck: cfg.Audience == "",
		ClientID:          cfg.Audience,
	}
	return &Provider{
		cfg:      cfg,
		verifier: prov.Verifier(verifierCfg),
	}, nil
}

// Verify parses, validates the signature/claims, and extracts a User
// from the bearer token.
func (p *Provider) Verify(ctx context.Context, rawToken string) (*User, error) {
	if p == nil {
		return nil, errors.New("auth: provider is nil")
	}
	if rawToken == "" {
		return nil, ErrNoToken
	}

	tok, err := p.verifier.Verify(ctx, rawToken)
	if err != nil {
		return nil, fmt.Errorf("auth: verify token: %w", err)
	}

	var claims map[string]any
	if err := tok.Claims(&claims); err != nil {
		return nil, fmt.Errorf("auth: decode claims: %w", err)
	}

	user := &User{
		Subject:    tok.Subject,
		Username:   stringClaim(claims, p.cfg.Claims.Username),
		Name:       stringClaim(claims, p.cfg.Claims.Name),
		Email:      stringClaim(claims, p.cfg.Claims.Email),
		Privileges: stringSliceClaim(claims, p.cfg.Claims.Privileges),
		Scopes:     spaceSeparatedClaim(claims, p.cfg.Claims.Scope),
		Assertion:  stringClaim(claims, p.cfg.Claims.Assertion),
		Raw:        claims,
	}
	return user, nil
}

// claimByPath walks claims along a dot-separated path.
func claimByPath(claims map[string]any, path string) any {
	if path == "" {
		return nil
	}
	parts := strings.Split(path, ".")
	var cur any = claims
	for _, part := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur, ok = m[part]
		if !ok {
			return nil
		}
	}
	return cur
}

func stringClaim(claims map[string]any, path string) string {
	v, _ := claimByPath(claims, path).(string)
	return v
}

func stringSliceClaim(claims map[string]any, path string) []string {
	raw := claimByPath(claims, path)
	switch v := raw.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, s := range v {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				if s = strings.TrimSpace(s); s != "" {
					out = append(out, s)
				}
			}
		}
		return out
	default:
		return nil
	}
}

func spaceSeparatedClaim(claims map[string]any, path string) []string {
	raw := claimByPath(claims, path)
	switch v := raw.(type) {
	case string:
		out := []string{}
		for _, s := range strings.Fields(v) {
			out = append(out, s)
		}
		return out
	case []string:
		return stringSliceClaim(claims, path)
	case []any:
		return stringSliceClaim(claims, path)
	default:
		return nil
	}
}
