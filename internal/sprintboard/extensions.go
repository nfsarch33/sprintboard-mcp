package sprintboard

import (
	"database/sql"
	"encoding/json"
	"time"
)

// MarshalJSON omits zero-valued time.Time fields so JSON consumers see
// `due_date`, `claimed_at`, `completed_at` only when populated.
func (t Ticket) MarshalJSON() ([]byte, error) {
	type ticketAlias Ticket
	out := struct {
		*ticketAlias
		DueDate     *time.Time `json:"due_date,omitempty"`
		ClaimedAt   *time.Time `json:"claimed_at,omitempty"`
		CompletedAt *time.Time `json:"completed_at,omitempty"`
	}{ticketAlias: (*ticketAlias)(&t)}
	if !t.DueDate.IsZero() {
		v := t.DueDate
		out.DueDate = &v
	}
	if !t.ClaimedAt.IsZero() {
		v := t.ClaimedAt
		out.ClaimedAt = &v
	}
	if !t.CompletedAt.IsZero() {
		v := t.CompletedAt
		out.CompletedAt = &v
	}
	return json.Marshal(out)
}

// encodeLabels marshals a label slice to a JSON array stored in the labels
// TEXT column. nil/empty returns "" so the column stays NULL on read.
func encodeLabels(labels []string) interface{} {
	if len(labels) == 0 {
		return nil
	}
	raw, err := json.Marshal(labels)
	if err != nil {
		return nil
	}
	return string(raw)
}

// decodeLabels parses the labels TEXT column. Empty/invalid returns nil so
// JSON marshalling of the parent Ticket omits the field via omitempty.
func decodeLabels(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

// SLA reports time-to-claim and time-to-complete for a single ticket. The
// `*_ms` fields are persisted at transition time (v8000-B18) so consumers can
// avoid recomputing from RFC3339 strings; the legacy duration fields remain
// for callers that haven't migrated yet.
type SLA struct {
	TicketID         string        `json:"ticket_id"`
	ClaimedBy        string        `json:"claimed_by"`
	CreatedAt        time.Time     `json:"created_at"`
	ClaimedAt        time.Time     `json:"claimed_at"`
	CompletedAt      time.Time     `json:"completed_at,omitempty"`
	TimeToClaim      time.Duration `json:"time_to_claim_ns"`
	TimeToComplete   time.Duration `json:"time_to_complete_ns,omitempty"`
	TimeToClaimMS    int64         `json:"time_to_claim_ms,omitempty"`
	TimeToCompleteMS int64         `json:"time_to_complete_ms,omitempty"`
}

// SprintSLAs returns SLA metrics for every claimed ticket in a sprint. Tickets
// that were never claimed are excluded so callers can compute averages cleanly.
func (s *Store) SprintSLAs(sprintID string) ([]SLA, error) {
	rows, err := s.db.Query(
		`SELECT id, claimed_by, created_at, claimed_at, completed_at,
		        time_to_claim_ms, time_to_complete_ms
		 FROM tickets
		 WHERE sprint_id = ? AND claimed_at IS NOT NULL AND claimed_at != ''
		 ORDER BY created_at ASC`,
		sprintID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SLA
	for rows.Next() {
		var (
			id                                  string
			claimedBy, createdAt                sql.NullString
			claimedAtStr, completedAtStr        sql.NullString
			timeToClaimMS, timeToCompleteMS     sql.NullInt64
		)
		if err := rows.Scan(
			&id, &claimedBy, &createdAt, &claimedAtStr, &completedAtStr,
			&timeToClaimMS, &timeToCompleteMS,
		); err != nil {
			return nil, err
		}
		sla := SLA{
			TicketID:    id,
			ClaimedBy:   nullString(claimedBy),
			CreatedAt:   parseTime(nullString(createdAt)),
			ClaimedAt:   parseTime(nullString(claimedAtStr)),
			CompletedAt: parseTime(nullString(completedAtStr)),
		}
		if !sla.CreatedAt.IsZero() && !sla.ClaimedAt.IsZero() {
			sla.TimeToClaim = sla.ClaimedAt.Sub(sla.CreatedAt)
		}
		if !sla.ClaimedAt.IsZero() && !sla.CompletedAt.IsZero() {
			sla.TimeToComplete = sla.CompletedAt.Sub(sla.ClaimedAt)
		}
		if timeToClaimMS.Valid {
			sla.TimeToClaimMS = timeToClaimMS.Int64
		} else if sla.TimeToClaim > 0 {
			sla.TimeToClaimMS = sla.TimeToClaim.Milliseconds()
		}
		if timeToCompleteMS.Valid {
			sla.TimeToCompleteMS = timeToCompleteMS.Int64
		} else if sla.TimeToComplete > 0 {
			sla.TimeToCompleteMS = sla.TimeToComplete.Milliseconds()
		}
		out = append(out, sla)
	}
	return out, rows.Err()
}
