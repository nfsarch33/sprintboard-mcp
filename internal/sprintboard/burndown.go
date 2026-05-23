package sprintboard

import (
	"database/sql"
	"fmt"
	"time"
)

type BurndownEntry struct {
	SprintID        string  `json:"sprint_id"`
	TotalEstimate   float64 `json:"total_estimate_hours"`
	DoneEstimate    float64 `json:"done_estimate_hours"`
	RemainingEstimate float64 `json:"remaining_estimate_hours"`
	TicketCount     int     `json:"ticket_count"`
	DoneCount       int     `json:"done_count"`
	Timestamp       string  `json:"timestamp"`
}

func (s *Store) SetTicketEstimate(ticketID string, hours float64) error {
	now := time.Now().Format(time.RFC3339)
	result, err := s.db.Exec(
		"UPDATE tickets SET estimate_hours = ?, updated_at = ? WHERE id = ?",
		hours, now, ticketID,
	)
	if err != nil {
		return fmt.Errorf("set estimate: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("ticket %s not found", ticketID)
	}
	return nil
}

func (s *Store) GetSprintBurndown(sprintID string) (*BurndownEntry, error) {
	var entry BurndownEntry
	entry.SprintID = sprintID
	entry.Timestamp = time.Now().Format(time.RFC3339)

	err := s.db.QueryRow(`
		SELECT 
			COALESCE(SUM(estimate_hours), 0),
			COALESCE(SUM(CASE WHEN status = 'done' THEN estimate_hours ELSE 0 END), 0),
			COUNT(*),
			SUM(CASE WHEN status = 'done' THEN 1 ELSE 0 END)
		FROM tickets WHERE sprint_id = ?
	`, sprintID).Scan(
		&entry.TotalEstimate,
		&entry.DoneEstimate,
		&entry.TicketCount,
		&entry.DoneCount,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("sprint %s not found", sprintID)
		}
		return nil, fmt.Errorf("burndown query: %w", err)
	}
	entry.RemainingEstimate = entry.TotalEstimate - entry.DoneEstimate
	return &entry, nil
}

func (s *Store) StealTicket(ticketID, agentID, reason string) error {
	now := time.Now().Format(time.RFC3339)
	result, err := s.db.Exec(`
		UPDATE tickets 
		SET claimed_by = ?, claimed_at = ?, status = 'in_progress', updated_at = ?
		WHERE id = ? AND (status = 'in_progress' OR status = 'backlog' OR status = 'ready')
	`, agentID, now, now, ticketID)
	if err != nil {
		return fmt.Errorf("steal ticket: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("ticket %s not found or in terminal state", ticketID)
	}
	return nil
}
