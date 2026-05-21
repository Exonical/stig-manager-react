package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Exonical/stig-manager-react/api/internal/auth"
	"github.com/Exonical/stig-manager-react/api/internal/store"
)

type fakeRecorder struct {
	mu      sync.Mutex
	entries []store.AuditEntry
	err     error
}

func (f *fakeRecorder) Record(_ context.Context, e store.AuditEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, e)
	return f.err
}

func (f *fakeRecorder) snapshot() []store.AuditEntry {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]store.AuditEntry, len(f.entries))
	copy(out, f.entries)
	return out
}

// wait spins briefly so the goroutine that calls Record gets a chance
// to land before assertions read the recorder buffer.
func wait(t *testing.T, rec *fakeRecorder, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(rec.snapshot()) >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("audit recorder never reached %d entries (have %d)", want, len(rec.snapshot()))
}

func TestSkipsReadMethods(t *testing.T) {
	rec := &fakeRecorder{}
	r := chi.NewRouter()
	r.Use(Middleware(rec, Config{}))
	r.Get("/api/collections", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for _, m := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		req := httptest.NewRequest(m, "/api/collections", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}
	time.Sleep(50 * time.Millisecond)
	if got := len(rec.snapshot()); got != 0 {
		t.Fatalf("expected GET/HEAD/OPTIONS to skip audit, got %d entries", got)
	}
}

func TestSkipsNonAPIPath(t *testing.T) {
	rec := &fakeRecorder{}
	r := chi.NewRouter()
	r.Use(Middleware(rec, Config{}))
	r.Post("/js/Env.js", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/js/Env.js", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	time.Sleep(50 * time.Millisecond)
	if got := len(rec.snapshot()); got != 0 {
		t.Fatalf("non-/api/* path should not be audited, got %d", got)
	}
}

func TestRecordsMutatingRequest(t *testing.T) {
	rec := &fakeRecorder{}
	r := chi.NewRouter()
	r.Use(Middleware(rec, Config{}))
	r.Post("/api/collections", func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		if !bytes.Equal(body, []byte(`{"name":"x"}`)) {
			t.Errorf("downstream handler saw mangled body %q", body)
		}
		w.WriteHeader(http.StatusCreated)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/collections",
		strings.NewReader(`{"name":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	wait(t, rec, 1)
	e := rec.snapshot()[0]
	if e.Method != http.MethodPost {
		t.Errorf("method: got %q", e.Method)
	}
	if e.Path != "/api/collections" {
		t.Errorf("path: got %q", e.Path)
	}
	if e.Status != http.StatusCreated {
		t.Errorf("status: got %d", e.Status)
	}
	if e.Route != "/api/collections" {
		t.Errorf("route: got %q", e.Route)
	}
	if string(e.Payload) != `{"name":"x"}` {
		t.Errorf("payload: got %s", string(e.Payload))
	}
	var meta map[string]any
	if err := json.Unmarshal(e.Metadata, &meta); err != nil {
		t.Fatalf("metadata: %v", err)
	}
	if meta["contentType"] != "application/json" {
		t.Errorf("metadata.contentType: got %v", meta["contentType"])
	}
}

func TestRedactsSensitiveKeys(t *testing.T) {
	rec := &fakeRecorder{}
	r := chi.NewRouter()
	r.Use(Middleware(rec, Config{RedactKeys: []string{"customSecret"}}))
	r.Post("/api/users", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	body := `{"username":"alice","password":"hunter2","nested":{"token":"abc","ok":true},"customSecret":"x"}`
	req := httptest.NewRequest(http.MethodPost, "/api/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	wait(t, rec, 1)
	got := string(rec.snapshot()[0].Payload)
	if strings.Contains(got, "hunter2") || strings.Contains(got, `"abc"`) || strings.Contains(got, `"customSecret":"x"`) {
		t.Fatalf("payload leaked secrets: %s", got)
	}
	if !strings.Contains(got, `"password":"***"`) {
		t.Fatalf("expected password redacted to ***, got %s", got)
	}
	if !strings.Contains(got, `"token":"***"`) {
		t.Fatalf("expected nested token redacted, got %s", got)
	}
}

func TestTruncatesLargeBodies(t *testing.T) {
	rec := &fakeRecorder{}
	r := chi.NewRouter()
	r.Use(Middleware(rec, Config{MaxBodyBytes: 16}))
	r.Post("/api/x", func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		// Downstream still sees the full body.
		if len(body) != 64 {
			t.Errorf("downstream body got %d bytes, want 64", len(body))
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/x",
		bytes.NewReader(bytes.Repeat([]byte("a"), 64)))
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	wait(t, rec, 1)
	e := rec.snapshot()[0]
	if len(e.Payload) != 0 {
		t.Errorf("truncated bodies should not persist payload, got %s", e.Payload)
	}
	var meta map[string]any
	if err := json.Unmarshal(e.Metadata, &meta); err != nil {
		t.Fatalf("metadata: %v", err)
	}
	body, _ := meta["body"].(map[string]any)
	if body == nil || body["truncated"] != true {
		t.Fatalf("expected metadata.body.truncated=true, got %v", meta)
	}
}

func TestCapturesAuthenticatedUser(t *testing.T) {
	rec := &fakeRecorder{}
	r := chi.NewRouter()
	r.Use(Middleware(rec, Config{}))
	r.Post("/api/x", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/api/x", strings.NewReader(`{}`))
	req = req.WithContext(auth.WithUser(req.Context(),
		&auth.User{Subject: "sub-abc", Username: "alice"}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	wait(t, rec, 1)
	e := rec.snapshot()[0]
	if e.Subject != "sub-abc" || e.Username != "alice" {
		t.Fatalf("auth not captured: subject=%q username=%q", e.Subject, e.Username)
	}
}

func TestNilRecorderIsNoop(t *testing.T) {
	r := chi.NewRouter()
	r.Use(Middleware(nil, Config{}))
	r.Post("/api/x", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/api/x", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("nil recorder should pass through, got %d", w.Code)
	}
}
