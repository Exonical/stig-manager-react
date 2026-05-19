package auth_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/Exonical/stig-manager-react/api/internal/auth"
)

// issuerFixture spins up an httptest server that imitates the OIDC
// discovery + JWKS endpoints Keycloak exposes, signs tokens with an
// ECDSA key, and returns the URL of the issuer.
type issuerFixture struct {
	server   *httptest.Server
	issuer   string
	keyID    string
	signer   jose.Signer
	signKey  *ecdsa.PrivateKey
	verifyJWKS jose.JSONWebKeySet
}

func newIssuerFixture(t *testing.T) *issuerFixture {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa key: %v", err)
	}

	const keyID = "test-key-1"
	jwks := jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{
			{
				Key:       priv.Public(),
				KeyID:     keyID,
				Algorithm: string(jose.ES256),
				Use:       "sig",
			},
		},
	}

	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.ES256, Key: priv},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", keyID),
	)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}

	fx := &issuerFixture{
		keyID:      keyID,
		signer:     signer,
		signKey:    priv,
		verifyJWKS: jwks,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                fx.issuer,
			"authorization_endpoint":                fx.issuer + "/protocol/openid-connect/auth",
			"token_endpoint":                        fx.issuer + "/protocol/openid-connect/token",
			"jwks_uri":                              fx.issuer + "/protocol/openid-connect/certs",
			"id_token_signing_alg_values_supported": []string{"ES256"},
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
		})
	})
	mux.HandleFunc("/protocol/openid-connect/certs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(fx.verifyJWKS)
	})

	fx.server = httptest.NewServer(mux)
	fx.issuer = fx.server.URL
	t.Cleanup(fx.server.Close)
	return fx
}

// sign returns a signed JWT with the given claim map. The "iss" and
// "exp" claims are auto-filled if missing.
func (f *issuerFixture) sign(t *testing.T, claims map[string]any) string {
	t.Helper()
	if _, ok := claims["iss"]; !ok {
		claims["iss"] = f.issuer
	}
	if _, ok := claims["exp"]; !ok {
		claims["exp"] = time.Now().Add(5 * time.Minute).Unix()
	}
	if _, ok := claims["iat"]; !ok {
		claims["iat"] = time.Now().Unix()
	}
	tok, err := jwt.Signed(f.signer).Claims(claims).Serialize()
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return tok
}

func TestVerify_HappyPath(t *testing.T) {
	t.Parallel()
	fx := newIssuerFixture(t)
	prov, err := auth.NewProvider(context.Background(), auth.Config{
		Issuer:   fx.issuer,
		Audience: "stig-manager",
		Claims:   auth.DefaultClaimPaths(),
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}

	tok := fx.sign(t, map[string]any{
		"aud":                "stig-manager",
		"sub":                "user-1",
		"preferred_username": "alice",
		"name":               "Alice Example",
		"email":              "alice@example.invalid",
		"scope":              "stig-manager:op:read stig-manager:collection",
		"realm_access": map[string]any{
			"roles": []string{"user", "admin"},
		},
		"jti": "tok-abc",
	})

	user, err := prov.Verify(context.Background(), tok)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if user.Username != "alice" {
		t.Errorf("username: got %q want alice", user.Username)
	}
	if user.Name != "Alice Example" {
		t.Errorf("name: got %q want %q", user.Name, "Alice Example")
	}
	if user.Email != "alice@example.invalid" {
		t.Errorf("email: got %q", user.Email)
	}
	if !user.HasPrivilege("admin") {
		t.Errorf("privileges: got %v want admin present", user.Privileges)
	}
	if user.Assertion != "tok-abc" {
		t.Errorf("assertion: got %q", user.Assertion)
	}
	wantScopes := []string{"stig-manager:op:read", "stig-manager:collection"}
	if !equalScopes(user.Scopes, wantScopes) {
		t.Errorf("scopes: got %v want %v", user.Scopes, wantScopes)
	}
}

func TestVerify_Failures(t *testing.T) {
	t.Parallel()
	fx := newIssuerFixture(t)
	prov, err := auth.NewProvider(context.Background(), auth.Config{
		Issuer:   fx.issuer,
		Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}

	cases := []struct {
		name   string
		claims map[string]any
		want   string // substring expected in error
	}{
		{
			name: "wrong audience",
			claims: map[string]any{
				"aud":                "other-client",
				"sub":                "u",
				"preferred_username": "alice",
			},
			want: "audience",
		},
		{
			name: "expired",
			claims: map[string]any{
				"aud":                "stig-manager",
				"sub":                "u",
				"preferred_username": "alice",
				"exp":                time.Now().Add(-5 * time.Minute).Unix(),
			},
			want: "expired",
		},
		{
			name: "wrong issuer",
			claims: map[string]any{
				"iss":                "http://attacker.example",
				"aud":                "stig-manager",
				"sub":                "u",
				"preferred_username": "alice",
			},
			// go-oidc surfaces this as "id token issued by a different
			// provider"; substring-match the discriminating word.
			want: "different provider",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tok := fx.sign(t, tc.claims)
			_, err := prov.Verify(context.Background(), tok)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.want)
			}
			if !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestVerify_NoToken(t *testing.T) {
	t.Parallel()
	fx := newIssuerFixture(t)
	prov, err := auth.NewProvider(context.Background(), auth.Config{
		Issuer:   fx.issuer,
		Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	if _, err := prov.Verify(context.Background(), ""); err == nil {
		t.Fatal("expected ErrNoToken")
	}
}

func TestHasScope_Hierarchy(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		have []string
		need string
		want bool
	}{
		{"exact write", []string{"stig-manager:stig"}, "stig-manager:stig", true},
		{"exact read", []string{"stig-manager:stig:read"}, "stig-manager:stig:read", true},
		{"write implies read", []string{"stig-manager:stig"}, "stig-manager:stig:read", true},
		{"read does NOT imply write", []string{"stig-manager:stig:read"}, "stig-manager:stig", false},
		{"unrelated scope", []string{"stig-manager:user:read"}, "stig-manager:stig:read", false},
		{"empty have", nil, "stig-manager:stig", false},
		{"empty required", []string{"stig-manager:stig"}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := &auth.User{Scopes: tc.have}
			got := u.HasScope(tc.need)
			if got != tc.want {
				t.Errorf("HasScope(%q) with %v = %v, want %v", tc.need, tc.have, got, tc.want)
			}
		})
	}
}

func TestMiddleware_NoTokenStillReaches(t *testing.T) {
	t.Parallel()
	fx := newIssuerFixture(t)
	prov, err := auth.NewProvider(context.Background(), auth.Config{Issuer: fx.issuer, Audience: "stig-manager"})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	called := false
	h := auth.Middleware(prov, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if _, ok := auth.FromContext(r.Context()); ok {
			t.Errorf("user unexpectedly present on context")
		}
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/op/configuration", nil)
	h.ServeHTTP(rec, req)
	if !called {
		t.Fatal("downstream handler was not reached")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rec.Code)
	}
}

func TestMiddleware_AttachesUser(t *testing.T) {
	t.Parallel()
	fx := newIssuerFixture(t)
	prov, err := auth.NewProvider(context.Background(), auth.Config{Issuer: fx.issuer, Audience: "stig-manager"})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	tok := fx.sign(t, map[string]any{
		"aud":                "stig-manager",
		"sub":                "u",
		"preferred_username": "alice",
		"scope":              "stig-manager:op:read",
	})
	var seen *auth.User
	h := auth.Middleware(prov, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.FromContext(r.Context())
		seen = u
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/op/appinfo", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	h.ServeHTTP(rec, req)
	if seen == nil {
		t.Fatal("user not attached to context")
	}
	if seen.Username != "alice" {
		t.Fatalf("username: got %q want alice", seen.Username)
	}
}

func TestMiddleware_NilProviderIsNoop(t *testing.T) {
	t.Parallel()
	called := false
	h := auth.Middleware(nil, nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/op/configuration", nil)
	req.Header.Set("Authorization", "Bearer ignored")
	h.ServeHTTP(rec, req)
	if !called {
		t.Fatal("downstream not reached with nil provider")
	}
}

func TestRequireUser(t *testing.T) {
	t.Parallel()
	guard := auth.RequireUser(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Without a user.
	rec := httptest.NewRecorder()
	guard.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/op/appinfo", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want 401", rec.Code)
	}

	// With a user.
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/op/appinfo", nil)
	req = req.WithContext(auth.WithUser(req.Context(), &auth.User{Username: "alice"}))
	guard.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rec.Code)
	}
}

func TestRequireScope(t *testing.T) {
	t.Parallel()
	guard := auth.RequireScope("stig-manager:op:read")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// No user → 401.
	rec := httptest.NewRecorder()
	guard.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/op/appinfo", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no user: got %d want 401", rec.Code)
	}

	// User without scope → 403.
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/op/appinfo", nil)
	req = req.WithContext(auth.WithUser(req.Context(), &auth.User{Username: "alice", Scopes: []string{"stig-manager:user"}}))
	guard.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("wrong scope: got %d want 403", rec.Code)
	}

	// User with broader (write) scope → 200 (hierarchy).
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/op/appinfo", nil)
	req = req.WithContext(auth.WithUser(req.Context(), &auth.User{Username: "alice", Scopes: []string{"stig-manager:op"}}))
	guard.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("hierarchical scope: got %d want 200 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestPrivilegesParsesArray(t *testing.T) {
	t.Parallel()
	fx := newIssuerFixture(t)
	prov, err := auth.NewProvider(context.Background(), auth.Config{Issuer: fx.issuer, Audience: "stig-manager"})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	tok := fx.sign(t, map[string]any{
		"aud":                "stig-manager",
		"sub":                "u",
		"preferred_username": "alice",
		"realm_access": map[string]any{
			"roles": []any{"user", "admin", "create_collection"},
		},
	})
	user, err := prov.Verify(context.Background(), tok)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !user.HasPrivilege("admin") || !user.HasPrivilege("user") || !user.HasPrivilege("create_collection") {
		t.Fatalf("privileges: %v", user.Privileges)
	}
}

func equalScopes(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]bool, len(a))
	for _, s := range a {
		m[s] = true
	}
	for _, s := range b {
		if !m[s] {
			return false
		}
	}
	return true
}

// dummy import-keeper so 'fmt' isn't elided after edits.
var _ = fmt.Sprintf
