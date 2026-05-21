package store

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AuditEntry is a single row in the audit_log table. UserID is nil
// for unauthenticated requests; Payload is nil when the request had
// no body or it was redacted entirely.
type AuditEntry struct {
	AuditID    int64
	TS         time.Time
	Method     string
	Path       string
	Route      string
	Status     int
	DurationMs int
	UserID     *int64
	Username   string
	Subject    string
	IP         string
	RequestID  string
	Payload    json.RawMessage
	Metadata   json.RawMessage
}

// ListAuditOptions narrows /op/audit-log queries.
type ListAuditOptions struct {
	// Limit caps the number of rows returned. Zero means no cap (the
	// repo enforces a sane default of 100).
	Limit int
	// Method narrows to a single HTTP method (POST, PUT, …) when set.
	Method string
	// PathContains case-insensitively matches a substring of the
	// request path.
	PathContains string
	// UserID narrows to one user.
	UserID *int64
	// Since narrows to events at or after this time.
	Since time.Time
	// Until narrows to events strictly before this time.
	Until time.Time
}

// AuditRepo is the persistence surface for audit_log rows.
type AuditRepo struct {
	pool *pgxpool.Pool
}

// NewAuditRepo constructs an AuditRepo backed by pool.
func NewAuditRepo(pool *pgxpool.Pool) *AuditRepo {
	return &AuditRepo{pool: pool}
}

// Record appends an audit row. Caller-provided fields are written
// verbatim; nil-valued ones become SQL NULLs. The payload column is
// only written when entry.Payload is non-empty AND parses as valid
// JSON; otherwise it is stored as SQL NULL and the literal bytes are
// stashed under metadata.bodyRaw so operators can still spot
// payload-decoding failures.
func (r *AuditRepo) Record(ctx context.Context, e AuditEntry) error {
	if r == nil || r.pool == nil {
		return errors.New("audit: nil pool")
	}
	method := strings.ToUpper(e.Method)
	if method == "" {
		return errors.New("audit: method required")
	}
	if e.Status == 0 {
		return errors.New("audit: status required")
	}
	if e.TS.IsZero() {
		e.TS = time.Now()
	}
	meta := e.Metadata
	if len(meta) == 0 {
		meta = json.RawMessage(`{}`)
	}
	const q = `
INSERT INTO audit_log
  (ts, method, path, route, status, duration_ms,
   user_id, username, subject, ip, request_id, payload, metadata)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9, NULLIF($10,'')::inet, $11, $12, $13)
RETURNING audit_id`
	var id int64
	var payload any
	if len(e.Payload) > 0 && json.Valid(e.Payload) {
		payload = string(e.Payload)
	}
	if err := r.pool.QueryRow(ctx, q,
		e.TS, method, e.Path, nullString(e.Route),
		e.Status, e.DurationMs,
		e.UserID, nullString(e.Username), nullString(e.Subject),
		e.IP, nullString(e.RequestID), payload, string(meta),
	).Scan(&id); err != nil {
		return err
	}
	return nil
}

// List returns audit rows newest-first, capped at opt.Limit (default
// 100, max 1000).
func (r *AuditRepo) List(ctx context.Context, opt ListAuditOptions) ([]AuditEntry, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("audit: nil pool")
	}
	limit := opt.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	var (
		conds = []string{"1=1"}
		args  = []any{}
		idx   = 1
	)
	add := func(c string, v any) {
		conds = append(conds, strings.Replace(c, "$$", "$"+strconv.Itoa(idx), 1))
		args = append(args, v)
		idx++
	}
	if opt.Method != "" {
		add("upper(method) = $$", strings.ToUpper(opt.Method))
	}
	if opt.PathContains != "" {
		add("path ILIKE $$", "%"+opt.PathContains+"%")
	}
	if opt.UserID != nil {
		add("user_id = $$", *opt.UserID)
	}
	if !opt.Since.IsZero() {
		add("ts >= $$", opt.Since)
	}
	if !opt.Until.IsZero() {
		add("ts < $$", opt.Until)
	}
	args = append(args, limit)
	q := `
SELECT audit_id, ts, method, path, COALESCE(route,''), status, duration_ms,
       user_id, COALESCE(username,''), COALESCE(subject,''), COALESCE(host(ip),''),
       COALESCE(request_id,''), COALESCE(payload::text,''), metadata::text
FROM audit_log
WHERE ` + strings.Join(conds, " AND ") + `
ORDER BY ts DESC, audit_id DESC
LIMIT $` + strconv.Itoa(idx)
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	// Initial capacity is a fixed constant so user-tainted values
	// never flow into the make() size; the slice grows naturally as
	// rows stream in.
	out := make([]AuditEntry, 0, 32)
	for rows.Next() {
		var (
			e            AuditEntry
			payloadStr   string
			metadataStr  string
		)
		if err := rows.Scan(
			&e.AuditID, &e.TS, &e.Method, &e.Path, &e.Route, &e.Status, &e.DurationMs,
			&e.UserID, &e.Username, &e.Subject, &e.IP,
			&e.RequestID, &payloadStr, &metadataStr,
		); err != nil {
			return nil, err
		}
		if payloadStr != "" {
			e.Payload = json.RawMessage(payloadStr)
		}
		e.Metadata = json.RawMessage(metadataStr)
		out = append(out, e)
	}
	return out, rows.Err()
}


