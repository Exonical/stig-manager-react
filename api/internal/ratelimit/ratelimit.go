// Package ratelimit provides an in-process token-bucket rate limiter
// keyed by either the authenticated user or the remote IP. The
// limiter is suitable for single-instance deployments; horizontally
// scaled rollouts should be paired with an external store (e.g.
// Redis) to share the bucket — that work is deferred.
package ratelimit

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/Exonical/stig-manager-react/api/internal/auth"
)

// Config tunes the per-client token bucket.
type Config struct {
	// RequestsPerSecond is the steady-state refill rate.
	RequestsPerSecond float64
	// Burst is the maximum number of tokens the bucket can hold.
	Burst int
	// Enabled toggles enforcement. When false the middleware is a
	// no-op (useful for development and tests).
	Enabled bool
	// IdleTTL is how long a per-key bucket is kept after its last
	// observed request. Zero means use the default of 10 minutes.
	IdleTTL time.Duration
}

// Limiter is a sync.Map of per-key token buckets. Stale entries are
// reaped lazily on access; a background goroutine is intentionally
// avoided so the limiter is cheap to construct in tests.
type Limiter struct {
	cfg     Config
	buckets sync.Map // map[string]*entry

	now func() time.Time
}

type entry struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

// New returns a Limiter ready for use. Sensible defaults are filled
// in for any field left at the zero value.
func New(cfg Config) *Limiter {
	if cfg.RequestsPerSecond <= 0 {
		cfg.RequestsPerSecond = 50
	}
	if cfg.Burst <= 0 {
		cfg.Burst = 100
	}
	if cfg.IdleTTL <= 0 {
		cfg.IdleTTL = 10 * time.Minute
	}
	return &Limiter{cfg: cfg, now: time.Now}
}

// Allow consumes a token for key and returns true if the request may
// proceed. retryAfter is set to the wait the caller must observe
// before retrying when allow is false.
func (l *Limiter) Allow(key string) (allow bool, retryAfter time.Duration) {
	if !l.cfg.Enabled {
		return true, 0
	}
	now := l.now()
	v, _ := l.buckets.LoadOrStore(key, &entry{
		lim:      rate.NewLimiter(rate.Limit(l.cfg.RequestsPerSecond), l.cfg.Burst),
		lastSeen: now,
	})
	e := v.(*entry)
	if !e.lim.AllowN(now, 1) {
		// rate.Limiter doesn't expose its delay directly; estimate
		// the wait from the configured rate. A token is replenished
		// every 1/rps seconds.
		return false, time.Duration(float64(time.Second) / l.cfg.RequestsPerSecond)
	}
	e.lastSeen = now
	return true, 0
}

// Sweep removes per-key buckets that have not been observed within
// cfg.IdleTTL. Call this periodically (e.g. from a ticker) to keep
// the map from growing unboundedly.
func (l *Limiter) Sweep() {
	cutoff := l.now().Add(-l.cfg.IdleTTL)
	l.buckets.Range(func(k, v any) bool {
		if e, ok := v.(*entry); ok && e.lastSeen.Before(cutoff) {
			l.buckets.Delete(k)
		}
		return true
	})
}

// Middleware returns an http.Handler middleware that enforces the
// limiter. Skipped paths bypass the limiter entirely (e.g. the SSE
// stream and the health probe). Authenticated requests are keyed by
// the user's Subject claim; anonymous requests by the remote IP.
func (l *Limiter) Middleware(skipPaths ...string) func(http.Handler) http.Handler {
	skip := make(map[string]struct{}, len(skipPaths))
	for _, p := range skipPaths {
		skip[p] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !l.cfg.Enabled {
				next.ServeHTTP(w, r)
				return
			}
			if _, ok := skip[r.URL.Path]; ok {
				next.ServeHTTP(w, r)
				return
			}
			key := keyFor(r)
			allow, retry := l.Allow(key)
			if allow {
				next.ServeHTTP(w, r)
				return
			}
			writeTooMany(w, retry)
		})
	}
}

// keyFor derives the bucket key for r. Authenticated users get keyed
// by their Subject claim so the limiter follows them across IPs;
// anonymous requests get keyed by client IP.
func keyFor(r *http.Request) string {
	if u, ok := auth.FromContext(r.Context()); ok && u.Subject != "" {
		return "u:" + u.Subject
	}
	return "ip:" + clientIP(r)
}

// clientIP picks the best-available client IP for the request.
// chi's RealIP middleware will have already rewritten r.RemoteAddr
// from X-Forwarded-For / X-Real-IP when those are present.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr may already be just an IP (e.g. in tests).
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}

// writeTooMany emits the standard 429 response with a Retry-After
// header (rounded up to whole seconds, minimum 1) and a small JSON
// body matching the API's error envelope.
func writeTooMany(w http.ResponseWriter, retry time.Duration) {
	secs := int(retry.Seconds())
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":  "Too Many Requests",
		"detail": "rate limit exceeded; retry after " + strconv.Itoa(secs) + "s",
	})
}
