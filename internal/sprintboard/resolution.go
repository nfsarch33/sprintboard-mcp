package sprintboard

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Human resolution of an escalated ticket.
//
// The agent-facing close path is CompleteTicket, which requires the caller to
// be the claimer. That guard is correct and stays: an agent may only close
// what it actually did. But it left a hole at the other end of the loop. When
// the fleet agent escalates -- it leaves the ticket in_progress, posts the
// failure evidence as a comment, and stops -- the human it escalated TO had no
// route at all. The only close path demanded agent_id == claimer, so a person
// could close an escalation only by impersonating the agent that raised it.
// Escalated tickets therefore accumulated forever and in_progress stopped
// distinguishing "an agent is working on this" from "this was handed to a
// human weeks ago".
//
// ResolveTicket is that missing route. It is deliberately a SEPARATE verb from
// completion rather than a relaxation of it:
//
//   - it does not require the actor to be the claimer, because the whole point
//     is that the claimer cannot finish;
//   - it lands on StatusResolvedByHuman, not StatusDone, so the board never
//     claims an agent delivered something a person wrote off;
//   - it records who closed it and why, on the ticket and in the transition
//     log, so the escalation comment keeps a matching answer.

var (
	// ErrTicketNotFound reports that no ticket carries the given id.
	ErrTicketNotFound = errors.New("sprintboard: ticket not found")

	// ErrTicketTerminal reports that the ticket is already closed. Resolving
	// twice is refused rather than silently overwritten: the first resolution
	// (or the agent's own completion) is the one that happened, and letting a
	// second call replace the recorded actor and reason would corrupt exactly
	// the audit trail this route exists to create.
	ErrTicketTerminal = errors.New("sprintboard: ticket is already in a terminal state")
)

// migrateResolution adds the human-resolution audit columns. Idempotent on
// re-open, matching the other ALTER-based migrations.
func (s *Store) migrateResolution() error {
	stmts := []string{
		`ALTER TABLE tickets ADD COLUMN resolved_by TEXT`,
		`ALTER TABLE tickets ADD COLUMN resolved_at TEXT`,
		`ALTER TABLE tickets ADD COLUMN resolution_reason TEXT`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil && !isAlterColumnExists(err) {
			return fmt.Errorf("migrate resolution: %w", err)
		}
	}
	return nil
}

// ResolveTicket closes ticketID as StatusResolvedByHuman on behalf of actor,
// recording reason. Unlike CompleteTicket it does NOT require actor to be the
// claimer -- an escalated ticket is one its claimer has already given up on.
//
// Both actor and reason are mandatory. A resolution with no name against it is
// indistinguishable from the silent accumulation this route exists to end, and
// a resolution with no reason destroys the pairing with the escalation comment
// that explains why the ticket was handed over in the first place.
//
// Returns ErrTicketNotFound if no such ticket exists and ErrTicketTerminal if
// it is already closed. The guard rides in the UPDATE's WHERE clause, so two
// concurrent resolutions cannot both win.
func (s *Store) ResolveTicket(ticketID, actor, reason string) (Ticket, error) {
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

	// completed_at is deliberately left alone. It feeds SprintSLAs's
	// time-to-complete, and stamping it here would report a completion
	// duration for work that was never completed. resolved_at is the
	// honest field for when a person closed the loop.
	res, err := s.db.Exec(
		`UPDATE tickets
		 SET status = ?, resolved_by = ?, resolved_at = ?, resolution_reason = ?,
		     updated_at = ?
		 WHERE id = ? AND status NOT IN `+terminalStatusSQL,
		StatusResolvedByHuman, actor, now, reason, now, ticketID,
	)
	if err != nil {
		return Ticket{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Ticket{}, fmt.Errorf("%w: %q is %s", ErrTicketTerminal, ticketID, fromStatus)
	}

	if _, err := s.db.Exec(
		`INSERT INTO ticket_transitions (ticket_id, from_status, to_status, agent_id, note, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		ticketID, fromStatus, StatusResolvedByHuman, actor, reason, now,
	); err != nil {
		return Ticket{}, err
	}

	return s.GetTicket(ticketID)
}

// StaleTicket is one in-progress ticket that has sat untouched, with how long
// it has sat there.
type StaleTicket struct {
	ID           string    `json:"id"`
	SprintID     string    `json:"sprint_id,omitempty"`
	Title        string    `json:"title"`
	ClaimedBy    string    `json:"claimed_by,omitempty"`
	InProgressAt time.Time `json:"in_progress_since"`
	AgeSeconds   int64     `json:"age_seconds"`
}

// StaleInProgress returns in_progress tickets whose claim is at least
// olderThan old, longest-waiting first. Pass 0 to get every in_progress ticket.
//
// This is the closest the board can get to "escalated and still open". The
// board cannot see escalations directly -- an escalation is expressed as
// (status stays in_progress) plus (a comment appears), and a comment is not a
// state change -- so this measures the observable symptom instead: a claim
// that stopped moving. That is a superset, and a useful one, since it also
// catches an agent that died mid-ticket without escalating at all.
//
// The cutoff is applied in Go rather than SQL on purpose. Timestamps are
// stored as RFC3339 strings, so a SQL string comparison against a formatted
// cutoff is only correct while every row shares one UTC offset; it silently
// goes wrong across a DST boundary or a second writer in another zone. The
// in_progress population is bounded by the fleet size, so parsing it in Go
// costs nothing and cannot be wrong.
func (s *Store) StaleInProgress(olderThan time.Duration) ([]StaleTicket, error) {
	rows, err := s.db.Query(
		`SELECT id, sprint_id, title, claimed_by, claimed_at, updated_at
		 FROM tickets WHERE status = ?`,
		StatusInProgress,
	)
	if err != nil {
		return nil, fmt.Errorf("query in-progress tickets: %w", err)
	}
	defer rows.Close()

	now := time.Now()
	out := []StaleTicket{}
	for rows.Next() {
		var (
			id, title                  string
			sprintID, claimedBy        sql.NullString
			claimedAtRaw, updatedAtRaw sql.NullString
		)
		if err := rows.Scan(&id, &sprintID, &title, &claimedBy, &claimedAtRaw, &updatedAtRaw); err != nil {
			return nil, fmt.Errorf("scan in-progress ticket: %w", err)
		}

		// claimed_at is the clock that matters: a comment (which is how an
		// escalation announces itself) does not touch updated_at, so for an
		// escalated ticket the two are equal anyway. updated_at is the
		// fallback for rows claimed before claimed_at existed.
		since := parseTime(nullString(claimedAtRaw))
		if since.IsZero() {
			since = parseTime(nullString(updatedAtRaw))
		}
		if since.IsZero() {
			continue // no usable clock; reporting an age would be a guess
		}

		age := now.Sub(since)
		if age < 0 {
			// This host's wall clock steps backwards. A claim stamped in the
			// future is not stale; treating it as hugely stale would page a
			// human about a clock bug.
			continue
		}
		if age < olderThan {
			continue
		}
		out = append(out, StaleTicket{
			ID:           id,
			SprintID:     nullString(sprintID),
			Title:        title,
			ClaimedBy:    nullString(claimedBy),
			InProgressAt: since,
			AgeSeconds:   int64(age.Seconds()),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].AgeSeconds != out[j].AgeSeconds {
			return out[i].AgeSeconds > out[j].AgeSeconds
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}
