package server

import (
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

// RequestCounter is a thread-safe accumulator of per-route request
// statistics. Counts feed into GetAppInfo's "requests" section.
type RequestCounter struct {
	mu           sync.RWMutex
	totalReq     int64
	totalDur     int64
	totalAPIReq  int64
	totalErrors  int64
	perOperation map[string]*operationStats
}

type operationStats struct {
	TotalRequests int64
	TotalDuration int64 // milliseconds, sum
	MinDuration   int64
	MaxDuration   int64
	Errors        map[string]int64
}

// NewRequestCounter constructs an empty counter ready to be wired as
// chi middleware.
func NewRequestCounter() *RequestCounter {
	return &RequestCounter{
		perOperation: map[string]*operationStats{},
	}
}

// Middleware returns a chi middleware that records a sample for every
// request that reaches a generated handler. Counter increments happen
// after the response is written so duration includes the full handler.
func (rc *RequestCounter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusWriter{ResponseWriter: w, code: 200}
		next.ServeHTTP(rw, r)
		rc.observe(r, rw.code, time.Since(start))
	})
}

func (rc *RequestCounter) observe(r *http.Request, status int, dur time.Duration) {
	durMs := dur.Milliseconds()
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.totalReq++
	rc.totalDur += durMs
	// Treat anything under /api as a counted API request (upstream's
	// definition).
	if len(r.URL.Path) >= 4 && r.URL.Path[:4] == "/api" {
		rc.totalAPIReq++
	}
	if status >= 400 {
		rc.totalErrors++
	}
	op := operationKey(r)
	if op == "" {
		return
	}
	s, ok := rc.perOperation[op]
	if !ok {
		s = &operationStats{MinDuration: durMs, Errors: map[string]int64{}}
		rc.perOperation[op] = s
	}
	s.TotalRequests++
	s.TotalDuration += durMs
	if durMs < s.MinDuration || s.MinDuration == 0 {
		s.MinDuration = durMs
	}
	if durMs > s.MaxDuration {
		s.MaxDuration = durMs
	}
	if status >= 400 {
		key := http.StatusText(status)
		if key == "" {
			key = "Unknown"
		}
		s.Errors[key]++
	}
}

// Snapshot is the read-only projection of the counter consumed by
// GetAppInfo.
type RequestSnapshot struct {
	TotalRequests    int64
	TotalAPIRequests int64
	TotalDuration    int64
	TotalErrors      int64
	Operations       map[string]OperationSnapshot
}

// OperationSnapshot mirrors the per-operation stats with copies of the
// internal maps so callers cannot mutate counter state by accident.
type OperationSnapshot struct {
	TotalRequests int64
	TotalDuration int64
	MinDuration   int64
	MaxDuration   int64
	Errors        map[string]int64
}

// Snapshot returns a deep copy of the current counter state.
func (rc *RequestCounter) Snapshot() RequestSnapshot {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	out := RequestSnapshot{
		TotalRequests:    rc.totalReq,
		TotalAPIRequests: rc.totalAPIReq,
		TotalDuration:    rc.totalDur,
		TotalErrors:      rc.totalErrors,
		Operations:       make(map[string]OperationSnapshot, len(rc.perOperation)),
	}
	for k, v := range rc.perOperation {
		errs := make(map[string]int64, len(v.Errors))
		for ek, ev := range v.Errors {
			errs[ek] = ev
		}
		out.Operations[k] = OperationSnapshot{
			TotalRequests: v.TotalRequests,
			TotalDuration: v.TotalDuration,
			MinDuration:   v.MinDuration,
			MaxDuration:   v.MaxDuration,
			Errors:        errs,
		}
	}
	return out
}

// operationKey returns a stable identifier for a request based on the
// matched route pattern (e.g. "GET /api/op/appinfo"). Falls back to
// the raw path when the route context is unavailable.
func operationKey(r *http.Request) string {
	ctx := chi.RouteContext(r.Context())
	if ctx != nil && ctx.RoutePattern() != "" {
		return r.Method + " " + ctx.RoutePattern()
	}
	return r.Method + " " + r.URL.Path
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (sw *statusWriter) WriteHeader(c int) {
	sw.code = c
	sw.ResponseWriter.WriteHeader(c)
}

// Flush makes statusWriter compatible with http.Flusher so SSE
// handlers downstream of the counter can still flush.
func (sw *statusWriter) Flush() {
	if f, ok := sw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
