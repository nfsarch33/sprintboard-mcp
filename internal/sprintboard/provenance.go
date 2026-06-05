package sprintboard

import (
	"fmt"
	"time"
)

func (s *Store) migrateProvenance() error {
	stmts := []string{
		`ALTER TABLE tickets ADD COLUMN branch TEXT`,
		`ALTER TABLE tickets ADD COLUMN pr_url TEXT`,
		`ALTER TABLE tickets ADD COLUMN merged_at TEXT`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil && !isAlterColumnExists(err) {
			return fmt.Errorf("migrate provenance: %w", err)
		}
	}
	return nil
}

func (s *Store) updateTicketBranch(ticketID, branch string) error {
	if branch == "" {
		return nil
	}
	res, err := s.db.Exec(
		`UPDATE tickets SET branch = ?, updated_at = ? WHERE id = ?`,
		branch, formatTime(time.Now()), ticketID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("ticket %q not found", ticketID)
	}
	return nil
}

func (s *Store) updateTicketProvenance(ticketID, branch, prURL string) error {
	if branch == "" && prURL == "" {
		return nil
	}
	now := formatTime(time.Now())
	res, err := s.db.Exec(
		`UPDATE tickets SET branch = COALESCE(NULLIF(?, ''), branch),
		    pr_url = COALESCE(NULLIF(?, ''), pr_url), updated_at = ?
		 WHERE id = ?`,
		branch, prURL, now, ticketID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("ticket %q not found", ticketID)
	}
	return nil
}

// SetTicketMergedAt records when a ticket's PR was merged.
func (s *Store) SetTicketMergedAt(ticketID string, mergedAt time.Time) error {
	if mergedAt.IsZero() {
		mergedAt = time.Now()
	}
	res, err := s.db.Exec(
		`UPDATE tickets SET merged_at = ?, updated_at = ? WHERE id = ?`,
		formatTime(mergedAt), formatTime(time.Now()), ticketID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("ticket %q not found", ticketID)
	}
	return nil
}
