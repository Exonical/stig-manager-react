package state

import (
	"context"
	"testing"
	"time"
)

func TestBrokerReplaysSnapshot(t *testing.T) {
	b := NewBroker()
	b.PublishSnapshot(Snapshot{CurrentState: "available", Db: true, Oidc: true})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := b.Subscribe(ctx)

	select {
	case ev := <-ch:
		if ev.Type != EventStateSnapshot {
			t.Fatalf("first event should be snapshot, got %s", ev.Type)
		}
		s, ok := ev.Data.(Snapshot)
		if !ok {
			t.Fatalf("snapshot payload type: %T", ev.Data)
		}
		if s.CurrentState != "available" || !s.Db || !s.Oidc {
			t.Errorf("snapshot mismatch: %+v", s)
		}
	case <-time.After(time.Second):
		t.Fatal("snapshot not delivered")
	}
}

func TestBrokerPublishFanOut(t *testing.T) {
	b := NewBroker()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, _ := b.Subscribe(ctx)
	c, _ := b.Subscribe(ctx)
	// drain initial snapshots
	<-a
	<-c

	b.Publish(Event{Type: EventJobRun, Data: JobRunEvent{RunID: "r1", JobID: "j1", State: "completed"}})

	for _, ch := range []<-chan Event{a, c} {
		select {
		case ev := <-ch:
			if ev.Type != EventJobRun {
				t.Errorf("type: got %s want job.run", ev.Type)
			}
		case <-time.After(time.Second):
			t.Fatal("event not delivered")
		}
	}
}

func TestBrokerPublishSnapshotSkipsWhenUnchanged(t *testing.T) {
	b := NewBroker()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := b.Subscribe(ctx)
	// initial snapshot
	<-ch

	b.PublishSnapshot(Snapshot{CurrentState: "starting", Db: false, Oidc: false})

	select {
	case ev := <-ch:
		t.Fatalf("unchanged snapshot should not emit, got %s", ev.Type)
	case <-time.After(50 * time.Millisecond):
	}

	b.PublishSnapshot(Snapshot{CurrentState: "available", Db: true, Oidc: true})
	select {
	case ev := <-ch:
		if ev.Type != EventStateChanged {
			t.Errorf("type: got %s want state.changed", ev.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("state.changed not delivered")
	}
}
