package sprintboard

import (
	"fmt"
	"time"
)

type CoordinationHandoff struct {
	ID        int64     `json:"id"`
	TicketID  string    `json:"ticket_id"`
	FromAgent string    `json:"from_agent"`
	ToAgent   string    `json:"to_agent"`
	Summary   string    `json:"summary"`
	Branch    string    `json:"branch,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) PublishHandoff(h CoordinationHandoff) (int64, error) {
	now := time.Now()
	if h.CreatedAt.IsZero() {
		h.CreatedAt = now
	}

	id, err := s.insertReturningID(
		`INSERT INTO handoffs (ticket_id, from_agent, to_agent, context_path, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		h.TicketID, h.FromAgent, h.ToAgent, h.Summary, formatTime(h.CreatedAt),
	)
	if err != nil {
		return 0, err
	}

	if h.Branch != "" {
		if err := s.updateTicketBranch(h.TicketID, h.Branch); err != nil {
			return 0, fmt.Errorf("persist branch: %w", err)
		}
	}

	// The handoffs table is the durable record. A best-effort bridge to an
	// external memory service used to run here; it was removed with the Mem0
	// retirement (see the commit that introduced this comment). Any future
	// bridge belongs behind an explicit interface with its own tests, not as
	// an unowned side effect of an insert.
	return id, nil
}

func (s *Store) SubscribeHandoffs(agentID string, since time.Time) ([]CoordinationHandoff, error) {
	rows, err := s.db.Query(
		`SELECT id, ticket_id, from_agent, to_agent, context_path, created_at
		 FROM handoffs WHERE to_agent = ? AND created_at >= ?
		 ORDER BY created_at DESC`,
		agentID, formatTime(since),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var handoffs []CoordinationHandoff
	for rows.Next() {
		var h CoordinationHandoff
		var createdAt, summary string
		if err := rows.Scan(&h.ID, &h.TicketID, &h.FromAgent, &h.ToAgent, &summary, &createdAt); err != nil {
			return nil, err
		}
		h.Summary = summary
		h.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		handoffs = append(handoffs, h)
	}
	return handoffs, rows.Err()
}

// The external memory bridge that lived here (bridgeToMem0 /
// mem0BridgeTimeout, reading MEM0_BASE_URL, MEM0_API_KEY and MEM0_TIMEOUT)
// has been removed. Mem0 is retired in favour of Engram, and this code was
// unreachable in practice: no MEM0_* variable is set for any process on any
// fleet node, so the function returned nil on its first line every time.
//
// It was not merely dead. It was the only outbound HTTP call in this package
// and carried both of the repository's HIGH-severity gosec findings (G704,
// SSRF via taint analysis) for a request that could never be sent.
//
// A replacement Engram bridge is deliberately NOT written here. Publishing a
// handoff to an external memory service is a separate concern from inserting
// a row, and hiding it inside PublishHandoff as an unowned, error-swallowing
// side effect is what made this hard to remove. If one is wanted, it belongs
// behind an explicit interface on Store with its own tests and its own
// failure semantics.
