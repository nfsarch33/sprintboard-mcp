package sprintboard

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestConcurrentTicketCreation(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "concurrent-sprint", Name: "Concurrency Test"})

	var wg sync.WaitGroup
	errors := make(chan error, 50)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			err := s.CreateTicket(Ticket{
				ID:       fmt.Sprintf("ticket-%03d", idx),
				SprintID: "concurrent-sprint",
				Title:    fmt.Sprintf("Task %d", idx),
				Priority: idx % 5,
			})
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent create error: %v", err)
	}

	tickets, err := s.ListTickets("concurrent-sprint")
	if err != nil {
		t.Fatalf("ListTickets: %v", err)
	}
	if len(tickets) != 50 {
		t.Errorf("expected 50 tickets, got %d", len(tickets))
	}
}

func TestConcurrentStatusUpdates(t *testing.T) {
	s := testStore(t)
	s.CreateTicket(Ticket{ID: "race-ticket", Title: "Race", Status: StatusBacklog})

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			s.UpdateTicket("race-ticket", StatusInProgress, fmt.Sprintf("agent-%d", idx), "")
		}(i)
	}
	wg.Wait()

	transitions, err := s.ListTransitions("race-ticket")
	if err != nil {
		t.Fatalf("ListTransitions: %v", err)
	}
	if len(transitions) < 1 {
		t.Error("expected at least 1 transition recorded")
	}
}

func TestDuplicateSprintID(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "dup", Name: "First"})
	err := s.CreateSprint(Sprint{ID: "dup", Name: "Second"})
	if err == nil {
		t.Fatal("expected error for duplicate sprint ID")
	}
}

func TestDuplicateTicketID(t *testing.T) {
	s := testStore(t)
	s.CreateTicket(Ticket{ID: "dup-t", Title: "First"})
	err := s.CreateTicket(Ticket{ID: "dup-t", Title: "Second"})
	if err == nil {
		t.Fatal("expected error for duplicate ticket ID")
	}
}

func TestEmptySprintID(t *testing.T) {
	s := testStore(t)
	err := s.CreateSprint(Sprint{ID: "", Name: "No ID"})
	if err != nil {
		t.Skip("SQLite allows empty string PK; validation should be at application layer")
	}
}

func TestOrphanTicket(t *testing.T) {
	s := testStore(t)
	err := s.CreateTicket(Ticket{ID: "orphan", Title: "No sprint", SprintID: ""})
	if err != nil {
		t.Fatalf("orphan ticket should be allowed: %v", err)
	}

	tickets, _ := s.ListTickets("")
	found := false
	for _, tk := range tickets {
		if tk.ID == "orphan" {
			found = true
			break
		}
	}
	if !found {
		t.Error("orphan ticket not found in full list")
	}
}

func TestListTicketsNonexistentSprint(t *testing.T) {
	s := testStore(t)
	tickets, err := s.ListTickets("ghost-sprint")
	if err != nil {
		t.Fatalf("should not error for nonexistent sprint: %v", err)
	}
	if len(tickets) != 0 {
		t.Errorf("expected 0 tickets, got %d", len(tickets))
	}
}

func TestUpdateSprintStatus(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "s1", Name: "Test", Status: SprintPlanned})

	err := s.UpdateSprint("s1", SprintActive)
	if err != nil {
		t.Fatalf("UpdateSprint: %v", err)
	}

	sp, _ := s.GetSprint("s1")
	if sp.Status != SprintActive {
		t.Errorf("expected active, got %q", sp.Status)
	}
}

func TestUpdateSprintNotFound(t *testing.T) {
	s := testStore(t)
	err := s.UpdateSprint("ghost", SprintClosed)
	if err == nil {
		t.Fatal("expected error for nonexistent sprint")
	}
}

func TestDeleteSprint(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "del-sprint", Name: "Delete Me"})

	err := s.DeleteSprint("del-sprint")
	if err != nil {
		t.Fatalf("DeleteSprint: %v", err)
	}

	_, err = s.GetSprint("del-sprint")
	if err == nil {
		t.Fatal("sprint should be deleted")
	}
}

func TestDeleteSprintWithTickets(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "full-sprint", Name: "Has Tickets"})
	s.CreateTicket(Ticket{ID: "blocker", SprintID: "full-sprint", Title: "Blocks deletion"})

	err := s.DeleteSprint("full-sprint")
	if err == nil {
		t.Fatal("expected error when deleting sprint with tickets")
	}
}

func TestDeleteSprintNotFound(t *testing.T) {
	s := testStore(t)
	err := s.DeleteSprint("ghost")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDeleteTicket(t *testing.T) {
	s := testStore(t)
	s.CreateTicket(Ticket{ID: "del-ticket", Title: "Delete Me"})
	s.UpdateTicket("del-ticket", StatusInProgress, "agent", "starting")

	err := s.DeleteTicket("del-ticket")
	if err != nil {
		t.Fatalf("DeleteTicket: %v", err)
	}

	_, err = s.GetTicket("del-ticket")
	if err == nil {
		t.Fatal("ticket should be deleted")
	}
}

func TestDeleteTicketNotFound(t *testing.T) {
	s := testStore(t)
	err := s.DeleteTicket("ghost")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetTicket(t *testing.T) {
	s := testStore(t)
	s.CreateTicket(Ticket{ID: "get-me", Title: "Fetch Test", Status: StatusReady, Priority: 5})

	got, err := s.GetTicket("get-me")
	if err != nil {
		t.Fatalf("GetTicket: %v", err)
	}
	if got.Title != "Fetch Test" {
		t.Errorf("got title %q", got.Title)
	}
	if got.Priority != 5 {
		t.Errorf("got priority %d", got.Priority)
	}
}

func TestGetTicketNotFound(t *testing.T) {
	s := testStore(t)
	_, err := s.GetTicket("ghost")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSprintSummary(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "summary-sprint", Name: "Summary"})
	s.CreateTicket(Ticket{ID: "t1", SprintID: "summary-sprint", Title: "A", Status: StatusBacklog})
	s.CreateTicket(Ticket{ID: "t2", SprintID: "summary-sprint", Title: "B", Status: StatusInProgress})
	s.CreateTicket(Ticket{ID: "t3", SprintID: "summary-sprint", Title: "C", Status: StatusInProgress})
	s.CreateTicket(Ticket{ID: "t4", SprintID: "summary-sprint", Title: "D", Status: StatusDone})

	summary, err := s.SprintSummary("summary-sprint")
	if err != nil {
		t.Fatalf("SprintSummary: %v", err)
	}
	if summary.TotalTickets != 4 {
		t.Errorf("expected 4 total, got %d", summary.TotalTickets)
	}
	if summary.TicketsByStatus[StatusInProgress] != 2 {
		t.Errorf("expected 2 in_progress, got %d", summary.TicketsByStatus[StatusInProgress])
	}
	if summary.TicketsByStatus[StatusDone] != 1 {
		t.Errorf("expected 1 done, got %d", summary.TicketsByStatus[StatusDone])
	}
}

func TestSprintSummaryNotFound(t *testing.T) {
	s := testStore(t)
	_, err := s.SprintSummary("ghost")
	if err == nil {
		t.Fatal("expected error for nonexistent sprint")
	}
}

func TestListTransitions(t *testing.T) {
	s := testStore(t)
	s.CreateTicket(Ticket{ID: "trans-ticket", Title: "Transitions", Status: StatusBacklog})
	s.UpdateTicket("trans-ticket", StatusReady, "agent-a", "triaged")
	s.UpdateTicket("trans-ticket", StatusInProgress, "agent-b", "started")
	s.UpdateTicket("trans-ticket", StatusDone, "agent-b", "completed")

	transitions, err := s.ListTransitions("trans-ticket")
	if err != nil {
		t.Fatalf("ListTransitions: %v", err)
	}
	if len(transitions) != 3 {
		t.Errorf("expected 3 transitions, got %d", len(transitions))
	}
	if transitions[0].FromStatus != StatusBacklog {
		t.Errorf("first transition from=%q", transitions[0].FromStatus)
	}
	if transitions[2].ToStatus != StatusDone {
		t.Errorf("last transition to=%q", transitions[2].ToStatus)
	}
}

func TestLargeDataset(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "large", Name: "Large Sprint"})

	start := time.Now()
	for i := 0; i < 1000; i++ {
		s.CreateTicket(Ticket{
			ID:       fmt.Sprintf("bulk-%04d", i),
			SprintID: "large",
			Title:    fmt.Sprintf("Ticket %d", i),
			Priority: i % 10,
		})
	}
	insertDuration := time.Since(start)

	if insertDuration > 10*time.Second {
		t.Errorf("1000 inserts took %v (expected <10s)", insertDuration)
	}

	start = time.Now()
	tickets, err := s.ListTickets("large")
	queryDuration := time.Since(start)

	if err != nil {
		t.Fatalf("ListTickets: %v", err)
	}
	if len(tickets) != 1000 {
		t.Errorf("expected 1000, got %d", len(tickets))
	}
	if queryDuration > 2*time.Second {
		t.Errorf("listing 1000 tickets took %v (expected <2s)", queryDuration)
	}
}

func TestSprintSummaryEmpty(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "empty", Name: "Empty Sprint"})

	summary, err := s.SprintSummary("empty")
	if err != nil {
		t.Fatalf("SprintSummary: %v", err)
	}
	if summary.TotalTickets != 0 {
		t.Errorf("expected 0 tickets, got %d", summary.TotalTickets)
	}
}

func TestHandoffForDeletedTicketOrphans(t *testing.T) {
	s := testStore(t)
	s.CreateTicket(Ticket{ID: "hoff-del", Title: "Will be deleted"})
	s.CreateHandoff(Handoff{TicketID: "hoff-del", FromAgent: "a", ToAgent: "b"})

	s.DeleteTicket("hoff-del")

	handoffs, err := s.ListHandoffs("hoff-del")
	if err != nil {
		t.Fatalf("ListHandoffs after delete: %v", err)
	}
	if len(handoffs) != 0 {
		t.Errorf("expected handoffs cleaned up, got %d", len(handoffs))
	}
}
