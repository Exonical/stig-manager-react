package jobs

import "testing"

func TestRegistryLookup(t *testing.T) {
	r := NewBuiltinRegistry()

	for _, want := range []string{"noop", "ping", "fail"} {
		if _, ok := r.Lookup(want); !ok {
			t.Errorf("built-in task %q missing from registry", want)
		}
	}
	if _, ok := r.Lookup("PING"); !ok {
		t.Errorf("registry should be case-insensitive on lookup")
	}
	if _, ok := r.Lookup("definitely-not-a-task"); ok {
		t.Errorf("unknown task should not be found")
	}
}

func TestRegistryDuplicatePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Register should panic on duplicate name")
		}
	}()
	r := NewRegistry()
	r.Register(Definition{Name: "dup"})
	r.Register(Definition{Name: "dup"})
}

func TestRegistryAll(t *testing.T) {
	r := NewBuiltinRegistry()
	defs := r.All()
	if len(defs) < 3 {
		t.Fatalf("expected at least 3 built-ins, got %d", len(defs))
	}
}
