package sprintboard

import (
	"errors"
	"testing"
	"time"
)

// v18860-1: the renew store primitive. The claimant and in-progress guards
// ride in the UPDATE's WHERE clause, so a renew racing a complete or a sweep
// cannot resurrect either side.

func TestRenewClaim_RefreshesLease(t *testing.T) {
	s := newTestStoreForTenants(t)
	if err := s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "t", Status: StatusReady}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := s.ClaimTicket("T1", "agent-a"); err != nil {
		t.Fatalf("claim: %v", err)
	}
	// Age the claim past the sweeper window.
	old := time.Now().UTC().Add(-time.Hour)
	if _, err := s.db.Exec(`UPDATE tickets SET claimed_at = ? WHERE id = ?`, formatTime(old), "T1"); err != nil {
		t.Fatalf("age claim: %v", err)
	}
	stale, err := s.StaleInProgress(30 * time.Minute)
	if err != nil || len(stale) != 1 {
		t.Fatalf("precondition: stale=%d err=%v, want the aged claim visible", len(stale), err)
	}

	if _, err := s.RenewClaim("T1", "agent-a"); err != nil {
		t.Fatalf("renew by holder: %v", err)
	}
	stale, err = s.StaleInProgress(30 * time.Minute)
	if err != nil {
		t.Fatalf("stale after renew: %v", err)
	}
	if len(stale) != 0 {
		t.Fatalf("renewed claim is still stale: %+v", stale)
	}
}

func TestRenewClaim_WrongAgentOrNotInProgressRejected(t *testing.T) {
	s := newTestStoreForTenants(t)
	if err := s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "t", Status: StatusReady}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := s.ClaimTicket("T1", "agent-a"); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if _, err := s.RenewClaim("T1", "agent-b"); !errors.Is(err, ErrTicketNotClaimedBy) {
		t.Fatalf("renew by non-holder = %v, want ErrTicketNotClaimedBy", err)
	}
	if _, err := s.ClaimTicket("T1", "agent-a"); err == nil {
		t.Fatal("double claim should not succeed")
	}
	if err := s.CompleteTicket("T1", "agent-a", "evidence", "", ""); err != nil {
		t.Fatalf("complete: %v", err)
	}
	// done is not in_progress: even the completing agent cannot renew a
	// closed ticket back to liveness.
	if _, err := s.RenewClaim("T1", "agent-a"); !errors.Is(err, ErrTicketNotClaimedBy) {
		t.Fatalf("renew of completed ticket = %v, want ErrTicketNotClaimedBy", err)
	}
}

func TestRenewClaim_SweeperReleasesUnrenewedClaims(t *testing.T) {
	s := newTestStoreForTenants(t)
	if err := s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "t", Status: StatusReady}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.CreateTicket(Ticket{ID: "T2", SprintID: "S1", Title: "t2", Status: StatusReady}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := s.ClaimTicket("T1", "agent-a"); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if _, err := s.ClaimTicket("T2", "agent-b"); err != nil {
		t.Fatalf("claim: %v", err)
	}
	old := formatTime(time.Now().UTC().Add(-time.Hour))
	if _, err := s.db.Exec(`UPDATE tickets SET claimed_at = ? WHERE id IN ('T1','T2')`, old); err != nil {
		t.Fatalf("age claims: %v", err)
	}
	// T1 renews; T2 does not.
	if _, err := s.RenewClaim("T1", "agent-a"); err != nil {
		t.Fatalf("renew: %v", err)
	}
	released, err := s.ReleaseStaleClaims(30 * time.Minute)
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if released != 1 {
		t.Fatalf("released = %d, want exactly 1 (the unrenewed claim)", released)
	}
	tk, err := s.GetTicket("T2")
	if err != nil {
		t.Fatalf("get T2: %v", err)
	}
	if tk.Status != StatusReady {
		t.Fatalf("T2 status = %q, want ready after sweep", tk.Status)
	}
	if tk, _ = s.GetTicket("T1"); tk.Status != StatusInProgress {
		t.Fatalf("T1 status = %q, want still in_progress (it renewed)", tk.Status)
	}
}
