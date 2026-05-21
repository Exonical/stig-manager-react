package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AppInfoRepo answers the lightweight queries needed by GetAppInfo and
// GetState — table sizes, row counts, schema migration version, etc.
// It deliberately stops short of upstream's MySQL-specific status
// variables; the Postgres equivalents are surfaced where reasonable.
type AppInfoRepo struct {
	pool *pgxpool.Pool
}

// NewAppInfoRepo constructs an AppInfoRepo bound to the given pool.
func NewAppInfoRepo(pool *pgxpool.Pool) *AppInfoRepo { return &AppInfoRepo{pool: pool} }

// Ping verifies the database is reachable. Used by the dependency
// probe.
func (r *AppInfoRepo) Ping(ctx context.Context) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("no pool")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return r.pool.Ping(ctx)
}

// TableStat is the lightweight projection returned by Tables.
type TableStat struct {
	Name       string
	Rows       int64
	DataLength int64
}

// Tables returns size statistics for every public table in the
// database, ordered by name. Uses pg_class.reltuples + pg_relation_size
// — both are estimates but match what an admin UI cares about
// (orders-of-magnitude visibility).
func (r *AppInfoRepo) Tables(ctx context.Context) ([]TableStat, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("no pool")
	}
	const q = `
		SELECT c.relname,
		       COALESCE(c.reltuples, 0)::bigint AS row_count,
		       COALESCE(pg_total_relation_size(c.oid), 0)::bigint AS data_length
		FROM   pg_class c
		JOIN   pg_namespace n ON n.oid = c.relnamespace
		WHERE  n.nspname = 'public'
		  AND  c.relkind IN ('r','p')
		ORDER  BY c.relname`
	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TableStat{}
	for rows.Next() {
		var t TableStat
		if err := rows.Scan(&t.Name, &t.Rows, &t.DataLength); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Postgres surfaces the values a SPA might want to display alongside
// the running build (version + uptime, plus a handful of GUCs).
type Postgres struct {
	Version     string         `json:"version"`
	Uptime      time.Duration  `json:"uptime"`
	Variables   map[string]any `json:"variables"`
	StartTime   time.Time      `json:"startTime"`
	Connections int            `json:"connections"`
}

// PostgresStatus returns the server version, uptime and a small set of
// useful GUCs. Errors are surfaced; the caller can fall back to a
// zero value when the DB is not reachable.
func (r *AppInfoRepo) PostgresStatus(ctx context.Context) (Postgres, error) {
	if r == nil || r.pool == nil {
		return Postgres{}, fmt.Errorf("no pool")
	}
	var p Postgres
	row := r.pool.QueryRow(ctx, `
		SELECT current_setting('server_version'),
		       pg_postmaster_start_time(),
		       NOW() - pg_postmaster_start_time(),
		       (SELECT COUNT(*) FROM pg_stat_activity)`)
	if err := row.Scan(&p.Version, &p.StartTime, &p.Uptime, &p.Connections); err != nil {
		return Postgres{}, err
	}
	gucs := []string{"server_version", "max_connections", "shared_buffers", "effective_cache_size", "timezone"}
	p.Variables = map[string]any{}
	for _, k := range gucs {
		var v string
		if err := r.pool.QueryRow(ctx, "SELECT current_setting($1)", k).Scan(&v); err != nil {
			continue
		}
		p.Variables[k] = v
	}
	return p, nil
}

// Counts summarises the row counts the AppInfo / dashboard surfaces
// rely on. Cheap-ish to compute on demand because all tables are small
// in v1 deployments; if this becomes a bottleneck we can move it
// behind a materialised view.
type Counts struct {
	Users       int64
	UserGroups  int64
	Collections int64
	Assets      int64
	Stigs       int64
	Reviews     int64
	Jobs        int64
	Runs        int64
}

// Counts returns the summary in a single round-trip via a UNION ALL.
func (r *AppInfoRepo) Counts(ctx context.Context) (Counts, error) {
	if r == nil || r.pool == nil {
		return Counts{}, fmt.Errorf("no pool")
	}
	const q = `
		SELECT
		  (SELECT count(*) FROM app_user),
		  (SELECT count(*) FROM user_group),
		  (SELECT count(*) FROM collection),
		  (SELECT count(*) FROM asset),
		  (SELECT count(*) FROM stig),
		  (SELECT count(*) FROM review),
		  (SELECT count(*) FROM job),
		  (SELECT count(*) FROM job_run)`
	var c Counts
	if err := r.pool.QueryRow(ctx, q).Scan(
		&c.Users, &c.UserGroups, &c.Collections, &c.Assets, &c.Stigs, &c.Reviews, &c.Jobs, &c.Runs,
	); err != nil {
		return Counts{}, err
	}
	return c, nil
}

// SchemaVersion returns the most recent goose migration version
// applied to the database. Used by GetAppInfo + GetConfiguration.
func (r *AppInfoRepo) SchemaVersion(ctx context.Context) (int64, error) {
	if r == nil || r.pool == nil {
		return 0, fmt.Errorf("no pool")
	}
	var v int64
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(version_id), 0)
		FROM   goose_db_version
		WHERE  is_applied = TRUE`).Scan(&v)
	return v, err
}
