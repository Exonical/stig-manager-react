// Package audit provides middleware that records mutating HTTP
// requests into the audit_log table for later forensic review. Reads
// (GET/HEAD/OPTIONS) are excluded so the table grows in proportion to
// write traffic only.
package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/Exonical/stig-manager-react/api/internal/auth"
	"github.com/Exonical/stig-manager-react/api/internal/store"
)

// Recorder is the subset of AuditRepo the middleware needs. Defining
// it as an interface here keeps the audit package free of pgx
// dependencies in tests.
type Recorder interface {
	Record(ctx context.Context, e store.AuditEntry) error
}

// Config tunes the middleware. All fields have safe defaults.
type Config struct {
	// Logger receives best-effort warnings when an audit row fails to
	// persist. nil means slog.Default().
	Logger *slog.Logger
	// MaxBodyBytes caps how much of the request body is stored under
	// payload. Bodies larger than this are stored as NULL with
	// `metadata.body.truncated=true`. Zero means 32 KiB.
	MaxBodyBytes int
	// RedactKeys is the additional list of JSON keys to redact in
	// recorded payloads on top of the built-in default set
	// (password, token, secret, authorization, api_key, …).
	RedactKeys []string
}

// Middleware returns chi middleware that records each non-read HTTP
// request to the audit log. When rec is nil the middleware is a
// no-op so tests can construct the server without a database.
func Middleware(rec Recorder, cfg Config) func(http.Handler) http.Handler {
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = 32 * 1024
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	redact := defaultRedactSet()
	for _, k := range cfg.RedactKeys {
		redact[strings.ToLower(k)] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if rec == nil || !isAudited(r) {
				next.ServeHTTP(w, r)
				return
			}
			start := time.Now()

			// Drain the body into a buffer so the downstream handler
			// still sees it. We deliberately cap what we audit;
			// oversized bodies are passed through but not recorded.
			var (
				bodyBuf       []byte
				bodyTruncated bool
			)
			if r.Body != nil {
				buf, _ := io.ReadAll(r.Body)
				r.Body = io.NopCloser(bytes.NewReader(buf))
				if len(buf) > cfg.MaxBodyBytes {
					bodyTruncated = true
				} else {
					bodyBuf = buf
				}
			}

			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			entry := buildEntry(r, ww.Status(), time.Since(start), bodyBuf, bodyTruncated, redact)

			// Recording is best-effort and asynchronous so the
			// response is never blocked on the audit insert.
			go func(e store.AuditEntry) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := rec.Record(ctx, e); err != nil {
					cfg.Logger.Warn("audit: persist failed",
						slog.String("method", e.Method),
						slog.String("path", e.Path),
						slog.Int("status", e.Status),
						slog.String("error", err.Error()))
				}
			}(entry)
		})
	}
}

// isAudited returns true for state-changing methods on /api/* paths.
// /js/Env.js, /health, /api/op/state/sse, and the GET surface are
// skipped to keep the table proportional to mutation volume.
func isAudited(r *http.Request) bool {
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return false
	}
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		return false
	}
	return true
}

func buildEntry(
	r *http.Request, status int, dur time.Duration,
	body []byte, truncated bool, redact map[string]struct{},
) store.AuditEntry {
	e := store.AuditEntry{
		Method:     r.Method,
		Path:       r.URL.Path,
		Status:     status,
		DurationMs: int(dur.Milliseconds()),
		IP:         clientIP(r),
		RequestID:  middleware.GetReqID(r.Context()),
	}
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		e.Route = rctx.RoutePattern()
	}
	if u, ok := auth.FromContext(r.Context()); ok {
		e.Subject = u.Subject
		e.Username = u.Username
	}
	meta := map[string]any{}
	if truncated {
		meta["body"] = map[string]any{"truncated": true, "limit": len(body)}
	}
	if ct := r.Header.Get("Content-Type"); ct != "" {
		meta["contentType"] = ct
	}
	if v, err := json.Marshal(meta); err == nil {
		e.Metadata = v
	}
	if len(body) > 0 {
		if cleaned, ok := redactJSON(body, redact); ok {
			e.Payload = cleaned
		}
	}
	return e
}

// defaultRedactSet returns the set of payload JSON keys whose values
// are always replaced with the literal string "***". The match is
// case-insensitive and applies at every level of the JSON tree.
func defaultRedactSet() map[string]struct{} {
	out := map[string]struct{}{}
	for _, k := range []string{
		"password",
		"secret",
		"token",
		"refreshtoken",
		"refresh_token",
		"clientsecret",
		"client_secret",
		"api_key",
		"apikey",
		"authorization",
	} {
		out[k] = struct{}{}
	}
	return out
}

// redactJSON tries to decode body as JSON, redacts known-sensitive
// keys recursively, and re-encodes. When body is not valid JSON the
// function returns ok=false so the caller skips the payload column
// rather than persisting opaque bytes.
func redactJSON(body []byte, redact map[string]struct{}) (json.RawMessage, bool) {
	if !json.Valid(body) {
		return nil, false
	}
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, false
	}
	cleaned := scrub(v, redact)
	out, err := json.Marshal(cleaned)
	if err != nil {
		return nil, false
	}
	return out, true
}

func scrub(v any, redact map[string]struct{}) any {
	switch t := v.(type) {
	case map[string]any:
		for k, vv := range t {
			if _, hit := redact[strings.ToLower(k)]; hit {
				t[k] = "***"
				continue
			}
			t[k] = scrub(vv, redact)
		}
		return t
	case []any:
		for i := range t {
			t[i] = scrub(t[i], redact)
		}
		return t
	default:
		return v
	}
}

// clientIP returns the best-available client IP for r. chi's RealIP
// will already have rewritten RemoteAddr from X-Forwarded-For when
// the upstream proxy provides it.
func clientIP(r *http.Request) string {
	addr := r.RemoteAddr
	if i := strings.LastIndex(addr, ":"); i > 0 {
		addr = addr[:i]
	}
	addr = strings.Trim(addr, "[]")
	return addr
}
