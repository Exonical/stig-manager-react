// Package jobs implements the in-process task registry + runner that
// powers POST /jobs/{jobId}/runs. Tasks are registered at server
// start-up; each TaskFunc receives an *OutputWriter that streams
// stdout/stderr/system messages into job_run_output as they happen.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Exonical/stig-manager-react/api/internal/store"
)

// Outcome is the terminal state returned by a TaskFunc. nil err →
// "completed"; non-nil err → "failed" (and the remaining tasks in the
// run are skipped).
type Outcome = error

// TaskFunc is the in-process implementation of a task. The function
// owns its own goroutine for the duration of one run. Output should
// be streamed via w.Stdout / w.Stderr / w.System rather than buffered.
type TaskFunc func(ctx context.Context, w *OutputWriter) Outcome

// Definition pairs a task's display metadata (mirrored to the
// job_task table at start-up) with the TaskFunc that executes it.
type Definition struct {
	Name        string
	Description string
	Command     string
	Run         TaskFunc
}

// Registry is the set of tasks known to this process. Concurrent-safe.
type Registry struct {
	mu      sync.RWMutex
	byName  map[string]Definition
	byID    map[int64]Definition
	idByKey map[string]int64
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		byName:  map[string]Definition{},
		byID:    map[int64]Definition{},
		idByKey: map[string]int64{},
	}
}

// Register adds a task to the registry. Panics on duplicate names —
// this is a programmer error, called once at boot.
func (r *Registry) Register(d Definition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := lower(d.Name)
	if _, dup := r.byName[key]; dup {
		panic("jobs: duplicate task name " + d.Name)
	}
	r.byName[key] = d
}

// Lookup returns the definition matching name (case-insensitive).
func (r *Registry) Lookup(name string) (Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.byName[lower(name)]
	return d, ok
}

// LookupByID returns the definition for a previously-seeded task_id.
func (r *Registry) LookupByID(id int64) (Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.byID[id]
	return d, ok
}

// All returns every registered task (ordered by name).
func (r *Registry) All() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Definition, 0, len(r.byName))
	for _, d := range r.byName {
		out = append(out, d)
	}
	return out
}

// Seed installs the in-process registry into the job_task table, then
// caches the resulting task_id → Definition map so the runner can map
// from job_task_link rows back to TaskFuncs.
func (r *Registry) Seed(ctx context.Context, repo *store.JobRepo) error {
	for _, d := range r.All() {
		desc := stringPtr(d.Description)
		cmd := stringPtr(d.Command)
		id, err := repo.UpsertTask(ctx, d.Name, desc, cmd)
		if err != nil {
			return fmt.Errorf("seed task %q: %w", d.Name, err)
		}
		r.mu.Lock()
		r.byID[id] = d
		r.idByKey[lower(d.Name)] = id
		r.mu.Unlock()
	}
	return nil
}

// IDByName returns the task_id that was assigned to name during Seed.
// Useful for tests that need to wire a job to a task without poking
// at the database.
func (r *Registry) IDByName(name string) (int64, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.idByKey[lower(name)]
	return id, ok
}

// OutputWriter is the per-task sink for streamed output. Each call
// appends one row to job_run_output and bumps the run's updated_at.
type OutputWriter struct {
	ctx    context.Context
	repo   *store.JobRepo
	runID  uuid.UUID
	taskID int64
	task   string
}

// Stdout streams a stdout-typed message.
func (w *OutputWriter) Stdout(msg string) error {
	return w.append("stdout", msg)
}

// Stderr streams a stderr-typed message.
func (w *OutputWriter) Stderr(msg string) error {
	return w.append("stderr", msg)
}

// System streams a system-typed message (e.g. "task started" /
// "task succeeded").
func (w *OutputWriter) System(msg string) error {
	return w.append("system", msg)
}

// Stdoutf is the printf-flavoured Stdout.
func (w *OutputWriter) Stdoutf(format string, args ...any) error {
	return w.Stdout(fmt.Sprintf(format, args...))
}

func (w *OutputWriter) append(typ, msg string) error {
	tid := w.taskID
	return w.repo.AppendOutput(w.ctx, w.runID, &tid, w.task, typ, msg)
}

// Listener is the optional hook that observes job run transitions
// for downstream consumers (today: the /op/state/sse broker). The
// package keeps the interface generic so jobs doesn't depend on the
// state package directly.
type Listener interface {
	JobRunTransition(runID uuid.UUID, jobID int64, jobName, state, message string)
}

// Runner orchestrates a single job_run end-to-end. The zero value is
// not usable; construct via NewRunner.
type Runner struct {
	repo     *store.JobRepo
	registry *Registry
	logger   *slog.Logger
	listener Listener
}

// NewRunner wires the runner against a JobRepo + Registry.
func NewRunner(repo *store.JobRepo, reg *Registry, logger *slog.Logger) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{repo: repo, registry: reg, logger: logger}
}

// SetListener attaches a Listener that is notified at every run
// state transition (running on start, completed/failed on terminal).
func (rn *Runner) SetListener(l Listener) {
	rn.listener = l
}

func (rn *Runner) notify(runID uuid.UUID, jobID int64, jobName, state, message string) {
	if rn.listener != nil {
		rn.listener.JobRunTransition(runID, jobID, jobName, state, message)
	}
}

// Start dispatches a goroutine that executes every task linked to
// jobID in declaration order. The supplied parent context is used as
// the base; the goroutine itself runs to completion (it does not
// respect ctx cancellation today — failure modes will land alongside
// the cron scheduler in a follow-up).
func (rn *Runner) Start(parent context.Context, jobID int64, runID uuid.UUID) {
	go rn.execute(parent, jobID, runID)
}

// RunSync runs jobID/runID synchronously and returns the terminal
// state. Used by tests + by RunImmediateJob when the spec demands
// the response carry the final state (the OpenAPI Job.runImmediate
// response is asynchronous, but the test path exercises the
// synchronous variant for determinism).
func (rn *Runner) RunSync(ctx context.Context, jobID int64, runID uuid.UUID) string {
	return rn.execute(ctx, jobID, runID)
}

func (rn *Runner) execute(parent context.Context, jobID int64, runID uuid.UUID) string {
	ctx := context.WithoutCancel(parent)

	job, err := rn.repo.Get(ctx, jobID)
	if err != nil {
		rn.logger.ErrorContext(ctx, "runner: load job", "err", err, "jobId", jobID, "runId", runID)
		_ = rn.repo.FinishRun(ctx, runID, "failed")
		_ = rn.repo.AppendOutput(ctx, runID, nil, "runner", "system",
			fmt.Sprintf("failed to load job: %v", err))
		rn.notify(runID, jobID, "", "failed", err.Error())
		return "failed"
	}

	rn.notify(runID, jobID, job.Name, "running", "")

	if len(job.Tasks) == 0 {
		_ = rn.repo.AppendOutput(ctx, runID, nil, "runner", "system",
			"job has no tasks; nothing to do")
		_ = rn.repo.FinishRun(ctx, runID, "completed")
		rn.notify(runID, jobID, job.Name, "completed", "job has no tasks")
		return "completed"
	}

	for _, t := range job.Tasks {
		def, ok := rn.registry.LookupByID(t.TaskID)
		if !ok {
			_ = rn.repo.AppendOutput(ctx, runID, &t.TaskID, t.Name, "system",
				fmt.Sprintf("task %q not registered in this process", t.Name))
			_ = rn.repo.FinishRun(ctx, runID, "failed")
			rn.notify(runID, jobID, job.Name, "failed", "task not registered: "+t.Name)
			return "failed"
		}
		writer := &OutputWriter{
			ctx: ctx, repo: rn.repo, runID: runID, taskID: t.TaskID, task: t.Name,
		}
		_ = writer.System("task started")
		started := time.Now()
		err := def.Run(ctx, writer)
		elapsed := time.Since(started).Round(time.Millisecond)
		if err != nil {
			_ = writer.Stderr(err.Error())
			_ = writer.System(fmt.Sprintf("task failed in %s", elapsed))
			_ = rn.repo.FinishRun(ctx, runID, "failed")
			rn.notify(runID, jobID, job.Name, "failed", err.Error())
			return "failed"
		}
		_ = writer.System(fmt.Sprintf("task completed in %s", elapsed))
	}
	_ = rn.repo.FinishRun(ctx, runID, "completed")
	rn.notify(runID, jobID, job.Name, "completed", "")
	return "completed"
}

// BuiltinDefinitions returns the registry pre-seeded with the always-
// available tasks shipped in the binary: noop (does nothing,
// succeeds), ping (writes "pong"), fail (always fails — useful for
// exercising the failed state).
func BuiltinDefinitions() []Definition {
	return []Definition{
		{
			Name:        "noop",
			Description: "Does nothing and succeeds. Useful for scheduler smoke tests.",
			Command:     "noop",
			Run: func(ctx context.Context, w *OutputWriter) Outcome {
				return w.Stdout("noop")
			},
		},
		{
			Name:        "ping",
			Description: "Writes \"pong\" to stdout and succeeds.",
			Command:     "ping",
			Run: func(ctx context.Context, w *OutputWriter) Outcome {
				return w.Stdout("pong")
			},
		},
		{
			Name:        "fail",
			Description: "Always fails. Useful for exercising the failed state.",
			Command:     "fail",
			Run: func(ctx context.Context, w *OutputWriter) Outcome {
				_ = w.Stderr("this task always fails")
				return errors.New("intentional failure")
			},
		},
	}
}

// NewBuiltinRegistry returns a Registry pre-populated with the
// BuiltinDefinitions.
func NewBuiltinRegistry() *Registry {
	r := NewRegistry()
	for _, d := range BuiltinDefinitions() {
		r.Register(d)
	}
	return r
}

func lower(s string) string {
	// ASCII-only lowering avoids unicode.ToLower's allocator cost in
	// the hot lookup path. Task names are ASCII per the OpenAPI spec.
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
