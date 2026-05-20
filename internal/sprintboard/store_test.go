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
