package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Exonical/stig-manager-react/api/internal/config"
)

// EnvScript is the body of the /js/Env.js script the SPA loads at boot
// to discover OIDC settings, API base path, and version info. The shape
// is byte-compatible with upstream's `STIGMAN.Env` so the same SPA
// configuration patterns work.
type EnvScript struct {
	Cfg     *config.Config
	Version string
	Commit  string
}

// ServeHTTP renders STIGMAN.Env as JavaScript. Cache headers are set so
// that browsers re-fetch the file on every page load — this lets ops
// change OIDC settings without forcing a hard refresh.
func (e EnvScript) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, max-age=0")

	env := map[string]any{
		"version": e.Version,
		"apiBase": e.Cfg.Client.APIBase,
		"commit": map[string]any{
			"sha": e.Commit,
		},
		"oauth": map[string]any{
			"authority":     e.Cfg.Client.Authority,
			"clientId":      e.Cfg.Client.ClientID,
			"extraScopes":   e.Cfg.Client.ExtraScopes,
			"scopePrefix":   e.Cfg.Client.ScopePrefix,
			"responseMode":  e.Cfg.Client.ResponseMode,
			"audienceValue": e.Cfg.Client.AudienceValue,
			"strictPkce":    e.Cfg.Client.StrictPKCE,
			"claims": map[string]any{
				"username":   e.Cfg.OIDC.Claims.Username,
				"name":       e.Cfg.OIDC.Claims.Name,
				"email":      e.Cfg.OIDC.Claims.Email,
				"privileges": e.Cfg.OIDC.Claims.Privileges,
				"scope":      e.Cfg.OIDC.Claims.Scope,
				"assertion":  e.Cfg.OIDC.Claims.Assertion,
			},
		},
	}

	// Top-level `let`/`const` declarations are script-local in modern
	// browsers — they do *not* become properties on `window`. The SPA
	// reads `window.STIGMAN.Env`, so we explicitly assign to window
	// here.
	var buf strings.Builder
	buf.WriteString("window.STIGMAN = ")
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]any{"Env": env})
	// json.Encoder writes a trailing newline; replace it with a
	// terminating semicolon for valid JS.
	out := strings.TrimRight(buf.String(), "\n") + ";\n"
	_, _ = w.Write([]byte(out))
}
