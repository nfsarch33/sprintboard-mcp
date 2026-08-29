package sprintboard

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestPublishHandoff_PersistsBranchOnTicket(t *testing.T) {
	s := testStore(t)

	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "task"})

	id, err := s.PublishHandoff(CoordinationHandoff{
		TicketID:  "T1",
		FromAgent: "cursor-parent",
		ToAgent:   "claude-code",
		Summary:   "Sprint v5026 ready for pickup",
		Branch:    "feat/v5026-pickup",
	})
	if err != nil {
		t.Fatalf("PublishHandoff: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero handoff ID")
	}

	got, err := s.GetTicket("T1")
	if err != nil {
		t.Fatalf("GetTicket: %v", err)
	}
	if got.Branch != "feat/v5026-pickup" {
		t.Errorf("branch = %q, want feat/v5026-pickup", got.Branch)
	}
}

func TestPublishHandoff(t *testing.T) {
	s := testStore(t)

	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "task"})

	id, err := s.PublishHandoff(CoordinationHandoff{
		TicketID:  "T1",
		FromAgent: "cursor-parent",
		ToAgent:   "claude-code",
		Summary:   "Sprint v5026 ready for pickup",
	})
	if err != nil {
		t.Fatalf("PublishHandoff: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero handoff ID")
	}
}

func TestSubscribeHandoffs_FiltersByAgent(t *testing.T) {
	s := testStore(t)

	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "task1"})
	s.CreateTicket(Ticket{ID: "T2", SprintID: "S1", Title: "task2"})

	s.PublishHandoff(CoordinationHandoff{
		TicketID: "T1", FromAgent: "cursor-parent", ToAgent: "claude-code",
		Summary: "for claude",
	})
	s.PublishHandoff(CoordinationHandoff{
		TicketID: "T2", FromAgent: "cursor-parent", ToAgent: "codex",
		Summary: "for codex",
	})

	since := time.Now().Add(-1 * time.Hour)
	handoffs, err := s.SubscribeHandoffs("claude-code", since)
	if err != nil {
		t.Fatalf("SubscribeHandoffs: %v", err)
	}
	if len(handoffs) != 1 {
		t.Errorf("got %d handoffs for claude-code, want 1", len(handoffs))
	}
	if len(handoffs) > 0 && handoffs[0].Summary != "for claude" {
		t.Errorf("summary = %q, want 'for claude'", handoffs[0].Summary)
	}
}

func TestSubscribeHandoffs_FiltersBySince(t *testing.T) {
	s := testStore(t)

	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "task"})

	s.PublishHandoff(CoordinationHandoff{
		TicketID: "T1", FromAgent: "cursor-parent", ToAgent: "claude-code",
		Summary: "recent handoff",
	})

	future := time.Now().Add(1 * time.Hour)
	handoffs, _ := s.SubscribeHandoffs("claude-code", future)
	if len(handoffs) != 0 {
		t.Errorf("got %d handoffs with future 'since', want 0", len(handoffs))
	}
}

// TestPublishHandoff_MakesNoOutboundRequest replaces the two tests that used
// to exercise the removed external-memory bridge. It is the regression guard
// for that removal: publishing a handoff must touch the database and nothing
// else. If a bridge is ever reintroduced as a hidden side effect of the
// insert, this fails.
func TestPublishHandoff_MakesNoOutboundRequest(t *testing.T) {
	var called int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&called, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Set every variable the removed bridge used to read. A reintroduced
	// bridge configured the old way would fire at this server.
	t.Setenv("MEM0_BASE_URL", server.URL)
	t.Setenv("MEM0_API_KEY", "not-a-real-key")
	t.Setenv("MEM0_TIMEOUT", "90s")
	t.Setenv("ENGRAM_BASE_URL", server.URL)

	s := testStore(t)
	if err := s.CreateSprint(Sprint{ID: "S1", Name: "test"}); err != nil {
		t.Fatalf("CreateSprint: %v", err)
	}
	if err := s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "task"}); err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}

	id, err := s.PublishHandoff(CoordinationHandoff{
		TicketID:  "T1",
		FromAgent: "agent-a",
		ToAgent:   "agent-b",
		Summary:   "handoff summary",
	})
	if err != nil {
		t.Fatalf("PublishHandoff: %v", err)
	}
	if id == 0 {
		t.Error("PublishHandoff returned id 0")
	}

	if n := atomic.LoadInt32(&called); n != 0 {
		t.Errorf("PublishHandoff made %d outbound HTTP request(s); it must only write to the store", n)
	}

	// Positive control: the durable write still happened. Without this, the
	// assertion above would also pass if PublishHandoff did nothing at all.
	got, err := s.SubscribeHandoffs("agent-b", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("SubscribeHandoffs: %v", err)
	}
	if len(got) != 1 || got[0].Summary != "handoff summary" {
		t.Fatalf("handoff not persisted: %+v", got)
	}
}
