package sprintboard

import (
	"errors"
	"testing"
	"time"
)

func TestClaimTicket_Success(t *testing.T) {
	s := testStore(t)

	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "task", Status: StatusReady})

	result, err := s.ClaimTicket("T1", "cursor-parent")
	if err != nil {
		t.Fatalf("ClaimTicket: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}
	if result.ClaimedBy != "cursor-parent" {
		t.Errorf("claimed_by = %q, want cursor-parent", result.ClaimedBy)
	}
}

func TestClaimTicket_AtomicConflict(t *testing.T) {
	s := testStore(t)

	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "task", Status: StatusReady})

	s.ClaimTicket("T1", "cursor-parent")

	result, err := s.ClaimTicket("T1", "claude-code")
	if err != nil {
		t.Fatalf("ClaimTicket: %v", err)
	}
	if result.Success {
		t.Fatal("expected conflict")
	}
	if result.ConflictBy != "claude-code" {
		t.Errorf("conflict_by = %q, want claude-code", result.ConflictBy)
	}
	if result.ClaimedBy != "cursor-parent" {
		t.Errorf("claimed_by = %q, want cursor-parent", result.ClaimedBy)
	}
}

func TestClaimTicket_SameAgentReclaimOK(t *testing.T) {
	s := testStore(t)

	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "task", Status: StatusReady})

	s.ClaimTicket("T1", "cursor-parent")
	result, err := s.ClaimTicket("T1", "cursor-parent")
	if err != nil {
		t.Fatalf("re-claim: %v", err)
	}
	if !result.Success {
		t.Fatal("same agent should be able to reclaim")
	}
}

func TestCompleteTicket_WithEvidence(t *testing.T) {
	s := testStore(t)

	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "task", Status: StatusReady})
	s.ClaimTicket("T1", "cursor-parent")

	err := s.CompleteTicket("T1", "cursor-parent", "SHA:abc123, 5/5 tests pass", "", "")
	if err != nil {
		t.Fatalf("CompleteTicket: %v", err)
	}

	ticket, _ := s.GetTicket("T1")
	if ticket.Status != StatusDone {
		t.Errorf("status = %q, want done", ticket.Status)
	}
}

func TestCompleteTicket_WrongAgent(t *testing.T) {
	s := testStore(t)

	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "task", Status: StatusReady})
	s.ClaimTicket("T1", "cursor-parent")

	err := s.CompleteTicket("T1", "claude-code", "evidence", "", "")
	if err == nil {
		t.Fatal("expected error for wrong agent completing")
	}
}

func TestReleaseStaleClaims(t *testing.T) {
	s := testStore(t)

	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "task", Status: StatusReady})
	s.ClaimTicket("T1", "cursor-parent")

	staleTime := time.Now().Add(-31 * time.Minute)
	s.db.Exec(`UPDATE tickets SET claimed_at = ? WHERE id = ?`,
		formatTime(staleTime), "T1")

	released, err := s.ReleaseStaleClaims(30 * time.Minute)
	if err != nil {
		t.Fatalf("ReleaseStaleClaims: %v", err)
	}
	if released != 1 {
		t.Errorf("released = %d, want 1", released)
	}

	ticket, _ := s.GetTicket("T1")
	if ticket.Status != StatusReady {
		t.Errorf("status = %q, want ready after release", ticket.Status)
	}
}

func TestReleaseNullClaims(t *testing.T) {
	s := testStore(t)

	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	// Ticket with in_progress status but no claimed_by (stale/corrupt state)
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "task", Status: StatusInProgress})
	// Ticket with in_progress and valid claimed_by -- must NOT be released
	s.CreateTicket(Ticket{ID: "T2", SprintID: "S1", Title: "task2", Status: StatusInProgress})
	s.ClaimTicket("T2", "cursor-parent")

	released, err := s.ReleaseNullClaims()
	if err != nil {
		t.Fatalf("ReleaseNullClaims: %v", err)
	}
	if released != 1 {
		t.Errorf("released = %d, want 1 (only T1)", released)
	}

	t1, _ := s.GetTicket("T1")
	if t1.Status != StatusBacklog {
		t.Errorf("T1 status = %q, want backlog", t1.Status)
	}

	t2, _ := s.GetTicket("T2")
	if t2.Status != StatusInProgress {
		t.Errorf("T2 status = %q, want in_progress (claimed by cursor-parent)", t2.Status)
	}
}

// v18850: the claim path must refuse terminal tickets and record the REAL
// previous status. Before this, a done or resolved ticket whose claimed_by was
// NULL (or matched the caller) could be claimed again -- resurrecting finished
// work -- and the audit row said "ready -> in_progress" whatever the truth.
func TestClaimTicket_TerminalTicketRefused(t *testing.T) {
	for _, status := range []TicketStatus{StatusDone, StatusResolvedByHuman} {
		s := testStore(t)
		s.CreateSprint(Sprint{ID: "S1", Name: "test"})
		s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "task", Status: status})

		result, err := s.ClaimTicket("T1", "agent-a")
		if !errors.Is(err, ErrTicketTerminal) {
			t.Fatalf("claim of %s ticket: err = %v, want ErrTicketTerminal", status, err)
		}
		if result.Success {
			t.Fatalf("claim of %s ticket reported success", status)
		}

		got, err := s.GetTicket("T1")
		if err != nil {
			t.Fatalf("GetTicket: %v", err)
		}
		if got.Status != status {
			t.Fatalf("ticket status mutated to %q, want unchanged %q", got.Status, status)
		}

		var n int
		s.db.QueryRow(`SELECT COUNT(*) FROM ticket_transitions WHERE ticket_id = 'T1'`).Scan(&n)
		if n != 0 {
			t.Fatalf("claim of %s ticket wrote %d transition row(s); audit trail must stay empty", status, n)
		}
	}
}

// Even the agent that finished the work cannot re-claim it: done is done, and
// the way back to ready is the requeue route with an actor and a reason.
func TestClaimTicket_SameAgentTerminalRefused(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "task", Status: StatusReady})
	if _, err := s.ClaimTicket("T1", "agent-a"); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if err := s.CompleteTicket("T1", "agent-a", "evidence", "", ""); err != nil {
		t.Fatalf("complete: %v", err)
	}

	if _, err := s.ClaimTicket("T1", "agent-a"); !errors.Is(err, ErrTicketTerminal) {
		t.Fatalf("same-agent re-claim of done ticket: err = %v, want ErrTicketTerminal", err)
	}
}

// The transition row must carry the ticket's actual previous status, not a
// hardcoded "ready": the audit trail is the board's history, and a backlog
// claim recorded as ready→in_progress never happened.
func TestClaimTicket_HonestPreviousStatus(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "task", Status: StatusBacklog})

	if _, err := s.ClaimTicket("T1", "agent-a"); err != nil {
		t.Fatalf("claim: %v", err)
	}

	var fromStatus, toStatus string
	err := s.db.QueryRow(
		`SELECT from_status, to_status FROM ticket_transitions WHERE ticket_id = 'T1'`,
	).Scan(&fromStatus, &toStatus)
	if err != nil {
		t.Fatalf("read transition: %v", err)
	}
	if fromStatus != string(StatusBacklog) {
		t.Fatalf("from_status = %q, want %q (the real previous status)", fromStatus, StatusBacklog)
	}
	if toStatus != string(StatusInProgress) {
		t.Fatalf("to_status = %q, want %q", toStatus, StatusInProgress)
	}
}
