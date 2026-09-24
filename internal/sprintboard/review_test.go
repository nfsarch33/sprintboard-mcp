package sprintboard

import (
	"errors"
	"testing"
	"time"
)

// A change waiting on review keeps its claim through the stale-claim sweep;
// the reviewer's rework verb hands it back with a fresh lease.

func reviewFixture(t *testing.T, ids ...string) *Store {
	t.Helper()
	s := newTestStoreForTenants(t)
	for _, id := range ids {
		if err := s.CreateTicket(Ticket{ID: id, SprintID: "S1", Title: id, Status: StatusReady}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
		if _, err := s.ClaimTicket(id, "agent-a"); err != nil {
			t.Fatalf("claim %s: %v", id, err)
		}
	}
	return s
}

func ageClaims(t *testing.T, s *Store, d time.Duration) {
	t.Helper()
	if _, err := s.db.Exec(`UPDATE tickets SET claimed_at = ?`, formatTime(time.Now().Add(-d))); err != nil {
		t.Fatalf("age claims: %v", err)
	}
}

func transitionCount(t *testing.T, s *Store, id string, to TicketStatus) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM ticket_transitions WHERE ticket_id = ? AND to_status = ?`, id, to).Scan(&n); err != nil {
		t.Fatalf("count transitions: %v", err)
	}
	return n
}

func TestSubmitForReview_SweeperKeepsReviewClaims(t *testing.T) {
	s := reviewFixture(t, "IN-REVIEW", "IDLE")
	if err := s.SubmitForReview("IN-REVIEW", "agent-a", "change is up"); err != nil {
		t.Fatalf("submit: %v", err)
	}
	ageClaims(t, s, 2*time.Hour)

	stale, err := s.StaleInProgress(30 * time.Minute)
	if err != nil {
		t.Fatalf("stale view: %v", err)
	}
	if len(stale) != 1 || stale[0].ID != "IDLE" {
		t.Fatalf("stale view = %+v, want only the idle in_progress claim", stale)
	}
	released, err := s.ReleaseStaleClaims(30 * time.Minute)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	// Positive control: the idle claim of the same age IS released.
	if released != 1 {
		t.Fatalf("released = %d, want exactly 1 (the idle claim)", released)
	}
	tk, err := s.GetTicket("IN-REVIEW")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if tk.Status != StatusReview || tk.ClaimedBy != "agent-a" {
		t.Fatalf("in-review ticket after sweep = %q claimed by %q, want review claimed by agent-a", tk.Status, tk.ClaimedBy)
	}
	if tk, _ = s.GetTicket("IDLE"); tk.Status != StatusReady {
		t.Fatalf("idle ticket after sweep = %q, want ready", tk.Status)
	}
}

func TestSubmitForReview_GuardsAndIdempotence(t *testing.T) {
	s := reviewFixture(t, "T1")
	if err := s.CreateTicket(Ticket{ID: "OPEN", SprintID: "S1", Title: "open", Status: StatusReady}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.SubmitForReview("T1", "agent-b", ""); !errors.Is(err, ErrTicketNotClaimedBy) {
		t.Fatalf("submit by non-holder = %v, want ErrTicketNotClaimedBy", err)
	}
	if err := s.SubmitForReview("OPEN", "agent-a", ""); !errors.Is(err, ErrTicketNotClaimedBy) {
		t.Fatalf("submit of unclaimed ticket = %v, want ErrTicketNotClaimedBy", err)
	}
	if err := s.SubmitForReview("", "agent-a", ""); err == nil {
		t.Fatal("submit without ticket id succeeded")
	}
	if err := s.SubmitForReview("T1", "agent-a", "rev 1"); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if err := s.SubmitForReview("T1", "agent-a", "rev 2"); err != nil {
		t.Fatalf("resubmit by holder = %v, want idempotent success", err)
	}
	if n := transitionCount(t, s, "T1", StatusReview); n != 1 {
		t.Fatalf("review transitions = %d, want 1 (resubmit adds none)", n)
	}
	// A renew is an in_progress verb; a ticket in review has nothing to renew.
	if _, err := s.RenewClaim("T1", "agent-a"); !errors.Is(err, ErrTicketNotClaimedBy) {
		t.Fatalf("renew in review = %v, want ErrTicketNotClaimedBy", err)
	}
}

func TestReturnToWork_RestartsTheLease(t *testing.T) {
	s := reviewFixture(t, "T1")
	if err := s.ReturnToWork("T1", "reviewer", "not in review yet"); !errors.Is(err, ErrTicketNotInReview) {
		t.Fatalf("rework of in_progress ticket = %v, want ErrTicketNotInReview", err)
	}
	if err := s.SubmitForReview("T1", "agent-a", ""); err != nil {
		t.Fatalf("submit: %v", err)
	}
	ageClaims(t, s, 2*time.Hour)
	if err := s.ReturnToWork("T1", "", "x"); err == nil {
		t.Fatal("rework without actor succeeded")
	}
	if err := s.ReturnToWork("T1", "reviewer", "tests fail on the merge result"); err != nil {
		t.Fatalf("rework: %v", err)
	}
	tk, err := s.GetTicket("T1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if tk.Status != StatusInProgress || tk.ClaimedBy != "agent-a" {
		t.Fatalf("after rework = %q claimed by %q, want in_progress claimed by agent-a", tk.Status, tk.ClaimedBy)
	}
	// The lease restarted: the two-hour-old claim is no longer stale.
	if stale, _ := s.StaleInProgress(30 * time.Minute); len(stale) != 0 {
		t.Fatalf("reworked claim is stale: %+v", stale)
	}
	if n := transitionCount(t, s, "T1", StatusInProgress); n < 1 {
		t.Fatalf("no review -> in_progress transition recorded")
	}
	if err := s.ReturnToWork("T1", "reviewer", "again"); !errors.Is(err, ErrTicketNotInReview) {
		t.Fatalf("second rework = %v, want ErrTicketNotInReview", err)
	}
}

func TestCompleteTicket_FromReviewRecordsTheRealFromStatus(t *testing.T) {
	s := reviewFixture(t, "T1")
	if err := s.SubmitForReview("T1", "agent-a", ""); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if err := s.CompleteTicket("T1", "agent-a", "merged", "", ""); err != nil {
		t.Fatalf("complete from review: %v", err)
	}
	var from string
	if err := s.db.QueryRow(`SELECT from_status FROM ticket_transitions WHERE ticket_id = ? AND to_status = ?`, "T1", StatusDone).Scan(&from); err != nil {
		t.Fatalf("read done transition: %v", err)
	}
	if TicketStatus(from) != StatusReview {
		t.Fatalf("done transition from = %q, want review", from)
	}
}
