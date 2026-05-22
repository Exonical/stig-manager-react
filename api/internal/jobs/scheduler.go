package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Exonical/stig-manager-react/api/internal/store"
)

// Scheduler dispatches the recurring + once-time jobs persisted in
// the `job` table. The loop polls every TickFreq, computes the next
// fire-time for each enabled scheduled job, and races a conditional
// UPDATE against the row. The UPDATE acts as the claim: only the
// scheduler that wins the race starts the run.
//
// The scheduler is meant to be a singleton per process; running two
// instances against the same database is safe (the claim is
// idempotent) but pointless.
type Scheduler struct {
	repo        *store.JobRepo
	runner      *Runner
	logger      *slog.Logger
	tick        time.Duration
	now         func() time.Time
	synchronous bool
}

// SchedulerConfig configures a Scheduler. Tick must be > 0.
type SchedulerConfig struct {
	Tick   time.Duration
	Logger *slog.Logger
	// Now overrides the clock for tests. nil → time.Now.
	Now func() time.Time
	// Synchronous makes RunOnce wait for each dispatched run to
	// finish before moving on. Production leaves this false so the
	// scheduler can fan out fast; integration tests flip it on so
	// assertions can read final state without polling.
	Synchronous bool
}

// NewScheduler wires a Scheduler against the given repo + runner.
// Returns an error when cfg.Tick is <= 0.
func NewScheduler(repo *store.JobRepo, runner *Runner, cfg SchedulerConfig) (*Scheduler, error) {
	if cfg.Tick <= 0 {
		return nil, fmt.Errorf("scheduler: Tick must be > 0, got %v", cfg.Tick)
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Scheduler{
		repo:        repo,
		runner:      runner,
		logger:      cfg.Logger,
		tick:        cfg.Tick,
		now:         cfg.Now,
		synchronous: cfg.Synchronous,
	}, nil
}

// Run blocks until ctx is cancelled, ticking every cfg.Tick and
// firing any due jobs. The initial dispatch happens immediately so
// callers don't have to wait a tick after start-up.
func (s *Scheduler) Run(ctx context.Context) {
	s.logger.InfoContext(ctx, "scheduler started", "tick", s.tick)
	defer s.logger.Info("scheduler stopped")

	s.RunOnce(ctx)
	t := time.NewTicker(s.tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.RunOnce(ctx)
		}
	}
}

// RunOnce dispatches every due job synchronously and returns when the
// cycle finishes. Exported for tests that need to step the scheduler
// without spinning up a goroutine + ticker.
func (s *Scheduler) RunOnce(ctx context.Context) {
	jobs, err := s.repo.ListEnabledScheduled(ctx)
	if err != nil {
		s.logger.ErrorContext(ctx, "scheduler: list scheduled jobs", "err", err)
		return
	}
	now := s.now()
	for _, j := range jobs {
		next, ok := ComputeNextFire(j, now)
		if !ok {
			continue
		}
		if next.After(now) {
			continue
		}
		claimed, err := s.repo.ClaimEventFire(ctx, j.JobID, next)
		if err != nil {
			s.logger.ErrorContext(ctx, "scheduler: claim event fire",
				"err", err, "job_id", j.JobID)
			continue
		}
		if !claimed {
			continue
		}
		runID, err := s.repo.CreateRun(ctx, j.JobID)
		if err != nil {
			s.logger.ErrorContext(ctx, "scheduler: create run",
				"err", err, "job_id", j.JobID)
			continue
		}
		s.logger.InfoContext(ctx, "scheduler firing job",
			"job_id", j.JobID, "job_name", j.Name,
			"run_id", runID.String(), "fire_time", next.Format(time.RFC3339))
		if s.synchronous {
			s.runner.RunSync(ctx, j.JobID, runID)
		} else {
			s.runner.Start(ctx, j.JobID, runID)
		}
	}
}

// ComputeNextFire returns the timestamp at which job j should next
// fire, given the current time. The second return is false when the
// job is not schedulable (manual, missing trigger fields, malformed
// interval, past event_ends, or 'once' with last_event_fire set).
//
// Semantics:
//   - event_type='once': first call returns event_starts; subsequent
//     calls (last_event_fire != nil) return ok=false.
//   - event_type='recurring': first call returns event_starts;
//     subsequent calls return last_event_fire + interval.
//   - event_ends, when set, gates the result: any fire-time strictly
//     after event_ends produces ok=false.
//
// The function is pure (no I/O) so tests can drive it directly.
func ComputeNextFire(j store.Job, now time.Time) (time.Time, bool) {
	if j.EventType == nil {
		return time.Time{}, false
	}
	if !j.EventEnabled {
		return time.Time{}, false
	}
	if j.EventStarts == nil {
		return time.Time{}, false
	}
	switch *j.EventType {
	case "once":
		if j.LastEventFire != nil {
			return time.Time{}, false
		}
		next := *j.EventStarts
		if j.EventEnds != nil && next.After(*j.EventEnds) {
			return time.Time{}, false
		}
		return next, true
	case "recurring":
		if j.EventIntervalFld == nil || j.EventIntervalVal == nil {
			return time.Time{}, false
		}
		n, err := strconv.Atoi(strings.TrimSpace(*j.EventIntervalVal))
		if err != nil || n <= 0 {
			return time.Time{}, false
		}
		var next time.Time
		if j.LastEventFire == nil {
			next = *j.EventStarts
		} else {
			adv, ok := addInterval(*j.LastEventFire, *j.EventIntervalFld, n)
			if !ok {
				return time.Time{}, false
			}
			next = adv
		}
		if j.EventEnds != nil && next.After(*j.EventEnds) {
			return time.Time{}, false
		}
		return next, true
	}
	return time.Time{}, false
}

// addInterval advances t by n × field. minute/hour use multiplied
// durations; day/week/month use calendar arithmetic so DST + month
// boundaries don't silently double-fire or skip a day.
func addInterval(t time.Time, field string, n int) (time.Time, bool) {
	switch strings.ToLower(strings.TrimSpace(field)) {
	case "minute":
		return t.Add(time.Duration(n) * time.Minute), true
	case "hour":
		return t.Add(time.Duration(n) * time.Hour), true
	case "day":
		return t.AddDate(0, 0, n), true
	case "week":
		return t.AddDate(0, 0, 7*n), true
	case "month":
		return t.AddDate(0, n, 0), true
	default:
		return time.Time{}, false
	}
}
