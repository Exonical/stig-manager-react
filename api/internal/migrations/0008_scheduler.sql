-- +goose Up
-- +goose StatementBegin

-- job.last_event_fire tracks the most recent timestamp at which the
-- scheduler dispatched the job's event. NULL means the job has never
-- fired (or was reset). The scheduler uses this column to compute the
-- next-fire time for recurring jobs and to skip already-fired 'once'
-- events.
--
-- Idempotent dispatch is enforced with a conditional UPDATE that only
-- bumps last_event_fire when the proposed fire-time is strictly
-- greater than the current value, so two scheduler ticks racing on
-- the same row will only produce one run.
ALTER TABLE job
    ADD COLUMN last_event_fire TIMESTAMPTZ;

-- Partial index supports the scheduler's primary scan: enabled rows
-- with an event_type bound.
CREATE INDEX idx_job_scheduler_due
    ON job (event_type, event_enabled)
    WHERE event_type IS NOT NULL AND event_enabled = TRUE;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_job_scheduler_due;
ALTER TABLE job DROP COLUMN IF EXISTS last_event_fire;

-- +goose StatementEnd
