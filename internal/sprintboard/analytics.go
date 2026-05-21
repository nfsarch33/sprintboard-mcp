package sprintboard

import (
	"database/sql"
	"sort"
	"time"
)

type AgentVelocity struct {
	AgentID        string  `json:"agent_id"`
	TicketsDone    int     `json:"tickets_done"`
	TotalHours     float64 `json:"total_hours"`
	TicketsPerHour float64 `json:"tickets_per_hour"`
}

type BurndownPoint struct {
	Timestamp      time.Time `json:"timestamp"`
	TicketsDone    int       `json:"tickets_done"`
	TicketsTotal   int       `json:"tickets_total"`
	RemainingCount int       `json:"remaining"`
}

// PriorityReadyList returns ready tickets sorted by priority DESC then by
// topological (dependency DAG) order within the same priority tier.
func (s *Store) PriorityReadyList(sprintID string) ([]Ticket, error) {
	ready, err := s.ReadyTickets(sprintID)
	if err != nil {
		return nil, err
	}

	topoOrder, err := s.TopologicalSort(sprintID)
	if err != nil {
		topoOrder = nil
	}

	topoRank := make(map[string]int, len(topoOrder))
	for i, id := range topoOrder {
		topoRank[id] = i
	}

	sort.SliceStable(ready, func(i, j int) bool {
		if ready[i].Priority != ready[j].Priority {
			return ready[i].Priority > ready[j].Priority
		}
		ri, oki := topoRank[ready[i].ID]
		rj, okj := topoRank[ready[j].ID]
		if oki && okj {
			return ri < rj
		}
		return oki
	})

	return ready, nil
}

// SprintVelocity computes tickets/hour per agent for a given sprint
// by examining ticket transitions to "done" status.
func (s *Store) SprintVelocity(sprintID string) ([]AgentVelocity, error) {
	sprint, err := s.GetSprint(sprintID)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(`
		SELECT t.agent_id,
		       COUNT(*) as done_count,
		       MIN(t.timestamp) as first_done,
		       MAX(t.timestamp) as last_done
		FROM ticket_transitions t
		JOIN tickets tk ON t.ticket_id = tk.id
		WHERE tk.sprint_id = ?
		  AND t.to_status = 'done'
		GROUP BY t.agent_id
		ORDER BY done_count DESC`,
		sprintID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var velocities []AgentVelocity
	for rows.Next() {
		var agentID string
		var count int
		var firstDone, lastDone sql.NullString
		if err := rows.Scan(&agentID, &count, &firstDone, &lastDone); err != nil {
			return nil, err
		}

		first := parseTime(nullStr(firstDone))
		if first.IsZero() {
			first = sprint.CreatedAt
		}

		last := parseTime(nullStr(lastDone))
		if last.IsZero() {
			last = time.Now()
		}

		hours := last.Sub(first).Hours()
		if hours < 0.001 {
			hours = 0.001
		}

		velocities = append(velocities, AgentVelocity{
			AgentID:        agentID,
			TicketsDone:    count,
			TotalHours:     hours,
			TicketsPerHour: float64(count) / hours,
		})
	}
	return velocities, rows.Err()
}

// SprintBurndown produces a timeline of ticket completions for charting.
func (s *Store) SprintBurndown(sprintID string) ([]BurndownPoint, error) {
	sprint, err := s.GetSprint(sprintID)
	if err != nil {
		return nil, err
	}

	var totalTickets int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM tickets WHERE sprint_id = ?`, sprintID,
	).Scan(&totalTickets); err != nil {
		return nil, err
	}

	points := []BurndownPoint{
		{Timestamp: sprint.CreatedAt, TicketsDone: 0, TicketsTotal: totalTickets, RemainingCount: totalTickets},
	}

	rows, err := s.db.Query(`
		SELECT t.timestamp
		FROM ticket_transitions t
		JOIN tickets tk ON t.ticket_id = tk.id
		WHERE tk.sprint_id = ?
		  AND t.to_status = 'done'
		ORDER BY t.timestamp ASC`,
		sprintID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	done := 0
	for rows.Next() {
		var ts string
		if err := rows.Scan(&ts); err != nil {
			return nil, err
		}
		done++
		points = append(points, BurndownPoint{
			Timestamp:      parseTime(ts),
			TicketsDone:    done,
			TicketsTotal:   totalTickets,
			RemainingCount: totalTickets - done,
		})
	}

	return points, rows.Err()
}

func nullStr(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}
