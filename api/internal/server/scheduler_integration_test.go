//go:build integration

package server_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/Exonical/stig-manager-react/api/internal/jobs"
	"github.com/Exonical/stig-manager-react/api/internal/store"
)

// schedulerHarness assembles the scheduler against a live postgres
// pool with the built-in task registry seeded. The returned scheduler
// is synchronous, so RunOnce blocks until every dispatched run
// finishes — keeps the assertions deterministic.
type schedulerHarness struct {
	repo      *store.JobRepo
	runner    *jobs.Runner
	scheduler *jobs.Scheduler
	noopID    int64
}

func newSchedulerHarness(t *testing.T) (*schedulerHarness, context.Context) {
	t.Helper()
	pool := newIntegrationPool(t)
	ctx := context.Background()
	repo := store.NewJobRepo(pool)

	registry := jobs.NewBuiltinRegistry()
	if err := registry.Seed(ctx, repo); err != nil {
		t.Fatalf("seed registry: %v", err)
	}
	noopID, ok := registry.IDByName("noop")
	if !ok {
		t.Fatalf("noop task id not seeded")
	}
	runner := jobs.NewRunner(repo, registry, slog.Default())
	sched, err := jobs.NewScheduler(repo, runner, jobs.SchedulerConfig{
		Tick:        time.Hour, // never auto-ticks; tests drive RunOnce manually
		Logger:      slog.Default(),
		Synchronous: true,
	})
	if err != nil {
		t.Fatalf("scheduler: %v", err)
	}
	return &schedulerHarness{
		repo: repo, runner: runner, scheduler: sched, noopID: noopID,
	}, ctx
}

func ptrStrInt(s string) *string { return &s }

// TestSchedulerFiresOnceJob exercises the 'once' event_type: the
// scheduler must create exactly one run for a 'once' job whose
// event_starts is in the past, and must NOT create a second run on
// subsequent ticks.
func TestSchedulerFiresOnceJob(t *testing.T) {
	h, ctx := newSchedulerHarness(t)

	starts := time.Now().Add(-time.Minute).UTC().Truncate(time.Microsecond)
	job, err := h.repo.Create(ctx, store.JobCreate{
		Name:        "sched-once",
		EventType:   ptrStrInt("once"),
		EventStarts: &starts,
		TaskIDs:     []int64{h.noopID},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	h.scheduler.RunOnce(ctx)
	runs, err := h.repo.ListRuns(ctx, job.JobID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("after first tick: want 1 run, got %d", len(runs))
	}
	if runs[0].State != "completed" {
		t.Fatalf("after first tick: want state=completed, got %q", runs[0].State)
	}

	got, err := h.repo.Get(ctx, job.JobID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.LastEventFire == nil {
		t.Fatalf("last_event_fire should be populated")
	}
	if !got.LastEventFire.Equal(starts) {
		t.Fatalf("last_event_fire: got %v, want %v", got.LastEventFire, starts)
	}

	// Subsequent ticks must NOT re-fire the once-time job.
	h.scheduler.RunOnce(ctx)
	h.scheduler.RunOnce(ctx)
	runs, err = h.repo.ListRuns(ctx, job.JobID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("after re-ticks: want 1 run, got %d", len(runs))
	}
}

// TestSchedulerFiresRecurringJob exercises the 'recurring' event_type:
// a job with a 1-minute interval and event_starts=2m ago should fire
// twice on back-to-back ticks (catching up two missed windows) and
// then stop until the next interval boundary has passed.
func TestSchedulerFiresRecurringJob(t *testing.T) {
	h, ctx := newSchedulerHarness(t)

	starts := time.Now().Add(-2 * time.Minute).UTC().Truncate(time.Microsecond)
	job, err := h.repo.Create(ctx, store.JobCreate{
		Name:             "sched-recurring",
		EventType:        ptrStrInt("recurring"),
		EventStarts:      &starts,
		EventIntervalFld: ptrStrInt("minute"),
		EventIntervalVal: ptrStrInt("1"),
		TaskIDs:          []int64{h.noopID},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	// Tick 1: first fire at event_starts.
	h.scheduler.RunOnce(ctx)
	runs, err := h.repo.ListRuns(ctx, job.JobID)
	if err != nil {
		t.Fatalf("list runs after 1: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("after tick 1: want 1 run, got %d", len(runs))
	}

	// Tick 2: next fire = starts + 1m, still in the past → fires again.
	h.scheduler.RunOnce(ctx)
	runs, err = h.repo.ListRuns(ctx, job.JobID)
	if err != nil {
		t.Fatalf("list runs after 2: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("after tick 2: want 2 runs, got %d", len(runs))
	}

	// Tick 3: next fire = starts + 2m, ~now or future → may or may not
	// fire depending on race with wall clock. The strong invariant is
	// that we never go beyond the expected catch-up window. Allow 2 or
	// 3 runs here.
	h.scheduler.RunOnce(ctx)
	runs, err = h.repo.ListRuns(ctx, job.JobID)
	if err != nil {
		t.Fatalf("list runs after 3: %v", err)
	}
	if got := len(runs); got < 2 || got > 3 {
		t.Fatalf("after tick 3: want 2..3 runs, got %d", got)
	}
}

// TestSchedulerSkipsDisabledJob verifies event_enabled=false short-
// circuits the scheduler even when the timing window has elapsed.
func TestSchedulerSkipsDisabledJob(t *testing.T) {
	h, ctx := newSchedulerHarness(t)

	starts := time.Now().Add(-time.Hour).UTC()
	enabled := false
	job, err := h.repo.Create(ctx, store.JobCreate{
		Name:         "sched-disabled",
		EventType:    ptrStrInt("once"),
		EventStarts:  &starts,
		EventEnabled: &enabled,
		TaskIDs:      []int64{h.noopID},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	h.scheduler.RunOnce(ctx)
	runs, err := h.repo.ListRuns(ctx, job.JobID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("disabled job should not fire, got %d runs", len(runs))
	}
}

// TestSchedulerHonorsEventEnds confirms event_ends bounds the
// recurring schedule: once the next fire-time exceeds event_ends,
// no further runs are dispatched.
func TestSchedulerHonorsEventEnds(t *testing.T) {
	h, ctx := newSchedulerHarness(t)

	starts := time.Now().Add(-2 * time.Minute).UTC().Truncate(time.Microsecond)
	ends := time.Now().Add(-90 * time.Second).UTC() // <=1m after starts; only first fire is within window
	job, err := h.repo.Create(ctx, store.JobCreate{
		Name:             "sched-bounded",
		EventType:        ptrStrInt("recurring"),
		EventStarts:      &starts,
		EventEnds:        &ends,
		EventIntervalFld: ptrStrInt("minute"),
		EventIntervalVal: ptrStrInt("1"),
		TaskIDs:          []int64{h.noopID},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	// First fire happens at starts (which is within [starts, ends]).
	h.scheduler.RunOnce(ctx)
	runs, err := h.repo.ListRuns(ctx, job.JobID)
	if err != nil {
		t.Fatalf("list runs after 1: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("after tick 1: want 1 run, got %d", len(runs))
	}

	// Second hypothetical fire would be at starts+1m, which is > ends
	// → scheduler must NOT dispatch.
	h.scheduler.RunOnce(ctx)
	runs, err = h.repo.ListRuns(ctx, job.JobID)
	if err != nil {
		t.Fatalf("list runs after 2: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("after tick 2: want 1 run (event_ends bounded), got %d", len(runs))
	}
}

// TestSchedulerIdempotentClaim verifies the conditional UPDATE in
// ClaimEventFire prevents double-fires on concurrent ticks: two
// schedulers (or two raced ticks) attempting to claim the same
// fire-time produce at most one run.
func TestSchedulerIdempotentClaim(t *testing.T) {
	h, ctx := newSchedulerHarness(t)

	starts := time.Now().Add(-time.Minute).UTC().Truncate(time.Microsecond)
	job, err := h.repo.Create(ctx, store.JobCreate{
		Name:        "sched-claim",
		EventType:   ptrStrInt("once"),
		EventStarts: &starts,
		TaskIDs:     []int64{h.noopID},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	// First claim succeeds.
	claimed1, err := h.repo.ClaimEventFire(ctx, job.JobID, starts)
	if err != nil {
		t.Fatalf("claim 1: %v", err)
	}
	if !claimed1 {
		t.Fatalf("first claim should succeed")
	}

	// Second claim at the same fire-time must fail (idempotent).
	claimed2, err := h.repo.ClaimEventFire(ctx, job.JobID, starts)
	if err != nil {
		t.Fatalf("claim 2: %v", err)
	}
	if claimed2 {
		t.Fatalf("second claim at same fire-time should fail")
	}

	// A claim at a strictly later fire-time succeeds.
	later := starts.Add(time.Minute)
	claimed3, err := h.repo.ClaimEventFire(ctx, job.JobID, later)
	if err != nil {
		t.Fatalf("claim 3: %v", err)
	}
	if !claimed3 {
		t.Fatalf("later claim should succeed")
	}
}

// TestSchedulerPatchResetsLastEventFire exercises the M20.1 fix:
// after a 'once' job fires, PATCHing its event_type must clear
// last_event_fire so the scheduler fires it again.
func TestSchedulerPatchResetsLastEventFire(t *testing.T) {
	h, ctx := newSchedulerHarness(t)

	starts := time.Now().Add(-time.Minute).UTC().Truncate(time.Microsecond)
	job, err := h.repo.Create(ctx, store.JobCreate{
		Name:        "sched-repatch",
		EventType:   ptrStrInt("once"),
		EventStarts: &starts,
		TaskIDs:     []int64{h.noopID},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	// Fire once.
	h.scheduler.RunOnce(ctx)
	runs, err := h.repo.ListRuns(ctx, job.JobID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("want 1 run after first tick, got %d", len(runs))
	}

	// Confirm last_event_fire is set.
	got, err := h.repo.Get(ctx, job.JobID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.LastEventFire == nil {
		t.Fatalf("last_event_fire should be set after first fire")
	}

	// PATCH the job with a new event_type (re-schedule).
	newStarts := time.Now().Add(-30 * time.Second).UTC().Truncate(time.Microsecond)
	_, err = h.repo.Patch(ctx, job.JobID, store.JobPatch{
		EventType:   ptrStrInt("once"),
		EventStarts: &newStarts,
	})
	if err != nil {
		t.Fatalf("patch job: %v", err)
	}

	// Verify last_event_fire was cleared.
	got, err = h.repo.Get(ctx, job.JobID)
	if err != nil {
		t.Fatalf("get job after patch: %v", err)
	}
	if got.LastEventFire != nil {
		t.Fatalf("last_event_fire should be nil after patch, got %v", got.LastEventFire)
	}

	// Fire again — should produce a second run.
	h.scheduler.RunOnce(ctx)
	runs, err = h.repo.ListRuns(ctx, job.JobID)
	if err != nil {
		t.Fatalf("list runs after re-fire: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("want 2 runs after re-fire, got %d", len(runs))
	}
}

// TestSchedulerClearEventResetsLastEventFire verifies that the
// ClearEvent path also resets last_event_fire.
func TestSchedulerClearEventResetsLastEventFire(t *testing.T) {
	h, ctx := newSchedulerHarness(t)

	starts := time.Now().Add(-time.Minute).UTC().Truncate(time.Microsecond)
	job, err := h.repo.Create(ctx, store.JobCreate{
		Name:        "sched-clear",
		EventType:   ptrStrInt("once"),
		EventStarts: &starts,
		TaskIDs:     []int64{h.noopID},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	// Fire once.
	h.scheduler.RunOnce(ctx)
	got, err := h.repo.Get(ctx, job.JobID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.LastEventFire == nil {
		t.Fatalf("last_event_fire should be set")
	}

	// ClearEvent — must null last_event_fire too.
	_, err = h.repo.Patch(ctx, job.JobID, store.JobPatch{ClearEvent: true})
	if err != nil {
		t.Fatalf("patch clear: %v", err)
	}
	got, err = h.repo.Get(ctx, job.JobID)
	if err != nil {
		t.Fatalf("get after clear: %v", err)
	}
	if got.LastEventFire != nil {
		t.Fatalf("last_event_fire should be nil after ClearEvent, got %v", got.LastEventFire)
	}
}
