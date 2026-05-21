package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/Exonical/stig-manager-react/api/internal/store"
)

// auditLogResponse is the read shape of /api/op/audit-log. Times are
// emitted as RFC3339 strings so SPA clients can consume them with
// `new Date()` without extra parsing.
type auditLogResponse struct {
	AuditID    int64           `json:"auditId"`
	TS         string          `json:"ts"`
	Method     string          `json:"method"`
	Path       string          `json:"path"`
	Route      string          `json:"route,omitempty"`
	Status     int             `json:"status"`
	DurationMs int             `json:"durationMs"`
	UserID     *int64          `json:"userId,omitempty"`
	Username   string          `json:"username,omitempty"`
	Subject    string          `json:"subject,omitempty"`
	IP         string          `json:"ip,omitempty"`
	RequestID  string          `json:"requestId,omitempty"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
}

// GetAuditLog handles GET /api/op/audit-log. Gated behind the
// stig-manager:op:read scope so the same admin tooling that consumes
// /op/appinfo and /op/state can query the audit log. Query params:
//
//   - limit         (1..1000, default 100)
//   - method        (POST | PUT | PATCH | DELETE)
//   - path          (substring match, case-insensitive)
//   - userId        (numeric app_user.user_id)
//   - since / until (RFC3339 timestamps)
//
// When no audit repo is wired (Pool was nil at boot) the endpoint
// returns 503 so clients can detect the unconfigured state.
func (s APIServer) GetAuditLog(w http.ResponseWriter, r *http.Request) {
	if !s.requireOpRead(w, r) {
		return
	}
	if s.Audit == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error":  "audit log unavailable",
			"detail": "audit repository not configured; the API was started without a database",
		})
		return
	}

	q := r.URL.Query()
	opt := store.ListAuditOptions{
		Method:       q.Get("method"),
		PathContains: q.Get("path"),
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			opt.Limit = n
		}
	}
	if v := q.Get("userId"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			opt.UserID = &id
		}
	}
	for _, p := range [2]struct {
		name string
		dst  *time.Time
	}{{"since", &opt.Since}, {"until", &opt.Until}} {
		if v := q.Get(p.name); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{
					"error":  "invalid timestamp",
					"detail": p.name + " must be RFC3339",
				})
				return
			}
			*p.dst = t
		}
	}

	rows, err := s.Audit.List(r.Context(), opt)
	if err != nil {
		s.logErr(r, "audit list", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": "internal error",
		})
		return
	}
	out := make([]auditLogResponse, 0, len(rows))
	for _, e := range rows {
		out = append(out, auditLogResponse{
			AuditID:    e.AuditID,
			TS:         e.TS.UTC().Format(time.RFC3339Nano),
			Method:     e.Method,
			Path:       e.Path,
			Route:      e.Route,
			Status:     e.Status,
			DurationMs: e.DurationMs,
			UserID:     e.UserID,
			Username:   e.Username,
			Subject:    e.Subject,
			IP:         e.IP,
			RequestID:  e.RequestID,
			Payload:    e.Payload,
			Metadata:   e.Metadata,
		})
	}
	writeJSON(w, http.StatusOK, out)
}


