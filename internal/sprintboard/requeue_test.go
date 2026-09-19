package sprintboard

import (
	"errors"
	"testing"
)

// v18850: RequeueTicket is the ONLY way back from a terminal status. It exists
// so the terminal guard on ClaimTicket (see claiming_test.go) does not make a
// wrongly-closed ticket unfixable: a human reopens it with an actor and a
// reason on the audit trail, and the ticket returns to ready for any agent.
func TestRequeueTicket_FromTerminal(t *testing.T) {
	for _, tc := range []struct {
		name       string
		closeFn    func(s *Store)
		wantStatus TicketStatus
	}{
		{
			name: "done by agent",
			closeFn: func(s *Store) {
				if _, err := s.ClaimTicket("T1", "agent-a"); err != nil {
					t.Fatalf("claim: %v", err)
				}
				if err := s.CompleteTicket("T1", "agent-a", "evidence", "", ""); err != nil {
					t.Fatalf("complete: %v", err)
				}
			},
			wantStatus: StatusDone,
		},
		{
			name: "resolved by human",
			closeFn: func(s *Store) {
				if _, err := s.ResolveTicket("T1", "operator", "won't do"); err != nil {
					t.Fatalf("resolve: %v", err)
				}
			},
			wantStatus: StatusResolvedByHuman,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testStore(t)
			s.CreateSprint(Sprint{ID: "S1", Name: "test"})
			s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "task", Status: StatusReady})
			tc.closeFn(s)

			got, err := s.RequeueTicket("T1", "operator", "wrong fix, run again")
			if err != nil {
				t.Fatalf("RequeueTicket: %v", err)
			}
			if got.Status != StatusReady {
				t.Fatalf("status after requeue = %q, want %q", got.Status, StatusReady)
			}
			if got.ClaimedBy != "" {
				t.Fatalf("claimed_by after requeue = %q, want cleared", got.ClaimedBy)
			}

			// The ticket is claimable again by a different agent.
			if _, err := s.ClaimTicket("T1", "agent-b"); err != nil {
				t.Fatalf("claim after requeue: %v", err)
			}

			var fromStatus, toStatus, note string
			if err := s.db.QueryRow(
				`SELECT from_status, to_status, note FROM ticket_transitions
				 WHERE ticket_id = 'T1' AND to_status = ?`, StatusReady,
			).Scan(&fromStatus, &toStatus, &note); err != nil {
				t.Fatalf("read requeue transition: %v", err)
			}
			if fromStatus != string(tc.wantStatus) {
				t.Fatalf("requeue from_status = %q, want the real previous status %q", fromStatus, tc.wantStatus)
			}
			if note == "" || !containsStr(note, "wrong fix") {
				t.Fatalf("requeue note = %q, want it to carry the reason", note)
			}
		})
	}
}

func TestRequeueTicket_NotTerminalRefused(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "task", Status: StatusReady})

	if _, err := s.RequeueTicket("T1", "operator", "reason"); !errors.Is(err, ErrTicketNotTerminal) {
		t.Fatalf("requeue of a ready ticket: err = %v, want ErrTicketNotTerminal", err)
	}
	got, err := s.GetTicket("T1")
	if err != nil {
		t.Fatalf("GetTicket: %v", err)
	}
	if got.Status != StatusReady {
		t.Fatalf("status mutated to %q; a non-terminal ticket must be untouched", got.Status)
	}
}

func TestRequeueTicket_Validation(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "task", Status: StatusDone})

	if _, err := s.RequeueTicket("T-missing", "operator", "reason"); !errors.Is(err, ErrTicketNotFound) {
		t.Fatalf("requeue of missing ticket: err = %v, want ErrTicketNotFound", err)
	}
	if _, err := s.RequeueTicket("T1", "", "reason"); err == nil {
		t.Fatal("requeue without actor accepted; actor is mandatory")
	}
	if _, err := s.RequeueTicket("T1", "operator", ""); err == nil {
		t.Fatal("requeue without reason accepted; reason is mandatory")
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
