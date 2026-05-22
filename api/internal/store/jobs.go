package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Task is the registry projection of a job_task row.
type Task struct {
	TaskID      int64
	Name        string
	Description *string
	Command     *string
}

// Job is the schedulable unit surfaced by GET /jobs. Event* fields
// describe the trigger ('once' / 'recurring' / manual-only when nil).
type Job struct {
	JobID            int64
	Name             string
	Description      *string
	EventType        *string
	EventID          *string
	EventEnabled     bool
	EventStarts      *time.Time
	EventEnds        *time.Time
	EventIntervalFld *string
	EventIntervalVal *string
	LastEventFire    *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
	CreatedByUserID  *int64
	UpdatedByUserID  *int64
	Tasks            []Task
	RunCount         int
	LastRun          *Run
}

// JobCreate is the input bundle for JobRepo.Create.
type JobCreate struct {
	Name             string
	Description      *string
	EventType        *string // "once" | "recurring" | nil
	EventStarts      *time.Time
	EventEnds        *time.Time
	EventEnabled     *bool // defaults to true
	EventIntervalFld *string
	EventIntervalVal *string
	TaskIDs          []int64
	CreatedByUserID  *int64
}

// JobPatch carries the partial-update payload. Each non-nil field is
// applied; TaskIDs, when non-nil, fully replace the task list.
type JobPatch struct {
	Name             *string
	Description      *string
	EventType        *string
	EventStarts      *time.Time
	EventEnds        *time.Time
	EventEnabled     *bool
	EventIntervalFld *string
	EventIntervalVal *string
	TaskIDs          *[]int64
	UpdatedByUserID  *int64
	// ClearEvent forces NULLing out the event_* columns when true.
	// Used to model PATCH bodies that explicitly remove the event.
	ClearEvent bool
}

// Run is one execution of a job; mirrors JobRun in the OpenAPI schema.
type Run struct {
	RunID     uuid.UUID
	JobID     int64
	State     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// RunOutput is one line of streamed output.
type RunOutput struct {
	Seq     int64
	Ts      time.Time
	TaskID  *int64
	Task    string
	Type    string
	Message string
}

// JobRepo is the data-layer entry point for /jobs and related runs.
type JobRepo struct {
	pool *pgxpool.Pool
}

// NewJobRepo constructs a JobRepo bound to pool.
func NewJobRepo(pool *pgxpool.Pool) *JobRepo {
	return &JobRepo{pool: pool}
}

// ListTasks returns every registered task ordered by name.
func (r *JobRepo) ListTasks(ctx context.Context) ([]Task, error) {
	rows, err := r.pool.Query(ctx, `
SELECT task_id, name, description, command
FROM job_task
ORDER BY lower(name) ASC`)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.TaskID, &t.Name, &t.Description, &t.Command); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// UpsertTask installs/refreshes a row in job_task. Returns the task_id
// resolved by lower(name) uniqueness — safe to call concurrently from
// the in-process registry seed.
func (r *JobRepo) UpsertTask(ctx context.Context, name string, description, command *string) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `
INSERT INTO job_task (name, description, command)
VALUES ($1, $2, $3)
ON CONFLICT ((lower(name))) DO UPDATE
   SET description = EXCLUDED.description,
       command     = EXCLUDED.command,
       updated_at  = now()
RETURNING task_id`, name, description, command).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert task %q: %w", name, err)
	}
	return id, nil
}

// List returns every job, ordered by lower(name). Tasks + RunCount +
// LastRun are populated.
func (r *JobRepo) List(ctx context.Context) ([]Job, error) {
	rows, err := r.pool.Query(ctx, `
SELECT job_id, name, description,
       event_type, event_id, event_enabled, event_starts, event_ends,
       event_interval_field, event_interval_value, last_event_fire,
       created_at, updated_at,
       created_by_user_id, updated_by_user_id
FROM job
ORDER BY lower(name) ASC`)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()
	var jobs []Job
	for rows.Next() {
		j, err := scanJobRow(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range jobs {
		if err := r.hydrateJob(ctx, &jobs[i]); err != nil {
			return nil, err
		}
	}
	return jobs, nil
}

// Get returns a single job by id with its tasks + run summary.
func (r *JobRepo) Get(ctx context.Context, jobID int64) (Job, error) {
	row := r.pool.QueryRow(ctx, `
SELECT job_id, name, description,
       event_type, event_id, event_enabled, event_starts, event_ends,
       event_interval_field, event_interval_value, last_event_fire,
       created_at, updated_at,
       created_by_user_id, updated_by_user_id
FROM job
WHERE job_id = $1`, jobID)
	j, err := scanJobRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Job{}, ErrNotFound
		}
		return Job{}, fmt.Errorf("get job %d: %w", jobID, err)
	}
	if err := r.hydrateJob(ctx, &j); err != nil {
		return Job{}, err
	}
	return j, nil
}

// Create inserts the job + its task list in one transaction.
func (r *JobRepo) Create(ctx context.Context, in JobCreate) (Job, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Job{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	enabled := true
	if in.EventEnabled != nil {
		enabled = *in.EventEnabled
	}
	eventID := newEventID()
	var jobID int64
	err = tx.QueryRow(ctx, `
INSERT INTO job (
    name, description,
    event_type, event_id, event_enabled, event_starts, event_ends,
    event_interval_field, event_interval_value,
    created_by_user_id, updated_by_user_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)
RETURNING job_id`,
		in.Name, in.Description,
		in.EventType, eventID, enabled, in.EventStarts, in.EventEnds,
		in.EventIntervalFld, in.EventIntervalVal,
		in.CreatedByUserID,
	).Scan(&jobID)
	if err != nil {
		return Job{}, fmt.Errorf("insert job: %w", err)
	}
	if err := setJobTasks(ctx, tx, jobID, in.TaskIDs); err != nil {
		return Job{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Job{}, fmt.Errorf("commit: %w", err)
	}
	return r.Get(ctx, jobID)
}

// Patch applies the partial update to jobID. Returns ErrNotFound when
// the job does not exist.
func (r *JobRepo) Patch(ctx context.Context, jobID int64, in JobPatch) (Job, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Job{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	sets := []string{}
	args := []any{}
	idx := 1
	if in.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", idx))
		args = append(args, *in.Name)
		idx++
	}
	if in.Description != nil {
		sets = append(sets, fmt.Sprintf("description = $%d", idx))
		args = append(args, *in.Description)
		idx++
	}
	if in.ClearEvent {
		sets = append(sets,
			"event_type = NULL",
			"event_starts = NULL",
			"event_ends = NULL",
			"event_interval_field = NULL",
			"event_interval_value = NULL",
			"last_event_fire = NULL")
	} else {
		if in.EventType != nil {
			sets = append(sets, fmt.Sprintf("event_type = $%d", idx))
			args = append(args, *in.EventType)
			idx++
			sets = append(sets, "last_event_fire = NULL")
		}
		if in.EventStarts != nil {
			sets = append(sets, fmt.Sprintf("event_starts = $%d", idx))
			args = append(args, *in.EventStarts)
			idx++
		}
		if in.EventEnds != nil {
			sets = append(sets, fmt.Sprintf("event_ends = $%d", idx))
			args = append(args, *in.EventEnds)
			idx++
		}
		if in.EventIntervalFld != nil {
			sets = append(sets, fmt.Sprintf("event_interval_field = $%d", idx))
			args = append(args, *in.EventIntervalFld)
			idx++
		}
		if in.EventIntervalVal != nil {
			sets = append(sets, fmt.Sprintf("event_interval_value = $%d", idx))
			args = append(args, *in.EventIntervalVal)
			idx++
		}
	}
	if in.EventEnabled != nil {
		sets = append(sets, fmt.Sprintf("event_enabled = $%d", idx))
		args = append(args, *in.EventEnabled)
		idx++
	}
	if in.UpdatedByUserID != nil {
		sets = append(sets, fmt.Sprintf("updated_by_user_id = $%d", idx))
		args = append(args, *in.UpdatedByUserID)
		idx++
	}

	if len(sets) > 0 {
		args = append(args, jobID)
		q := fmt.Sprintf("UPDATE job SET %s WHERE job_id = $%d", strings.Join(sets, ", "), idx)
		tag, err := tx.Exec(ctx, q, args...)
		if err != nil {
			return Job{}, fmt.Errorf("update job: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return Job{}, ErrNotFound
		}
	} else {
		// verify existence even when no fields changed
		var exists bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM job WHERE job_id = $1)`, jobID,
		).Scan(&exists); err != nil {
			return Job{}, fmt.Errorf("verify job: %w", err)
		}
		if !exists {
			return Job{}, ErrNotFound
		}
	}
	if in.TaskIDs != nil {
		if err := setJobTasks(ctx, tx, jobID, *in.TaskIDs); err != nil {
			return Job{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Job{}, fmt.Errorf("commit: %w", err)
	}
	return r.Get(ctx, jobID)
}

// Delete removes the job (and cascades to job_task_link + job_run +
// job_run_output).
func (r *JobRepo) Delete(ctx context.Context, jobID int64) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM job WHERE job_id = $1`, jobID)
	if err != nil {
		return fmt.Errorf("delete job %d: %w", jobID, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListRuns returns every run for jobID ordered newest-first.
func (r *JobRepo) ListRuns(ctx context.Context, jobID int64) ([]Run, error) {
	rows, err := r.pool.Query(ctx, `
SELECT run_id, job_id, state, created_at, updated_at
FROM job_run
WHERE job_id = $1
ORDER BY created_at DESC`, jobID)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()
	var out []Run
	for rows.Next() {
		var r Run
		if err := rows.Scan(&r.RunID, &r.JobID, &r.State, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetRun returns a single run by id.
func (r *JobRepo) GetRun(ctx context.Context, runID uuid.UUID) (Run, error) {
	var run Run
	err := r.pool.QueryRow(ctx, `
SELECT run_id, job_id, state, created_at, updated_at
FROM job_run
WHERE run_id = $1`, runID).Scan(&run.RunID, &run.JobID, &run.State, &run.CreatedAt, &run.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Run{}, ErrNotFound
		}
		return Run{}, fmt.Errorf("get run: %w", err)
	}
	return run, nil
}

// DeleteRun removes a single run (cascades job_run_output).
func (r *JobRepo) DeleteRun(ctx context.Context, runID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM job_run WHERE run_id = $1`, runID)
	if err != nil {
		return fmt.Errorf("delete run: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateRun inserts a fresh job_run row in state='running' and returns
// the assigned run UUID.
func (r *JobRepo) CreateRun(ctx context.Context, jobID int64) (uuid.UUID, error) {
	id := uuid.New()
	_, err := r.pool.Exec(ctx, `
INSERT INTO job_run (run_id, job_id, state)
VALUES ($1, $2, 'running')`, id, jobID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("create run: %w", err)
	}
	return id, nil
}

// FinishRun transitions a run to a terminal state.
func (r *JobRepo) FinishRun(ctx context.Context, runID uuid.UUID, state string) error {
	_, err := r.pool.Exec(ctx, `
UPDATE job_run SET state = $1, updated_at = now() WHERE run_id = $2`, state, runID)
	return err
}

// AppendOutput writes a line to job_run_output. Seq is auto-allocated
// as one more than the current max for the run.
func (r *JobRepo) AppendOutput(ctx context.Context, runID uuid.UUID, taskID *int64, task, outType, message string) error {
	_, err := r.pool.Exec(ctx, `
INSERT INTO job_run_output (run_id, seq, ts, task_id, task, type, message)
SELECT $1, COALESCE(MAX(seq), 0) + 1, now(), $2, $3, $4, $5
FROM job_run_output
WHERE run_id = $1`, runID, taskID, task, outType, message)
	return err
}

// ListOutput returns every output line for runID with seq > afterSeq,
// ordered by seq ascending. afterSeq=0 returns the full log.
func (r *JobRepo) ListOutput(ctx context.Context, runID uuid.UUID, afterSeq int64) ([]RunOutput, error) {
	rows, err := r.pool.Query(ctx, `
SELECT seq, ts, task_id, task, type, message
FROM job_run_output
WHERE run_id = $1 AND seq > $2
ORDER BY seq ASC`, runID, afterSeq)
	if err != nil {
		return nil, fmt.Errorf("list output: %w", err)
	}
	defer rows.Close()
	var out []RunOutput
	for rows.Next() {
		var o RunOutput
		if err := rows.Scan(&o.Seq, &o.Ts, &o.TaskID, &o.Task, &o.Type, &o.Message); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// hydrateJob loads job.Tasks + RunCount + LastRun for an already-
// hydrated job row.
func (r *JobRepo) hydrateJob(ctx context.Context, j *Job) error {
	rows, err := r.pool.Query(ctx, `
SELECT t.task_id, t.name, t.description, t.command
FROM job_task_link l
JOIN job_task t ON t.task_id = l.task_id
WHERE l.job_id = $1
ORDER BY l.seq ASC`, j.JobID)
	if err != nil {
		return fmt.Errorf("hydrate job tasks: %w", err)
	}
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.TaskID, &t.Name, &t.Description, &t.Command); err != nil {
			rows.Close()
			return err
		}
		j.Tasks = append(j.Tasks, t)
	}
	rows.Close()

	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM job_run WHERE job_id = $1`, j.JobID,
	).Scan(&j.RunCount); err != nil {
		return fmt.Errorf("hydrate run count: %w", err)
	}
	var last Run
	err = r.pool.QueryRow(ctx, `
SELECT run_id, job_id, state, created_at, updated_at
FROM job_run
WHERE job_id = $1
ORDER BY created_at DESC
LIMIT 1`, j.JobID).Scan(&last.RunID, &last.JobID, &last.State, &last.CreatedAt, &last.UpdatedAt)
	if err == nil {
		j.LastRun = &last
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("hydrate last run: %w", err)
	}
	return nil
}

// setJobTasks replaces the job_task_link rows for jobID with the given
// ordered taskIDs. Verifies each task exists.
func setJobTasks(ctx context.Context, tx pgx.Tx, jobID int64, taskIDs []int64) error {
	if _, err := tx.Exec(ctx, `DELETE FROM job_task_link WHERE job_id = $1`, jobID); err != nil {
		return fmt.Errorf("clear task links: %w", err)
	}
	for i, tid := range taskIDs {
		if _, err := tx.Exec(ctx, `
INSERT INTO job_task_link (job_id, task_id, seq) VALUES ($1, $2, $3)`,
			jobID, tid, i); err != nil {
			return fmt.Errorf("link task %d: %w", tid, err)
		}
	}
	return nil
}

// scanJobRow demultiplexes a SELECT job_id, name, …, created_at,
// updated_at, created_by_user_id, updated_by_user_id row.
func scanJobRow(s pgRow) (Job, error) {
	var j Job
	err := s.Scan(
		&j.JobID, &j.Name, &j.Description,
		&j.EventType, &j.EventID, &j.EventEnabled, &j.EventStarts, &j.EventEnds,
		&j.EventIntervalFld, &j.EventIntervalVal, &j.LastEventFire,
		&j.CreatedAt, &j.UpdatedAt,
		&j.CreatedByUserID, &j.UpdatedByUserID,
	)
	return j, err
}

// ListEnabledScheduled returns every job with a non-NULL event_type
// and event_enabled = TRUE. Tasks and run summaries are NOT hydrated;
// the scheduler only needs the trigger metadata.
func (r *JobRepo) ListEnabledScheduled(ctx context.Context) ([]Job, error) {
	rows, err := r.pool.Query(ctx, `
SELECT job_id, name, description,
       event_type, event_id, event_enabled, event_starts, event_ends,
       event_interval_field, event_interval_value, last_event_fire,
       created_at, updated_at,
       created_by_user_id, updated_by_user_id
FROM job
WHERE event_type IS NOT NULL AND event_enabled = TRUE
ORDER BY job_id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list scheduled jobs: %w", err)
	}
	defer rows.Close()
	var out []Job
	for rows.Next() {
		j, err := scanJobRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// ClaimEventFire conditionally updates last_event_fire for jobID to
// fireTime, returning (true, nil) when the update happened. The
// condition (last_event_fire IS NULL OR last_event_fire < fireTime)
// makes the claim idempotent across concurrent scheduler ticks and
// safe against restarts: two ticks proposing the same fire time will
// only succeed once.
func (r *JobRepo) ClaimEventFire(ctx context.Context, jobID int64, fireTime time.Time) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
UPDATE job
SET    last_event_fire = $2,
       updated_at      = now()
WHERE  job_id = $1
  AND  (last_event_fire IS NULL OR last_event_fire < $2)
  AND  event_enabled = TRUE
  AND  event_type IS NOT NULL`, jobID, fireTime)
	if err != nil {
		return false, fmt.Errorf("claim event fire: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// newEventID generates a short, stable string handle for the event
// column. Upstream surfaces this as a 45-char identifier so clients
// can correlate occurrences of recurring jobs.
func newEventID() string {
	return "evt-" + uuid.NewString()
}
