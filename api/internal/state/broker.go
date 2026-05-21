// Package state hosts the in-process publish/subscribe broker that
// powers /op/state/sse. It deliberately avoids any external
// dependencies (no Redis, no NATS) — events are fanned out to in-
// process subscribers only. A future milestone can swap the
// implementation for a multi-node broadcaster without changing the
// surface used by callers.
package state

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"
)

// EventType enumerates the SSE event names emitted on the wire.
type EventType string

const (
	// EventStateSnapshot is the inaugural event emitted to every new
	// subscriber so it can render the current dependencies/state
	// without making a separate GET /op/state call.
	EventStateSnapshot EventType = "state.snapshot"
	// EventStateChanged announces a change to one of the values
	// surfaced by GET /op/state (e.g. database connectivity dropped).
	EventStateChanged EventType = "state.changed"
	// EventJobRun announces a transition in a job run's state
	// (running → completed | failed). The payload is JobRunEvent.
	EventJobRun EventType = "job.run"
)

// Event is a single SSE-bound message.
type Event struct {
	// Type drives the SSE "event:" field. Required.
	Type EventType `json:"type"`
	// Data is JSON-encoded and emitted as the "data:" field.
	Data any `json:"data,omitempty"`
	// Ts records when the event was published. Mirrored into "data"
	// when no explicit timestamp is provided by the publisher.
	Ts time.Time `json:"ts"`
}

// Snapshot is the payload of EventStateSnapshot + EventStateChanged.
// CurrentState is one of "available" | "starting" | "unavailable" —
// it matches the State enum in the OpenAPI spec.
type Snapshot struct {
	CurrentState string    `json:"currentState"`
	Db           bool      `json:"db"`
	Oidc         bool      `json:"oidc"`
	UI           string    `json:"ui,omitempty"`
	Since        time.Time `json:"since"`
}

// JobRunEvent is the payload of EventJobRun.
type JobRunEvent struct {
	RunID    string `json:"runId"`
	JobID    string `json:"jobId"`
	State    string `json:"state"`
	Message  string `json:"message,omitempty"`
}

// Broker fans out Events to every active subscriber.
type Broker struct {
	mu          sync.RWMutex
	subscribers map[uint64]chan Event
	nextID      atomic.Uint64
	// last is the most-recent snapshot published; replayed to new
	// subscribers so they don't have to wait for the next transition.
	last      Snapshot
	since     time.Time
	publishMu sync.Mutex
}

// NewBroker returns a Broker initialised with a "starting" snapshot.
func NewBroker() *Broker {
	now := time.Now().UTC()
	b := &Broker{
		subscribers: map[uint64]chan Event{},
		since:       now,
		last: Snapshot{
			CurrentState: "starting",
			Since:        now,
		},
	}
	return b
}

// Subscribe registers a channel that will receive future events.
// Unsubscribe by cancelling ctx or by calling the returned function.
// The buffered channel has space for 32 events; if a slow consumer
// fills it, subsequent publishes for that subscriber are silently
// dropped. The returned cancel func is idempotent.
func (b *Broker) Subscribe(ctx context.Context) (<-chan Event, func()) {
	ch := make(chan Event, 32)
	id := b.nextID.Add(1)
	b.mu.Lock()
	b.subscribers[id] = ch
	b.mu.Unlock()

	cancel := func() {
		b.mu.Lock()
		if c, ok := b.subscribers[id]; ok {
			delete(b.subscribers, id)
			close(c)
		}
		b.mu.Unlock()
	}

	// Replay the latest snapshot so the new subscriber can render the
	// current dependency state without a follow-up REST call.
	ch <- Event{
		Type: EventStateSnapshot,
		Data: b.LastSnapshot(),
		Ts:   time.Now().UTC(),
	}

	if ctx != nil {
		go func() {
			<-ctx.Done()
			cancel()
		}()
	}
	return ch, cancel
}

// Publish broadcasts ev to every active subscriber. Slow subscribers
// are not blocked on — events for full channels are dropped.
func (b *Broker) Publish(ev Event) {
	b.publishMu.Lock()
	if ev.Ts.IsZero() {
		ev.Ts = time.Now().UTC()
	}
	b.publishMu.Unlock()

	b.mu.RLock()
	for _, ch := range b.subscribers {
		select {
		case ch <- ev:
		default:
		}
	}
	b.mu.RUnlock()
}

// PublishSnapshot updates the cached snapshot and emits an
// EventStateChanged when the dependency bits or current state differ
// from the previously cached value.
func (b *Broker) PublishSnapshot(s Snapshot) {
	b.publishMu.Lock()
	prev := b.last
	if s.Since.IsZero() {
		s.Since = prev.Since
		if s.Since.IsZero() {
			s.Since = time.Now().UTC()
		}
	}
	b.last = s
	b.publishMu.Unlock()

	if prev.CurrentState == s.CurrentState && prev.Db == s.Db && prev.Oidc == s.Oidc {
		return
	}
	b.Publish(Event{Type: EventStateChanged, Data: s})
}

// LastSnapshot returns a copy of the cached snapshot.
func (b *Broker) LastSnapshot() Snapshot {
	b.publishMu.Lock()
	defer b.publishMu.Unlock()
	return b.last
}

// MarshalEvent encodes ev for transport over an SSE channel. Returned
// bytes do *not* include the trailing blank line — the SSE writer
// appends that.
func MarshalEvent(ev Event) ([]byte, error) {
	return json.Marshal(ev)
}
