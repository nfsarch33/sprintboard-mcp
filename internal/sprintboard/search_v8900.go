package sprintboard

import (
	"database/sql"
	"fmt"
	"strings"
)

// TicketFilter is the v8900-B16 search predicate. All fields are optional;
// the zero value means "match every ticket".
type TicketFilter struct {
	Query       string       // free-text match against title, description, acceptance_criteria
	Status      TicketStatus // exact-match status
	Owner       string       // owner_agent
	Labels      []string     // any-of match against the encoded labels JSON array
	PriorityMin int          // priority >= PriorityMin
	SprintID    string       // exact-match sprint_id
	Limit       int          // 0 means unbounded
}

// SearchTickets returns tickets matching the supplied filter, ordered by
// priority desc then created_at asc (matching ListTickets).
func (s *Store) SearchTickets(f TicketFilter) ([]Ticket, error) {
	var (
		conds []string
		args  []interface{}
	)

	if f.Query != "" {
		like := "%" + f.Query + "%"
		conds = append(conds, "(title LIKE ? OR description LIKE ? OR acceptance_criteria LIKE ?)")
		args = append(args, like, like, like)
	}
	if f.Status != "" {
		conds = append(conds, "status = ?")
		args = append(args, string(f.Status))
	}
	if f.Owner != "" {
		conds = append(conds, "owner_agent = ?")
		args = append(args, f.Owner)
	}
	if f.SprintID != "" {
		conds = append(conds, "sprint_id = ?")
		args = append(args, f.SprintID)
	}
	if f.PriorityMin > 0 {
		conds = append(conds, "priority >= ?")
		args = append(args, f.PriorityMin)
	}
	for _, label := range f.Labels {
		// labels is stored as a JSON array string e.g. ["backend","go"]; the
		// LIKE pattern keys on the quoted token so "backend" doesn't match
		// "backendpolish".
		conds = append(conds, "labels LIKE ?")
		args = append(args, "%\""+label+"\"%")
	}

	q := `SELECT id, sprint_id, title, description, status, owner_agent, priority, acceptance_criteria, handoff_doc_path, due_date, labels, claimed_by, claimed_at, completed_at, created_at, updated_at FROM tickets`
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += " ORDER BY priority DESC, created_at ASC"
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", f.Limit)
	}

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Ticket
	for rows.Next() {
		var t Ticket
		var createdAt, updatedAt string
		var sprintID, description, ownerAgent, acceptanceCriteria, handoffDocPath sql.NullString
		var dueDate, labelsRaw, claimedBy, claimedAt, completedAt sql.NullString
		if err := rows.Scan(&t.ID, &sprintID, &t.Title, &description, &t.Status,
			&ownerAgent, &t.Priority, &acceptanceCriteria, &handoffDocPath,
			&dueDate, &labelsRaw, &claimedBy, &claimedAt, &completedAt,
			&createdAt, &updatedAt); err != nil {
			return nil, err
		}
		t.SprintID = nullString(sprintID)
		t.Description = nullString(description)
		t.OwnerAgent = nullString(ownerAgent)
		t.AcceptanceCriteria = nullString(acceptanceCriteria)
		t.HandoffDocPath = nullString(handoffDocPath)
		t.DueDate = parseTime(nullString(dueDate))
		t.Labels = decodeLabels(nullString(labelsRaw))
		t.ClaimedBy = nullString(claimedBy)
		t.ClaimedAt = parseTime(nullString(claimedAt))
		t.CompletedAt = parseTime(nullString(completedAt))
		t.CreatedAt = parseTime(createdAt)
		t.UpdatedAt = parseTime(updatedAt)
		out = append(out, t)
	}
	return out, rows.Err()
}
