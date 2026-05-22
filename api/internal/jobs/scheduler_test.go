package jobs

import (
	"testing"
	"time"

	"github.com/Exonical/stig-manager-react/api/internal/store"
)

func ptrStr(s string) *string        { return &s }
func ptrTime(t time.Time) *time.Time { return &t }

func TestComputeNextFire_Manual(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	j := store.Job{EventEnabled: true}
	if _, ok := ComputeNextFire(j, now); ok {
		t.Fatalf("manual job (event_type nil) should not fire")
	}
}

func TestComputeNextFire_Disabled(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	starts := now.Add(-time.Hour)
	j := store.Job{
		EventType:    ptrStr("once"),
		EventStarts:  &starts,
		EventEnabled: false,
	}
	if _, ok := ComputeNextFire(j, now); ok {
		t.Fatalf("disabled job should not fire")
	}
}

func TestComputeNextFire_OnceFirst(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	starts := now.Add(-time.Minute)
	j := store.Job{
		EventType:    ptrStr("once"),
		EventStarts:  &starts,
		EventEnabled: true,
	}
	got, ok := ComputeNextFire(j, now)
	if !ok {
		t.Fatalf("once with last_event_fire=nil should fire")
	}
	if !got.Equal(starts) {
		t.Fatalf("once first fire should equal event_starts, got %v want %v", got, starts)
	}
}

func TestComputeNextFire_OnceAlreadyFired(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	starts := now.Add(-time.Hour)
	last := now.Add(-30 * time.Minute)
	j := store.Job{
		EventType:     ptrStr("once"),
		EventStarts:   &starts,
		EventEnabled:  true,
		LastEventFire: &last,
	}
	if _, ok := ComputeNextFire(j, now); ok {
		t.Fatalf("once with last_event_fire set should not re-fire")
	}
}

func TestComputeNextFire_RecurringFirst(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	starts := now.Add(-15 * time.Minute)
	j := store.Job{
		EventType:        ptrStr("recurring"),
		EventStarts:      &starts,
		EventEnabled:     true,
		EventIntervalFld: ptrStr("hour"),
		EventIntervalVal: ptrStr("1"),
	}
	got, ok := ComputeNextFire(j, now)
	if !ok {
		t.Fatalf("recurring with last_event_fire=nil should fire")
	}
	if !got.Equal(starts) {
		t.Fatalf("recurring first fire should equal event_starts, got %v want %v", got, starts)
	}
}

func TestComputeNextFire_RecurringIntervals(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	last := now.Add(-time.Hour)

	cases := []struct {
		field string
		value string
		want  time.Time
	}{
		{"minute", "5", last.Add(5 * time.Minute)},
		{"hour", "2", last.Add(2 * time.Hour)},
		{"day", "1", last.AddDate(0, 0, 1)},
		{"week", "2", last.AddDate(0, 0, 14)},
		{"month", "1", last.AddDate(0, 1, 0)},
		{"HOUR", "1", last.Add(time.Hour)},       // case-insensitive
		{"hour", " 3 ", last.Add(3 * time.Hour)}, // trimmed
	}
	for _, c := range cases {
		t.Run(c.field+"/"+c.value, func(t *testing.T) {
			fld := c.field
			val := c.value
			j := store.Job{
				EventType:        ptrStr("recurring"),
				EventStarts:      &last, // first-fire only used if LastEventFire nil
				EventEnabled:     true,
				LastEventFire:    &last,
				EventIntervalFld: &fld,
				EventIntervalVal: &val,
			}
			got, ok := ComputeNextFire(j, now)
			if !ok {
				t.Fatalf("expected schedulable, got ok=false")
			}
			if !got.Equal(c.want) {
				t.Fatalf("next fire mismatch: got %v want %v", got, c.want)
			}
		})
	}
}

func TestComputeNextFire_InvalidInterval(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	starts := now.Add(-time.Hour)
	last := now.Add(-30 * time.Minute)

	cases := []struct {
		name  string
		field string
		value string
	}{
		{"unknown_field", "fortnight", "1"},
		{"non_numeric_value", "hour", "abc"},
		{"zero_value", "hour", "0"},
		{"negative_value", "hour", "-1"},
		{"empty_value", "hour", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fld := c.field
			val := c.value
			j := store.Job{
				EventType:        ptrStr("recurring"),
				EventStarts:      &starts,
				EventEnabled:     true,
				LastEventFire:    &last,
				EventIntervalFld: &fld,
				EventIntervalVal: &val,
			}
			if _, ok := ComputeNextFire(j, now); ok {
				t.Fatalf("invalid interval %q/%q should not be schedulable", c.field, c.value)
			}
		})
	}
}

func TestComputeNextFire_MissingIntervalFields(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	starts := now.Add(-time.Hour)
	last := now.Add(-30 * time.Minute)

	// Recurring without interval_field / interval_value.
	val := "1"
	j := store.Job{
		EventType:        ptrStr("recurring"),
		EventStarts:      &starts,
		EventEnabled:     true,
		LastEventFire:    &last,
		EventIntervalVal: &val,
		// EventIntervalFld nil
	}
	if _, ok := ComputeNextFire(j, now); ok {
		t.Fatalf("recurring without interval_field should not fire")
	}

	fld := "hour"
	j2 := store.Job{
		EventType:        ptrStr("recurring"),
		EventStarts:      &starts,
		EventEnabled:     true,
		LastEventFire:    &last,
		EventIntervalFld: &fld,
		// EventIntervalVal nil
	}
	if _, ok := ComputeNextFire(j2, now); ok {
		t.Fatalf("recurring without interval_value should not fire")
	}
}

func TestComputeNextFire_EventEnds(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	starts := now.Add(-time.Hour)
	last := now.Add(-time.Minute)
	ends := now.Add(-time.Second) // event_ends is in the past relative to next fire

	fld := "hour"
	val := "1"
	j := store.Job{
		EventType:        ptrStr("recurring"),
		EventStarts:      &starts,
		EventEnds:        &ends,
		EventEnabled:     true,
		LastEventFire:    &last,
		EventIntervalFld: &fld,
		EventIntervalVal: &val,
	}
	if _, ok := ComputeNextFire(j, now); ok {
		t.Fatalf("next fire (last+1h) is past event_ends, should not fire")
	}

	// Once-time job whose start is past event_ends.
	jOnce := store.Job{
		EventType:    ptrStr("once"),
		EventStarts:  ptrTime(now.Add(time.Hour)),
		EventEnds:    ptrTime(now.Add(time.Minute)),
		EventEnabled: true,
	}
	if _, ok := ComputeNextFire(jOnce, now); ok {
		t.Fatalf("once with event_starts > event_ends should not fire")
	}
}

func TestComputeNextFire_UnknownEventType(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	starts := now.Add(-time.Hour)
	j := store.Job{
		EventType:    ptrStr("daily-at-noon"), // not 'once' or 'recurring'
		EventStarts:  &starts,
		EventEnabled: true,
	}
	if _, ok := ComputeNextFire(j, now); ok {
		t.Fatalf("unknown event_type should not be schedulable")
	}
}

func TestNewScheduler_TickValidation(t *testing.T) {
	_, err := NewScheduler(nil, nil, SchedulerConfig{Tick: 0})
	if err == nil {
		t.Fatalf("expected error for Tick=0")
	}
	_, err = NewScheduler(nil, nil, SchedulerConfig{Tick: -1})
	if err == nil {
		t.Fatalf("expected error for negative Tick")
	}
	_, err = NewScheduler(nil, nil, SchedulerConfig{Tick: time.Second})
	if err != nil {
		t.Fatalf("Tick=1s should be accepted, got: %v", err)
	}
}
