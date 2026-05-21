package sprintboard

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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

func TestBridgeToMem0UsesMemoriesEndpoint(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/memories" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	t.Setenv("MEM0_BASE_URL", server.URL)
	t.Setenv("MEM0_API_KEY", "")

	err := bridgeToMem0(CoordinationHandoff{
		TicketID:  "T1",
		FromAgent: "codex",
		ToAgent:   "cursor-parent",
		Summary:   "foundation smoke",
	})
	if err != nil {
		t.Fatalf("bridgeToMem0: %v (path %q)", err, gotPath)
	}
	if gotPath != "/memories" {
		t.Fatalf("path = %q, want /memories", gotPath)
	}
}

func TestMem0BridgeTimeoutUsesEnv(t *testing.T) {
	t.Setenv("MEM0_TIMEOUT", "90s")
	if got := mem0BridgeTimeout(); got != 90*time.Second {
		t.Fatalf("timeout = %s, want 90s", got)
	}
}
