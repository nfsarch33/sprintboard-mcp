package sprintboard

import (
	"errors"
	"fmt"
	"time"
)

// TicketComment is a single audit-log style comment attached to a ticket.
// Used by agents to record sprint signal that doesn't deserve a status
// transition (e.g. "switched embedder to ollama, still investigating").
type TicketComment struct {
	ID        int64     `json:"id"`
	TicketID  string    `json:"ticket_id"`
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// migrateComments creates the ticket_comments table on first open. Idempotent.
func (s *Store) migrateComments() error {
	schema := `
	CREATE TABLE IF NOT EXISTS ticket_comments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ticket_id TEXT NOT NULL,
		author TEXT NOT NULL,
		body TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY (ticket_id) REFERENCES tickets(id)
	);
	CREATE INDEX IF NOT EXISTS idx_ticket_comments_ticket
		ON ticket_comments(ticket_id, created_at);
	`
	if _, err := s.db.ExecDDL(schema); err != nil {
		return fmt.Errorf("migrate ticket_comments: %w", err)
	}
	return nil
}

// AddTicketComment appends a new comment for ticket_id. Empty inputs are
// rejected so the audit log doesn't accumulate noise rows.
func (s *Store) AddTicketComment(ticketID, author, body string) (TicketComment, error) {
	if ticketID == "" {
		return TicketComment{}, errors.New("sprintboard: ticket_id is required")
	}
	if author == "" {
		return TicketComment{}, errors.New("sprintboard: author is required")
	}
	if body == "" {
		return TicketComment{}, errors.New("sprintboard: body is required")
	}

	now := time.Now()
	id, err := s.insertReturningID(
		`INSERT INTO ticket_comments (ticket_id, author, body, created_at)
		 VALUES (?, ?, ?, ?)`,
		ticketID, author, body, formatTime(now),
	)
	if err != nil {
		return TicketComment{}, fmt.Errorf("insert ticket_comment: %w", err)
	}
	return TicketComment{
		ID:        id,
		TicketID:  ticketID,
		Author:    author,
		Body:      body,
		CreatedAt: now,
	}, nil
}

// ListTicketComments returns every comment for ticketID in chronological
// order. A ticket with no comments returns (nil, nil); the caller should
// treat that as an empty slice.
func (s *Store) ListTicketComments(ticketID string) ([]TicketComment, error) {
	rows, err := s.db.Query(
		`SELECT id, ticket_id, author, body, created_at
		 FROM ticket_comments
		 WHERE ticket_id = ?
		 ORDER BY id ASC`,
		ticketID,
	)
	if err != nil {
		return nil, fmt.Errorf("query ticket_comments: %w", err)
	}
	defer rows.Close()

	var out []TicketComment
	for rows.Next() {
		var c TicketComment
		var created string
		if err := rows.Scan(&c.ID, &c.TicketID, &c.Author, &c.Body, &created); err != nil {
			return nil, fmt.Errorf("scan ticket_comment: %w", err)
		}
		c.CreatedAt = parseTime(created)
		out = append(out, c)
	}
	return out, rows.Err()
}
