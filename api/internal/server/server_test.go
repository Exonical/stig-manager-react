package server_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/Exonical/stig-manager-react/api/internal/auth"
	"github.com/Exonical/stig-manager-react/api/internal/config"
	"github.com/Exonical/stig-manager-react/api/internal/server"
)

type oidcFixture struct {
	server *httptest.Server
	issuer string
	signer jose.Signer
}

func newOIDCFixture(t *testing.T) *oidcFixture {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa key: %v", err)
	}
	const kid = "test-key-1"
	jwks := jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{
			{Key: priv.Public(), KeyID: kid, Algorithm: string(jose.ES256), Use: "sig"},
		},
	}
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.ES256, Key: priv},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", kid),
	)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	fx := &oidcFixture{signer: signer}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                fx.issuer,
			"authorization_endpoint":                fx.issuer + "/auth",
			"token_endpoint":                        fx.issuer + "/token",
			"jwks_uri":                              fx.issuer + "/certs",
			"id_token_signing_alg_values_supported": []string{"ES256"},
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
		})
	})
	mux.HandleFunc("/certs", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jwks)
	})
	fx.server = httptest.NewServer(mux)
	fx.issuer = fx.server.URL
	t.Cleanup(fx.server.Close)
	return fx
}

func (f *oidcFixture) token(t *testing.T, scope string) string {
	t.Helper()
	tok, err := jwt.Signed(f.signer).Claims(map[string]any{
		"iss":                f.issuer,
		"aud":                "stig-manager",
		"sub":                "user-1",
		"preferred_username": "alice",
		"scope":              scope,
		"exp":                time.Now().Add(5 * time.Minute).Unix(),
		"iat":                time.Now().Unix(),
		"realm_access":       map[string]any{"roles": []string{"user", "admin"}},
	}).Serialize()
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return tok
}

func newTestServer(t *testing.T, opts ...serverOpt) http.Handler {
	t.Helper()
	cfg := &config.Config{
		HTTPAddr:       ":0",
		AllowedOrigins: []string{"*"},
		Client: config.ClientConfig{
			APIBase:      "api",
			ClientID:     "stig-manager",
			ResponseMode: "fragment",
			StrictPKCE:   true,
		},
		OIDC: config.OIDCConfig{
			Claims: config.OIDCClaimPaths{
				Username:   "preferred_username",
				Name:       "name",
				Email:      "email",
				Privileges: "realm_access.roles",
				Scope:      "scope",
				Assertion:  "jti",
			},
		},
	}
	options := server.Options{
		Version:   "1.2.3",
		Commit:    "abc123",
		BuildDate: "2026-05-19",
		Config:    cfg,
	}
	for _, o := range opts {
		o(&options)
	}
	srv := server.New(options)
	return srv.Router()
}

type serverOpt func(*server.Options)

func withAuth(prov *auth.Provider) serverOpt {
	return func(o *server.Options) { o.AuthProvider = prov }
}

func withClientAuthority(authority string) serverOpt {
	return func(o *server.Options) { o.Config.Client.Authority = authority }
}

func TestHealth(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	newTestServer(t).ServeHTTP(rec, req)
	if got := rec.Result().StatusCode; got != http.StatusOK {
		t.Fatalf("status: got %d want %d", got, http.StatusOK)
	}
}

func TestConfiguration_NoAuthRequired(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/op/configuration", nil)

	newTestServer(t).ServeHTTP(rec, req)
	if got := rec.Result().StatusCode; got != http.StatusOK {
		t.Fatalf("status: got %d want %d (body=%s)", got, http.StatusOK, rec.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["version"] != "v1" {
		t.Errorf("version: got %v want v1", body["version"])
	}
	if body["classification"] != "U" {
		t.Errorf("classification: got %v want U", body["classification"])
	}
}

func TestAppInfo_RequiresScope(t *testing.T) {
	t.Parallel()
	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(context.Background(), auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	handler := newTestServer(t, withAuth(prov))

	// No bearer → 401.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/op/appinfo", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-token: got %d want 401 (body=%s)", rec.Code, rec.Body.String())
	}

	// Wrong scope → 403.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/op/appinfo", nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:user:read"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("wrong-scope: got %d want 403 (body=%s)", rec.Code, rec.Body.String())
	}

	// Right scope (write implies read) → 200, payload reflects build.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/op/appinfo", nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:op"))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ok: got %d want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["version"] != "1.2.3" {
		t.Errorf("version: got %v want 1.2.3", body["version"])
	}
}

// TestUnimplementedReturns501 confirms that operations we have not yet
// overridden still fall through to the api.Unimplemented 501 stub.
// /users has only optional query parameters, so it bypasses the
// generated 400 validation layer and exercises Unimplemented directly.
// A valid bearer token is required because the auth middleware gates
// every /api/* operation that declares a security requirement.
func TestUnimplementedReturns501(t *testing.T) {
	t.Parallel()
	fx := newOIDCFixture(t)
	prov, err := auth.NewProvider(context.Background(), auth.Config{
		Issuer: fx.issuer, Audience: "stig-manager",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	handler := newTestServer(t, withAuth(prov))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	req.Header.Set("Authorization", "Bearer "+fx.token(t, "stig-manager:user:read"))
	handler.ServeHTTP(rec, req)
	if got := rec.Result().StatusCode; got != http.StatusNotImplemented {
		t.Fatalf("status: got %d want %d (body=%s)", got, http.StatusNotImplemented, rec.Body.String())
	}
}

func TestEnvJSExposesOIDCSettings(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/js/Env.js", nil)

	newTestServer(t, withClientAuthority("http://localhost:8080/realms/stigman")).
		ServeHTTP(rec, req)

	if got := rec.Result().StatusCode; got != http.StatusOK {
		t.Fatalf("status: got %d want %d", got, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/javascript") {
		t.Errorf("content-type: got %q want javascript", ct)
	}
	body := rec.Body.String()
	// The script must declare `const STIGMAN = {...}` so the SPA can
	// reference `STIGMAN.Env` after the script tag.
	if !strings.HasPrefix(body, "const STIGMAN = ") {
		t.Fatalf("script must start with `const STIGMAN = `; got: %s", body)
	}
	if !strings.Contains(body, `"authority":"http://localhost:8080/realms/stigman"`) {
		t.Errorf("authority not present in body: %s", body)
	}
	if !strings.Contains(body, `"clientId":"stig-manager"`) {
		t.Errorf("clientId not present in body")
	}
	if !strings.Contains(body, `"apiBase":"api"`) {
		t.Errorf("apiBase not present in body")
	}
}
