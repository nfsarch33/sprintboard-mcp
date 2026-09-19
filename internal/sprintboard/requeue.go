package sprintboard

import (
	"errors"
	"fmt"
	"time"
)

// Requeue: the way back from a terminal status.
//
// The terminal guard on ClaimTicket makes done/resolved tickets unclaimable,
// which is correct until it isn't: a ticket closed by mistake, or whose
// delivered work turned out to be wrong, must be openable again. RequeueTicket
// is that path, and it is deliberately narrow:
//
//   - it only moves a TERMINAL ticket; requeueing live work is a force-release
//     of someone's in-flight claim and a different verb entirely;
//   - it requires an actor and a reason, recorded on the transition row, so
//     the audit trail always names who reopened closed work and why;
//   - it clears the claim and lands on ready, the state agents pick work up
//     from, and leaves evidence/completed_at untouched -- the closed attempt
//     stays visible in history rather than being erased.
//
// Together with the terminal guard it replaces the old failure mode -- a
// finished ticket silently resurrected by a claim with a false "ready ->
// in_progress" audit row -- with an explicit, attributed reopen.

// ErrTicketNotTerminal reports a requeue attempt against a ticket that is not
// closed. Live work is released through its own path, not this one.
var ErrTicketNotTerminal = errors.New("sprintboard: ticket is not in a terminal state")

// RequeueTicket moves ticketID from a terminal status back to ready, clearing
// the claim. Actor and reason are mandatory and land on the audit trail.
//
// Returns ErrTicketNotFound if no such ticket exists and ErrTicketNotTerminal
// if it is not closed. The terminal guard rides in the UPDATE's WHERE clause
// so two concurrent requeues cannot double-apply.
func (s *Store) RequeueTicket(ticketID, actor, reason string) (Ticket, error) {
	if ticketID == "" {
		return Ticket{}, errors.New("sprintboard: ticket_id is required")
	}
	if actor == "" {
		return Ticket{}, errors.New("sprintboard: actor is required")
	}
	if reason == "" {
		return Ticket{}, errors.New("sprintboard: reason is required")
	}

	var fromStatus string
	if err := s.db.QueryRow(`SELECT status FROM tickets WHERE id = ?`, ticketID).
		Scan(&fromStatus); err != nil {
		return Ticket{}, fmt.Errorf("%w: %q", ErrTicketNotFound, ticketID)
	}

	now := formatTime(time.Now())
	// claimed_by/claimed_at are cleared so the next claim is a fresh race;
	// evidence, completed_at and the SLA columns keep the closed attempt's
	// history. updated_at moves because the row changed, which is what
	// stale-claim sweeps and UIs sort on.
	res, err := s.db.Exec(
		`UPDATE tickets
		 SET status = ?, claimed_by = NULL, claimed_at = NULL, updated_at = ?
		 WHERE id = ? AND status IN `+terminalStatusSQL,
		StatusReady, now, ticketID,
	)
	if err != nil {
		return Ticket{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Ticket{}, fmt.Errorf("%w: %q is %s", ErrTicketNotTerminal, ticketID, fromStatus)
	}

	if _, err := s.db.Exec(
		`INSERT INTO ticket_transitions (ticket_id, from_status, to_status, agent_id, note, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		ticketID, fromStatus, StatusReady, actor, "requeued: "+reason, now,
	); err != nil {
		return Ticket{}, err
	}

	return s.GetTicket(ticketID)
}
