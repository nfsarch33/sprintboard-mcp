package sprintboard

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenAndMigrate(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "sprint.db")

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("db file should exist")
	}
}

func TestCreateAndListSprints(t *testing.T) {
	s := testStore(t)

	err := s.CreateSprint(Sprint{
		ID:         "v6080",
		Name:       "Component Registry",
		OwnerAgent: "cursor-parent",
		Theme:      "platform",
	})
	if err != nil {
		t.Fatalf("CreateSprint: %v", err)
	}

	err = s.CreateSprint(Sprint{
		ID:         "v6082",
		Name:       "Command Catalog",
		OwnerAgent: "cursor-parent",
		Theme:      "platform",
	})
	if err != nil {
		t.Fatalf("CreateSprint 2: %v", err)
	}

	sprints, err := s.ListSprints()
	if err != nil {
		t.Fatalf("ListSprints: %v", err)
	}
	if len(sprints) != 2 {
		t.Errorf("expected 2 sprints, got %d", len(sprints))
	}
}

func TestSprintReadsTolerateNullOwnerAgent(t *testing.T) {
	s := testStore(t)
	if err := s.CreateSprint(Sprint{
		ID:     "v7101",
		Name:   "Mem0 Production Gate",
		Status: SprintPlanned,
		Theme:  "coordination",
	}); err != nil {
		t.Fatalf("CreateSprint: %v", err)
	}
	if _, err := s.db.Exec(`UPDATE sprints SET owner_agent = NULL WHERE id = ?`, "v7101"); err != nil {
		t.Fatalf("force NULL owner_agent: %v", err)
	}

	sp, err := s.GetSprint("v7101")
	if err != nil {
		t.Fatalf("GetSprint with NULL owner_agent: %v", err)
	}
	if sp.OwnerAgent != "" {
		t.Fatalf("expected empty owner for NULL owner_agent, got %q", sp.OwnerAgent)
	}

	sprints, err := s.ListSprints()
	if err != nil {
		t.Fatalf("ListSprints with NULL owner_agent: %v", err)
	}
	if len(sprints) != 1 || sprints[0].OwnerAgent != "" {
		t.Fatalf("unexpected sprint list result: %+v", sprints)
	}
}

func TestGetSprint(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "v6080", Name: "Test Sprint", Status: SprintActive})

	sp, err := s.GetSprint("v6080")
	if err != nil {
		t.Fatalf("GetSprint: %v", err)
	}
	if sp.Name != "Test Sprint" {
		t.Errorf("got name %q", sp.Name)
	}
	if sp.Status != SprintActive {
		t.Errorf("got status %q", sp.Status)
	}
}

func TestGetSprintNotFound(t *testing.T) {
	s := testStore(t)
	_, err := s.GetSprint("missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateAndListTickets(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "v6080", Name: "Sprint"})

	err := s.CreateTicket(Ticket{
		ID:       "t-001",
		SprintID: "v6080",
		Title:    "Implement component registry",
		Status:   StatusInProgress,
		Priority: 3,
	})
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}

	err = s.CreateTicket(Ticket{
		ID:       "t-002",
		SprintID: "v6080",
		Title:    "Write tests",
		Status:   StatusBacklog,
		Priority: 1,
	})
	if err != nil {
		t.Fatalf("CreateTicket 2: %v", err)
	}

	tickets, err := s.ListTickets("v6080")
	if err != nil {
		t.Fatalf("ListTickets: %v", err)
	}
	if len(tickets) != 2 {
		t.Errorf("expected 2 tickets, got %d", len(tickets))
	}
	if tickets[0].Priority < tickets[1].Priority {
		t.Error("tickets should be ordered by priority DESC")
	}
}

func TestListTicketsAll(t *testing.T) {
	s := testStore(t)
	s.CreateTicket(Ticket{ID: "t-1", Title: "A"})
	s.CreateTicket(Ticket{ID: "t-2", Title: "B"})

	all, err := s.ListTickets("")
	if err != nil {
		t.Fatalf("ListTickets all: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2, got %d", len(all))
	}
}

func TestTicketReadsTolerateNullableTextColumns(t *testing.T) {
	s := testStore(t)
	if err := s.CreateTicket(Ticket{ID: "t-null", Title: "Nullable ticket", Status: StatusBacklog}); err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	if _, err := s.db.Exec(`UPDATE tickets SET sprint_id = NULL, description = NULL, owner_agent = NULL, acceptance_criteria = NULL, handoff_doc_path = NULL WHERE id = ?`, "t-null"); err != nil {
		t.Fatalf("force nullable ticket columns: %v", err)
	}

	ticket, err := s.GetTicket("t-null")
	if err != nil {
		t.Fatalf("GetTicket with nullable text columns: %v", err)
	}
	if ticket.OwnerAgent != "" || ticket.Description != "" || ticket.AcceptanceCriteria != "" || ticket.HandoffDocPath != "" {
		t.Fatalf("expected nullable text fields to read as empty strings, got %+v", ticket)
	}

	tickets, err := s.ListTickets("")
	if err != nil {
		t.Fatalf("ListTickets with nullable text columns: %v", err)
	}
	if len(tickets) != 1 || tickets[0].OwnerAgent != "" {
		t.Fatalf("unexpected ticket list result: %+v", tickets)
	}
}

func TestUpdateTicket(t *testing.T) {
	s := testStore(t)
	s.CreateTicket(Ticket{ID: "t-1", Title: "Task", Status: StatusBacklog})

	err := s.UpdateTicket("t-1", StatusInProgress, "cursor-parent", "starting work")
	if err != nil {
		t.Fatalf("UpdateTicket: %v", err)
	}

	tickets, _ := s.ListTickets("")
	if tickets[0].Status != StatusInProgress {
		t.Errorf("expected in_progress, got %q", tickets[0].Status)
	}
}

func TestUpdateTicketNotFound(t *testing.T) {
	s := testStore(t)
	err := s.UpdateTicket("ghost", StatusDone, "agent", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAssignTicket(t *testing.T) {
	s := testStore(t)
	s.CreateTicket(Ticket{ID: "t-1", Title: "Task"})

	err := s.AssignTicket("t-1", "codex-ec")
	if err != nil {
		t.Fatalf("AssignTicket: %v", err)
	}

	tickets, _ := s.ListTickets("")
	if tickets[0].OwnerAgent != "codex-ec" {
		t.Errorf("expected codex-ec, got %q", tickets[0].OwnerAgent)
	}
}

func TestAssignTicketNotFound(t *testing.T) {
	s := testStore(t)
	err := s.AssignTicket("ghost", "agent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateAndListHandoffs(t *testing.T) {
	s := testStore(t)
	s.CreateTicket(Ticket{ID: "t-1", Title: "Task"})

	err := s.CreateHandoff(Handoff{
		TicketID:    "t-1",
		FromAgent:   "cursor-parent",
		ToAgent:     "codex-ec",
		ContextPath: "session-handoffs/2026-05-18-handoff.md",
		CreatedAt:   time.Now(),
	})
	if err != nil {
		t.Fatalf("CreateHandoff: %v", err)
	}

	handoffs, err := s.ListHandoffs("t-1")
	if err != nil {
		t.Fatalf("ListHandoffs: %v", err)
	}
	if len(handoffs) != 1 {
		t.Errorf("expected 1, got %d", len(handoffs))
	}
	if handoffs[0].ToAgent != "codex-ec" {
		t.Errorf("expected codex-ec, got %q", handoffs[0].ToAgent)
	}
}

func TestTransitionsRecorded(t *testing.T) {
	s := testStore(t)
	s.CreateTicket(Ticket{ID: "t-1", Title: "Task", Status: StatusBacklog})
	s.UpdateTicket("t-1", StatusInProgress, "cursor-parent", "starting")
	s.UpdateTicket("t-1", StatusDone, "cursor-parent", "done")

	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM ticket_transitions WHERE ticket_id = ?`, "t-1").Scan(&count)
	if count != 2 {
		t.Errorf("expected 2 transitions, got %d", count)
	}
}

func TestDefaultDBPath(t *testing.T) {
	path := DefaultDBPath()
	if path == "" {
		t.Fatal("DefaultDBPath should not be empty")
	}
	if !filepath.IsAbs(path) {
		t.Errorf("expected absolute path, got %q", path)
	}
}
