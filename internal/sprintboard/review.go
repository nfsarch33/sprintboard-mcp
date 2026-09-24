package sprintboard

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrTicketNotInReview reports a rework request for a ticket that is not
// waiting on review under a claim. Like ErrTicketNotClaimedBy it is the
// ordinary outcome of a race (the change merged, or its claimant already took
// it back), not a failure.
var ErrTicketNotInReview = errors.New("sprintboard: ticket is not in review under a claim")

// SubmitForReview moves a ticket its claimant has finished from in_progress to
// review and keeps the claim. The stale-claim sweeper and the stale view only
// consider in_progress work, so a change that waits on a reviewer or a merge
// queue is not released for being idle: the wait belongs to the reviewer, not
// to the claimant.
//
// Idempotent for the claimant: submitting a ticket that is already in review
// under the same claim (a new revision of the same change) succeeds without a
// second transition row. Anything else (another agent, an unclaimed ticket, a
// ticket in any other status) is ErrTicketNotClaimedBy.
func (s *Store) SubmitForReview(ticketID, agentID, note string) error {
	if ticketID == "" {
		return errors.New("sprintboard: ticket_id is required")
	}
	if agentID == "" {
		return errors.New("sprintboard: agent_id is required")
	}
	now := formatTime(time.Now())
	res, err := s.db.Exec(
		`UPDATE tickets SET status = ?, updated_at = ?
		 WHERE id = ? AND claimed_by = ? AND status = ?`,
		StatusReview, now, ticketID, agentID, StatusInProgress,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		var status string
		err := s.db.QueryRow(
			`SELECT status FROM tickets WHERE id = ? AND claimed_by = ?`,
			ticketID, agentID,
		).Scan(&status)
		switch {
		case err == nil && TicketStatus(status) == StatusReview:
			return nil
		case err != nil && !errors.Is(err, sql.ErrNoRows):
			return err
		}
		return fmt.Errorf("%w: %q by %q", ErrTicketNotClaimedBy, ticketID, agentID)
	}
	_, err = s.db.Exec(
		`INSERT INTO ticket_transitions (ticket_id, from_status, to_status, agent_id, note, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		ticketID, StatusInProgress, StatusReview, agentID, note, now,
	)
	return err
}

// ReturnToWork is the reviewer's "changes requested" verb: a ticket in review
// goes back to in_progress for the SAME claimant, and the claim's lease
// restarts now, so the claimant gets a full sweeper window to pick the change
// back up before the claim can lapse. Any agent may call it; the actor and the
// reason are recorded on the transition. A ticket that is not in review under
// a claim is ErrTicketNotInReview.
func (s *Store) ReturnToWork(ticketID, actor, reason string) error {
	if ticketID == "" {
		return errors.New("sprintboard: ticket_id is required")
	}
	if actor == "" {
		return errors.New("sprintboard: actor is required")
	}
	now := formatTime(time.Now())
	res, err := s.db.Exec(
		`UPDATE tickets SET status = ?, claimed_at = ?, updated_at = ?
		 WHERE id = ? AND status = ? AND claimed_by IS NOT NULL AND claimed_by != ''`,
		StatusInProgress, now, now, ticketID, StatusReview,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("%w: %q", ErrTicketNotInReview, ticketID)
	}
	_, err = s.db.Exec(
		`INSERT INTO ticket_transitions (ticket_id, from_status, to_status, agent_id, note, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		ticketID, StatusReview, StatusInProgress, actor, reason, now,
	)
	return err
}
