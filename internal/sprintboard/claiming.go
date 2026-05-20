package sprintboard

import (
	"database/sql"
	"fmt"
	"time"
)

func (s *Store) migrateClaiming() error {
	stmts := []string{
		`ALTER TABLE tickets ADD COLUMN claimed_by TEXT`,
		`ALTER TABLE tickets ADD COLUMN claimed_at TEXT`,
		`ALTER TABLE tickets ADD COLUMN evidence TEXT`,
	}
	for _, stmt := range stmts {
		_, err := s.db.Exec(stmt)
		if err != nil && !isAlterColumnExists(err) {
			return fmt.Errorf("migrate claiming: %w", err)
		}
	}
	return nil
}

func isAlterColumnExists(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "duplicate column") || contains(msg, "already exists")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

type ClaimResult struct {
	Success    bool   `json:"success"`
	TicketID   string `json:"ticket_id"`
	ClaimedBy  string `json:"claimed_by"`
	ConflictBy string `json:"conflict_by,omitempty"`
}

func (s *Store) ClaimTicket(ticketID, agentID string) (ClaimResult, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return ClaimResult{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var currentClaim sql.NullString
	err = tx.QueryRow(
		`SELECT claimed_by FROM tickets WHERE id = ?`, ticketID,
	).Scan(&currentClaim)
	if err != nil {
		return ClaimResult{}, fmt.Errorf("ticket %q not found: %w", ticketID, err)
	}

	if currentClaim.Valid && currentClaim.String != "" && currentClaim.String != agentID {
		return ClaimResult{
			Success:    false,
			TicketID:   ticketID,
			ClaimedBy:  currentClaim.String,
			ConflictBy: agentID,
		}, nil
	}

	now := formatTime(time.Now())
	_, err = tx.Exec(
		`UPDATE tickets SET claimed_by = ?, claimed_at = ?, status = ?, updated_at = ?
		 WHERE id = ?`,
		agentID, now, StatusInProgress, now, ticketID,
	)
	if err != nil {
		return ClaimResult{}, err
	}

	_, err = tx.Exec(
		`INSERT INTO ticket_transitions (ticket_id, from_status, to_status, agent_id, note, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		ticketID, StatusReady, StatusInProgress, agentID, "claimed", now,
	)
	if err != nil {
		return ClaimResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return ClaimResult{}, err
	}

	return ClaimResult{
		Success:   true,
		TicketID:  ticketID,
		ClaimedBy: agentID,
	}, nil
}

func (s *Store) CompleteTicket(ticketID, agentID, evidence string) error {
	now := formatTime(time.Now())
	res, err := s.db.Exec(
		`UPDATE tickets SET status = ?, evidence = ?, updated_at = ?
		 WHERE id = ? AND claimed_by = ?`,
		StatusDone, evidence, now, ticketID, agentID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("ticket %q not claimed by %q", ticketID, agentID)
	}

	_, err = s.db.Exec(
		`INSERT INTO ticket_transitions (ticket_id, from_status, to_status, agent_id, note, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		ticketID, StatusInProgress, StatusDone, agentID, evidence, now,
	)
	return err
}

func (s *Store) ReleaseStaleClaims(expiry time.Duration) (int64, error) {
	cutoff := time.Now().Add(-expiry)
	res, err := s.db.Exec(
		`UPDATE tickets SET claimed_by = NULL, claimed_at = NULL, status = ?
		 WHERE claimed_by IS NOT NULL AND claimed_at < ? AND status = ?`,
		StatusReady, formatTime(cutoff), StatusInProgress,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ReleaseNullClaims resets tickets that are stuck in in_progress with no claimed_by.
// This handles corruption from direct DB writes or race conditions that bypass ClaimTicket.
func (s *Store) ReleaseNullClaims() (int64, error) {
	res, err := s.db.Exec(
		`UPDATE tickets SET status = ?, updated_at = datetime('now')
		 WHERE status = ? AND (claimed_by IS NULL OR claimed_by = '')`,
		StatusBacklog, StatusInProgress,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
