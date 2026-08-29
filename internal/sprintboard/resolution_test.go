package sprintboard

import (
	"errors"
	"testing"
	"time"
)

// escalated builds the state the fleet agent actually leaves behind when it
// gives up on a ticket: claimed by the agent, still in_progress, with the
// failure evidence in a comment and no completion.
func escalated(t *testing.T, s *Store, ticketID, agentID string) {
	t.Helper()
	// Duplicate-sprint errors are expected on the second call within a test;
	// CreateTicket below fails loudly if the sprint genuinely isn't there.
	_ = s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	if err := s.CreateTicket(Ticket{ID: ticketID, SprintID: "S1", Title: "poison", Status: StatusReady}); err != nil {
		t.Fatalf("CreateTicket %s: %v", ticketID, err)
	}
	if _, err := s.ClaimTicket(ticketID, agentID); err != nil {
		t.Fatalf("ClaimTicket %s: %v", ticketID, err)
	}
	if _, err := s.AddTicketComment(ticketID, agentID, "verifier failed; escalating"); err != nil {
		t.Fatalf("AddTicketComment %s: %v", ticketID, err)
	}
}

func TestResolveTicket_ClosesEscalationAndRecordsWhoAndWhy(t *testing.T) {
	s := testStore(t)
	escalated(t, s, "T1", "fleet-agent-01")

	got, err := s.ResolveTicket("T1", "operator", "test artefact; no human action needed")
	if err != nil {
		t.Fatalf("ResolveTicket: %v", err)
	}

	if got.Status != StatusResolvedByHuman {
		t.Errorf("status = %q, want %q", got.Status, StatusResolvedByHuman)
	}
	if got.ResolvedBy != "operator" {
		t.Errorf("resolved_by = %q, want operator", got.ResolvedBy)
	}
	if got.ResolutionReason != "test artefact; no human action needed" {
		t.Errorf("resolution_reason = %q", got.ResolutionReason)
	}
	if got.ResolvedAt.IsZero() {
		t.Error("resolved_at not stamped")
	}

	// The claimer is preserved on purpose: "the agent had this, a person
	// closed it" is the audit trail, and clearing it would erase half.
	if got.ClaimedBy != "fleet-agent-01" {
		t.Errorf("claimed_by = %q, want the escalating agent to survive resolution", got.ClaimedBy)
	}

	// completed_at must stay empty. It feeds SprintSLAs's time-to-complete,
	// and a completion duration for work that was never completed is a lie
	// the SLA report would repeat forever.
	if !got.CompletedAt.IsZero() {
		t.Errorf("completed_at = %v, want zero: the ticket was resolved, not completed", got.CompletedAt)
	}
}

func TestResolveTicket_WritesTransitionWithActorAndReason(t *testing.T) {
	s := testStore(t)
	escalated(t, s, "T1", "fleet-agent-01")

	if _, err := s.ResolveTicket("T1", "operator", "poison ticket, closing"); err != nil {
		t.Fatalf("ResolveTicket: %v", err)
	}

	var from, to, agent, note string
	err := s.db.QueryRow(
		`SELECT from_status, to_status, agent_id, note FROM ticket_transitions
		 WHERE ticket_id = ? ORDER BY id DESC LIMIT 1`, "T1",
	).Scan(&from, &to, &agent, &note)
	if err != nil {
		t.Fatalf("read transition: %v", err)
	}
	if from != string(StatusInProgress) || to != string(StatusResolvedByHuman) {
		t.Errorf("transition %s -> %s, want in_progress -> resolved_by_human", from, to)
	}
	if agent != "operator" || note != "poison ticket, closing" {
		t.Errorf("transition recorded agent=%q note=%q, want the human actor and reason", agent, note)
	}
}

func TestResolveTicket_RequiresActorAndReason(t *testing.T) {
	s := testStore(t)
	escalated(t, s, "T1", "agent-a")

	if _, err := s.ResolveTicket("T1", "", "some reason"); err == nil {
		t.Error("resolved with no actor; an anonymous resolution has no audit value")
	}
	if _, err := s.ResolveTicket("T1", "operator", ""); err == nil {
		t.Error("resolved with no reason; the escalation comment would have no answer")
	}
	// Neither rejection may have closed the ticket.
	got, err := s.GetTicket("T1")
	if err != nil {
		t.Fatalf("GetTicket: %v", err)
	}
	if got.Status != StatusInProgress {
		t.Errorf("status = %q after two rejected resolutions, want in_progress", got.Status)
	}
}

func TestResolveTicket_RefusesTerminalTickets(t *testing.T) {
	s := testStore(t)
	escalated(t, s, "T1", "agent-a")
	escalated(t, s, "T2", "agent-a")

	// Already completed by its agent: resolution must not overwrite a real
	// delivery with a human write-off.
	if err := s.CompleteTicket("T2", "agent-a", "shipped", "", ""); err != nil {
		t.Fatalf("CompleteTicket: %v", err)
	}
	if _, err := s.ResolveTicket("T2", "operator", "closing"); !errors.Is(err, ErrTicketTerminal) {
		t.Errorf("resolve of a done ticket = %v, want ErrTicketTerminal", err)
	}

	// Already resolved: the second call must not replace the recorded actor.
	if _, err := s.ResolveTicket("T1", "operator", "first reason"); err != nil {
		t.Fatalf("first ResolveTicket: %v", err)
	}
	if _, err := s.ResolveTicket("T1", "someone-else", "second reason"); !errors.Is(err, ErrTicketTerminal) {
		t.Errorf("double resolve = %v, want ErrTicketTerminal", err)
	}
	got, _ := s.GetTicket("T1")
	if got.ResolvedBy != "operator" || got.ResolutionReason != "first reason" {
		t.Errorf("resolution overwritten: by=%q reason=%q", got.ResolvedBy, got.ResolutionReason)
	}
}

func TestResolveTicket_NotFound(t *testing.T) {
	s := testStore(t)
	if _, err := s.ResolveTicket("nope", "operator", "reason"); !errors.Is(err, ErrTicketNotFound) {
		t.Errorf("resolve of an unknown ticket = %v, want ErrTicketNotFound", err)
	}
}

// TestResolveTicket_UnblocksDependents is the ripple test for the new terminal
// status. A resolved blocker that still counted as open would freeze every
// ticket downstream of an escalation.
func TestResolveTicket_UnblocksDependents(t *testing.T) {
	s := testStore(t)
	if err := s.CreateSprint(Sprint{ID: "S1", Name: "test"}); err != nil {
		t.Fatalf("CreateSprint: %v", err)
	}
	if err := s.CreateTicket(Ticket{ID: "blocker", SprintID: "S1", Title: "blocker", Status: StatusReady}); err != nil {
		t.Fatalf("CreateTicket blocker: %v", err)
	}
	if err := s.CreateTicket(Ticket{ID: "dependent", SprintID: "S1", Title: "dependent", Status: StatusReady}); err != nil {
		t.Fatalf("CreateTicket dependent: %v", err)
	}
	if err := s.AddDependency("dependent", "blocker"); err != nil {
		t.Fatalf("AddDependency: %v", err)
	}
	if _, err := s.ClaimTicket("blocker", "agent-a"); err != nil {
		t.Fatalf("ClaimTicket: %v", err)
	}

	// Positive control: while the blocker is open, the dependent is blocked.
	// Without this the test would pass even if BlockedBy returned nothing at
	// all, which is the failure mode it exists to catch.
	blockers, err := s.BlockedBy("dependent")
	if err != nil {
		t.Fatalf("BlockedBy: %v", err)
	}
	if len(blockers) != 1 {
		t.Fatalf("blockers before resolution = %v, want [blocker]", blockers)
	}

	if _, err := s.ResolveTicket("blocker", "operator", "written off"); err != nil {
		t.Fatalf("ResolveTicket: %v", err)
	}

	blockers, err = s.BlockedBy("dependent")
	if err != nil {
		t.Fatalf("BlockedBy: %v", err)
	}
	if len(blockers) != 0 {
		t.Errorf("blockers after resolution = %v, want none", blockers)
	}

	ready, err := s.ReadyTickets("S1")
	if err != nil {
		t.Fatalf("ReadyTickets: %v", err)
	}
	var sawDependent bool
	for _, tk := range ready {
		if tk.ID == "dependent" {
			sawDependent = true
		}
	}
	if !sawDependent {
		t.Error("dependent absent from the ready queue; a resolved blocker still blocks")
	}
}

// TestResolveTicket_CountsAsClosedWork guards the two aggregates that would
// otherwise report a resolved ticket as outstanding forever.
func TestResolveTicket_CountsAsClosedWork(t *testing.T) {
	s := testStore(t)
	if err := s.CreateSprint(Sprint{ID: "S1", Name: "test"}); err != nil {
		t.Fatalf("CreateSprint: %v", err)
	}
	if err := s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "poison", Status: StatusReady, OwnerAgent: "agent-a"}); err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	if err := s.SetTicketEstimate("T1", 3); err != nil {
		t.Fatalf("SetTicketEstimate: %v", err)
	}
	if _, err := s.ClaimTicket("T1", "agent-a"); err != nil {
		t.Fatalf("ClaimTicket: %v", err)
	}

	before, err := s.GetSprintBurndown("S1")
	if err != nil {
		t.Fatalf("GetSprintBurndown: %v", err)
	}
	if before.RemainingEstimate != 3 {
		t.Fatalf("remaining before resolution = %v, want 3", before.RemainingEstimate)
	}

	if _, err := s.ResolveTicket("T1", "operator", "written off"); err != nil {
		t.Fatalf("ResolveTicket: %v", err)
	}

	after, err := s.GetSprintBurndown("S1")
	if err != nil {
		t.Fatalf("GetSprintBurndown: %v", err)
	}
	if after.RemainingEstimate != 0 {
		t.Errorf("remaining after resolution = %v, want 0: the sprint can never burn down", after.RemainingEstimate)
	}

	wl, err := s.AgentWorkload("S1")
	if err != nil {
		t.Fatalf("AgentWorkload: %v", err)
	}
	if len(wl) != 1 {
		t.Fatalf("workload entries = %d, want 1", len(wl))
	}
	if wl[0].QueuedTickets != 0 {
		t.Errorf("queued = %d, want 0: a resolved ticket is not pending work", wl[0].QueuedTickets)
	}
	if wl[0].DoneTickets != 1 {
		t.Errorf("done = %d, want 1", wl[0].DoneTickets)
	}
}

func TestStaleInProgress_ReportsAgeAndDropsResolved(t *testing.T) {
	s := testStore(t)
	escalated(t, s, "T1", "fleet-agent-01")

	// Backdate the claim so the ticket is genuinely old rather than merely
	// present. Direct UPDATE is the only way to move the clock here.
	old := time.Now().Add(-48 * time.Hour)
	if _, err := s.db.Exec(`UPDATE tickets SET claimed_at = ? WHERE id = ?`, formatTime(old), "T1"); err != nil {
		t.Fatalf("backdate claim: %v", err)
	}

	stale, err := s.StaleInProgress(24 * time.Hour)
	if err != nil {
		t.Fatalf("StaleInProgress: %v", err)
	}
	if len(stale) != 1 || stale[0].ID != "T1" {
		t.Fatalf("stale = %+v, want [T1]", stale)
	}
	if stale[0].ClaimedBy != "fleet-agent-01" {
		t.Errorf("claimed_by = %q, want the escalating agent", stale[0].ClaimedBy)
	}
	if stale[0].AgeSeconds < int64((47 * time.Hour).Seconds()) {
		t.Errorf("age = %ds, want roughly 48h", stale[0].AgeSeconds)
	}

	// A longer window must not match it.
	if got, err := s.StaleInProgress(72 * time.Hour); err != nil || len(got) != 0 {
		t.Errorf("StaleInProgress(72h) = %+v, %v; want empty", got, err)
	}

	// The point of the whole change: resolving the ticket takes it off the
	// view. Reverting the resolve route would leave it here forever.
	if _, err := s.ResolveTicket("T1", "operator", "closing"); err != nil {
		t.Fatalf("ResolveTicket: %v", err)
	}
	got, err := s.StaleInProgress(24 * time.Hour)
	if err != nil {
		t.Fatalf("StaleInProgress: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("stale after resolution = %+v, want empty", got)
	}
}

// TestStaleInProgress_IgnoresFutureClaims covers this host specifically: its
// wall clock steps backwards, so a claim can carry a future timestamp. Such a
// ticket is not stale, and reporting a huge age for it would page a human
// about a clock bug.
func TestStaleInProgress_IgnoresFutureClaims(t *testing.T) {
	s := testStore(t)
	escalated(t, s, "T1", "agent-a")

	future := time.Now().Add(2 * time.Hour)
	if _, err := s.db.Exec(`UPDATE tickets SET claimed_at = ? WHERE id = ?`, formatTime(future), "T1"); err != nil {
		t.Fatalf("post-date claim: %v", err)
	}

	got, err := s.StaleInProgress(0)
	if err != nil {
		t.Fatalf("StaleInProgress: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("stale = %+v, want empty for a future-dated claim", got)
	}
}
