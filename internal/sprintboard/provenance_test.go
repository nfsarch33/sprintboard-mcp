package sprintboard

import (
	"testing"
	"time"
)

func TestMigrateProvenance_Idempotent(t *testing.T) {
	s := testStore(t)
	if err := s.migrateProvenance(); err != nil {
		t.Fatalf("second migrateProvenance: %v", err)
	}
}

func TestCreateTicket_WithProvenance(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "S1", Name: "test"})

	err := s.CreateTicket(Ticket{
		ID: "T-prov", SprintID: "S1", Title: "provenance",
		Branch: "feat/T-prov-scope", PRURL: "https://github.com/nfsarch33/sprintboard-mcp/pull/99",
	})
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}

	got, err := s.GetTicket("T-prov")
	if err != nil {
		t.Fatalf("GetTicket: %v", err)
	}
	if got.Branch != "feat/T-prov-scope" {
		t.Errorf("branch = %q", got.Branch)
	}
	if got.PRURL != "https://github.com/nfsarch33/sprintboard-mcp/pull/99" {
		t.Errorf("pr_url = %q", got.PRURL)
	}
}

func TestCompleteTicket_PersistsProvenance(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "task"})
	s.ClaimTicket("T1", "cursor-parent")

	err := s.CompleteTicket("T1", "cursor-parent", "sha abc tests pass",
		"feat/T1-done", "https://github.com/nfsarch33/sprintboard-mcp/pull/6")
	if err != nil {
		t.Fatalf("CompleteTicket: %v", err)
	}

	got, _ := s.GetTicket("T1")
	if got.Branch != "feat/T1-done" {
		t.Errorf("branch = %q", got.Branch)
	}
	if got.PRURL != "https://github.com/nfsarch33/sprintboard-mcp/pull/6" {
		t.Errorf("pr_url = %q", got.PRURL)
	}
	if got.Status != StatusDone {
		t.Errorf("status = %q", got.Status)
	}
}

func TestPublishHandoff_PersistsBranch(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "task"})

	_, err := s.PublishHandoff(CoordinationHandoff{
		TicketID: "T1", FromAgent: "cursor-parent", ToAgent: "claude-code",
		Summary: "pick up", Branch: "feat/T1-handoff",
	})
	if err != nil {
		t.Fatalf("PublishHandoff: %v", err)
	}

	got, _ := s.GetTicket("T1")
	if got.Branch != "feat/T1-handoff" {
		t.Errorf("branch = %q, want feat/T1-handoff", got.Branch)
	}
}

func TestSetTicketMergedAt(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "task"})

	merged := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	if err := s.SetTicketMergedAt("T1", merged); err != nil {
		t.Fatalf("SetTicketMergedAt: %v", err)
	}

	got, _ := s.GetTicket("T1")
	if got.MergedAt.IsZero() {
		t.Fatal("merged_at not set")
	}
	if !got.MergedAt.Equal(merged) {
		t.Errorf("merged_at = %v, want %v", got.MergedAt, merged)
	}
}
