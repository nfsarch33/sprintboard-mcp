// T-8800-B14: AgentWorkload analytics.
//
// Returns one row per agent that owns or claims at least one ticket, with
// active vs done counts. Active = anything that isn't done/blocked. The result
// is sorted by ActiveTickets DESC so an orchestrator can pop the least-loaded
// agent off the back of the slice.
package sprintboard

import (
	"sort"
)

type AgentWorkloadEntry struct {
	AgentID        string `json:"agent_id"`
	ActiveTickets  int    `json:"active_tickets"`
	QueuedTickets  int    `json:"queued_tickets"`
	DoneTickets    int    `json:"done_tickets"`
	BlockedTickets int    `json:"blocked_tickets"`
	TotalAssigned  int    `json:"total_assigned"`
}

// AgentWorkload aggregates per-agent ticket counts. If sprintID is empty the
// counts span every sprint. The "agent" is the claimed_by, falling back to
// owner_agent when no claim exists. Tickets with neither are skipped.
func (s *Store) AgentWorkload(sprintID string) ([]AgentWorkloadEntry, error) {
	query := `SELECT COALESCE(NULLIF(claimed_by, ''), owner_agent) AS agent, status FROM tickets
	          WHERE COALESCE(NULLIF(claimed_by, ''), owner_agent) IS NOT NULL
	            AND COALESCE(NULLIF(claimed_by, ''), owner_agent) != ''`
	args := []interface{}{}
	if sprintID != "" {
		query += ` AND sprint_id = ?`
		args = append(args, sprintID)
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	agg := map[string]*AgentWorkloadEntry{}
	for rows.Next() {
		var owner, status string
		if err := rows.Scan(&owner, &status); err != nil {
			return nil, err
		}
		entry, ok := agg[owner]
		if !ok {
			entry = &AgentWorkloadEntry{AgentID: owner}
			agg[owner] = entry
		}
		entry.TotalAssigned++
		switch TicketStatus(status) {
		case StatusDone:
			entry.DoneTickets++
		case StatusBlocked:
			entry.BlockedTickets++
		case StatusInProgress, StatusReview, StatusReadyHandoff:
			entry.ActiveTickets++
		default:
			entry.QueuedTickets++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]AgentWorkloadEntry, 0, len(agg))
	for _, e := range agg {
		out = append(out, *e)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ActiveTickets != out[j].ActiveTickets {
			return out[i].ActiveTickets > out[j].ActiveTickets
		}
		return out[i].AgentID < out[j].AgentID
	})
	return out, nil
}
