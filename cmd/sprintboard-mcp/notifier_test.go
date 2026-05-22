package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/nfsarch33/sprintboard-mcp/internal/mcptelemetry"
	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

// recordedEvent captures a single webhook envelope decoded by the test sink.
type recordedEvent struct {
	Event     string         `json:"event"`
	Payload   map[string]any `json:"payload"`
	EmittedAt string         `json:"emitted_at"`
}

// startSink spins up an httptest server that records every JSON envelope it
// receives. It returns the server, the events slice, and a wait func that
// blocks until at least n envelopes have arrived (or t.Fatal on timeout).
func startSink(t *testing.T) (*httptest.Server, *[]recordedEvent, func(int)) {
	t.Helper()

	var (
		mu     sync.Mutex
		events []recordedEvent
		gotN   = make(chan int, 32)
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var ev recordedEvent
		if err := json.Unmarshal(body, &ev); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		events = append(events, ev)
		count := len(events)
		mu.Unlock()
		gotN <- count
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	wait := func(n int) {
		t.Helper()
		deadline := time.After(2 * time.Second)
		for {
			mu.Lock()
			have := len(events)
			mu.Unlock()
			if have >= n {
				return
			}
			select {
			case <-gotN:
			case <-deadline:
				t.Fatalf("timeout waiting for %d webhook events; have %d", n, have)
			}
		}
	}

	return srv, &events, wait
}

// newServerWithNotifier builds a Server whose notifier targets the given URL.
func newServerWithNotifier(t *testing.T, url string) *Server {
	t.Helper()
	dir := t.TempDir()
	store, err := sprintboard.NewStore(filepath.Join(dir, "sprintboard.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	rec, err := mcptelemetry.New(mcptelemetry.Config{Enabled: false})
	if err != nil {
		t.Fatalf("telemetry.New: %v", err)
	}
	emb := sprintboard.NewEmbedder(sprintboard.EmbedderConfig{Dimension: 8})
	return &Server{
		store:     store,
		agentID:   "test-agent",
		telemetry: rec,
		embedder:  emb,
		notifier:  NewHTTPNotifier(url, 500*time.Millisecond),
	}
}

// TestNotifier_TicketClaimAndComplete drives task_claim and task_complete
// through dispatch and asserts both events arrive at the sink with the
// expected payload shape.
func TestNotifier_TicketClaimAndComplete(t *testing.T) {
	t.Parallel()
	sink, events, wait := startSink(t)
	s := newServerWithNotifier(t, sink.URL)

	if _, isErr := s.dispatch("sprint_create", mustArgs(t, map[string]string{
		"id": "v9100", "name": "Webhook Sprint",
	})); isErr {
		t.Fatal("sprint_create failed")
	}
	if _, isErr := s.dispatch("ticket_create", mustArgs(t, map[string]any{
		"id": "T-WH-1", "sprint_id": "v9100", "title": "webhook ticket",
	})); isErr {
		t.Fatal("ticket_create failed")
	}
	if _, isErr := s.dispatch("agent_register", mustArgs(t, map[string]string{
		"agent_id": "alpha", "surface": "test", "capabilities": "go",
	})); isErr {
		t.Fatal("agent_register failed")
	}

	if out, isErr := s.dispatch("task_claim", mustArgs(t, map[string]string{
		"ticket_id": "T-WH-1", "agent_id": "alpha",
	})); isErr {
		t.Fatalf("task_claim: %s", out)
	}
	if out, isErr := s.dispatch("task_complete", mustArgs(t, map[string]string{
		"ticket_id": "T-WH-1", "agent_id": "alpha", "evidence": "tests green",
	})); isErr {
		t.Fatalf("task_complete: %s", out)
	}

	wait(2)

	got := *events
	if len(got) != 2 {
		t.Fatalf("expected 2 events, got %d: %+v", len(got), got)
	}
	if got[0].Event != "ticket_claimed" {
		t.Errorf("event[0] = %q, want ticket_claimed", got[0].Event)
	}
	if got[1].Event != "ticket_completed" {
		t.Errorf("event[1] = %q, want ticket_completed", got[1].Event)
	}
	if got[1].Payload["ticket_id"] != "T-WH-1" {
		t.Errorf("event[1].payload.ticket_id = %v, want T-WH-1", got[1].Payload["ticket_id"])
	}
	if got[1].Payload["evidence"] != "tests green" {
		t.Errorf("event[1].payload.evidence = %v, want tests green", got[1].Payload["evidence"])
	}
}

// TestNotifier_SprintClose asserts the sprint_close event is emitted with
// the correct sprint_id payload.
func TestNotifier_SprintClose(t *testing.T) {
	t.Parallel()
	sink, events, wait := startSink(t)
	s := newServerWithNotifier(t, sink.URL)

	if _, isErr := s.dispatch("sprint_create", mustArgs(t, map[string]string{
		"id": "v9101", "name": "Close Sprint",
	})); isErr {
		t.Fatal("sprint_create failed")
	}
	if out, isErr := s.dispatch("sprint_close", mustArgs(t, map[string]string{
		"sprint_id": "v9101",
	})); isErr {
		t.Fatalf("sprint_close: %s", out)
	}

	wait(1)
	got := *events
	if len(got) != 1 || got[0].Event != "sprint_closed" {
		t.Fatalf("expected sprint_closed event, got %+v", got)
	}
	if got[0].Payload["sprint_id"] != "v9101" {
		t.Errorf("sprint_id = %v, want v9101", got[0].Payload["sprint_id"])
	}
}

// TestNotifier_NopByDefault asserts the dispatch path tolerates a nil
// notifier (server constructed without webhook config).
func TestNotifier_NopByDefault(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store, err := sprintboard.NewStore(filepath.Join(dir, "sprintboard.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	rec, _ := mcptelemetry.New(mcptelemetry.Config{Enabled: false})
	s := &Server{
		store:     store,
		agentID:   "test-agent",
		telemetry: rec,
		embedder:  sprintboard.NewEmbedder(sprintboard.EmbedderConfig{Dimension: 8}),
		// notifier intentionally nil
	}

	// Setup
	if _, isErr := s.dispatch("sprint_create", mustArgs(t, map[string]string{
		"id": "v9102", "name": "Nop Sprint",
	})); isErr {
		t.Fatal("sprint_create failed")
	}
	if out, isErr := s.dispatch("sprint_close", mustArgs(t, map[string]string{
		"sprint_id": "v9102",
	})); isErr {
		t.Fatalf("sprint_close with nil notifier should succeed: %s", out)
	}
}

// TestHTTPNotifier_EmptyURLIsNop asserts NewHTTPNotifier collapses to the
// no-op implementation when handed an empty URL.
func TestHTTPNotifier_EmptyURLIsNop(t *testing.T) {
	t.Parallel()
	n := NewHTTPNotifier("", 0)
	if _, ok := n.(nopNotifier); !ok {
		t.Errorf("empty URL should yield nopNotifier, got %T", n)
	}
}
