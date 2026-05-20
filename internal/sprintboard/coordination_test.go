package sprintboard

import (
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
