package sprintboard

import (
	"errors"
	"fmt"
	"time"
)

// ErrTicketNotClaimedBy reports a renew attempt by an agent that does not
// hold the claim (or a ticket that is not in progress at all). It is an
// ordinary outcome for a poller racing a completed ticket, not a failure.
var ErrTicketNotClaimedBy = errors.New("sprintboard: ticket not currently claimed by this agent")

// RenewClaim refreshes claimed_at on a ticket the agent already holds
// (v18860-1), extending its lease against the stale-claim sweeper. Long runs
// that are demonstrably alive — the poller's in-place infra retry can hold a
// claim for tens of minutes — renew instead of being released.
//
// The claimant and in-progress guards ride in the UPDATE's WHERE clause, so a
// renew racing a complete or a sweep cannot resurrect either side. Returns
// the new claimed_at timestamp and ErrTicketNotClaimedBy when the guard
// rejects the renew.
func (s *Store) RenewClaim(ticketID, agentID string) (time.Time, error) {
	if ticketID == "" {
		return time.Time{}, errors.New("sprintboard: ticket_id is required")
	}
	if agentID == "" {
		return time.Time{}, errors.New("sprintboard: agent_id is required")
	}
	// formatTime(time.Now()), NOT .UTC(): ClaimTicket stamps local time and
	// ReleaseStaleClaims compares claimed_at lexically in SQL — mixing UTC
	// and offset-bearing stamps breaks that compare exactly the way
	// StaleInProgress's comment warns about. One clock convention per column.
	now := time.Now()
	res, err := s.db.Exec(
		`UPDATE tickets SET claimed_at = ?, updated_at = ?
		 WHERE id = ? AND claimed_by = ? AND status = ?`,
		formatTime(now), formatTime(now), ticketID, agentID, StatusInProgress,
	)
	if err != nil {
		return time.Time{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return time.Time{}, fmt.Errorf("%w: %q by %q", ErrTicketNotClaimedBy, ticketID, agentID)
	}
	return now, nil
}
