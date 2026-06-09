package sprintboard

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// TerminalSessionEvent stores post-shell terminal observability rows (Phase 3 Part B).
type TerminalSessionEvent struct {
	ID           int64     `json:"id"`
	Host         string    `json:"host"`
	SessionID    string    `json:"session_id"`
	CommandClass string    `json:"command_class"`
	ExitCode     *int      `json:"exit_code,omitempty"`
	DurationMs   int64     `json:"duration_ms"`
	Status       string    `json:"status"`
	Payload      any       `json:"payload"`
	PayloadJSON  []byte    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

func (s *Store) migrateTerminalSessionEvents() error {
	schema := `
	CREATE TABLE IF NOT EXISTS terminal_session_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		host TEXT NOT NULL,
		session_id TEXT NOT NULL,
		command_class TEXT NOT NULL,
		exit_code INTEGER,
		duration_ms INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL,
		payload TEXT NOT NULL,
		created_at TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_terminal_session_events_host_created
		ON terminal_session_events(host, created_at);
	CREATE INDEX IF NOT EXISTS idx_terminal_session_events_session
		ON terminal_session_events(session_id);
	`
	if _, err := s.db.ExecDDL(schema); err != nil {
		return fmt.Errorf("migrate terminal_session_events: %w", err)
	}
	return nil
}

// InsertTerminalSessionEvent persists one shell session completion row.
func (s *Store) InsertTerminalSessionEvent(ev TerminalSessionEvent) (int64, error) {
	if ev.Host == "" {
		return 0, fmt.Errorf("host is required")
	}
	if ev.SessionID == "" {
		return 0, fmt.Errorf("session_id is required")
	}
	if ev.CommandClass == "" {
		return 0, fmt.Errorf("command_class is required")
	}
	if ev.Status == "" {
		return 0, fmt.Errorf("status is required")
	}
	if ev.Payload == nil && len(ev.PayloadJSON) == 0 {
		return 0, fmt.Errorf("payload is required")
	}

	payloadJSON := ev.PayloadJSON
	if len(payloadJSON) == 0 {
		var err error
		payloadJSON, err = json.Marshal(ev.Payload)
		if err != nil {
			return 0, fmt.Errorf("marshal payload: %w", err)
		}
	}

	now := time.Now().UTC()
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = now
	}

	res, err := s.db.Exec(
		`INSERT INTO terminal_session_events (host, session_id, command_class, exit_code, duration_ms, status, payload, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		ev.Host, ev.SessionID, ev.CommandClass, ev.ExitCode, ev.DurationMs, ev.Status,
		string(payloadJSON), formatTime(ev.CreatedAt),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetTerminalSessionEvent returns one event by id.
func (s *Store) GetTerminalSessionEvent(id int64) (TerminalSessionEvent, error) {
	var ev TerminalSessionEvent
	var createdAt, payload sql.NullString
	var exitCode sql.NullInt64
	err := s.db.QueryRow(
		`SELECT id, host, session_id, command_class, exit_code, duration_ms, status, payload, created_at
		 FROM terminal_session_events WHERE id = ?`, id,
	).Scan(&ev.ID, &ev.Host, &ev.SessionID, &ev.CommandClass, &exitCode, &ev.DurationMs, &ev.Status, &payload, &createdAt)
	if err != nil {
		return TerminalSessionEvent{}, fmt.Errorf("terminal session event %d: %w", id, err)
	}
	if exitCode.Valid {
		code := int(exitCode.Int64)
		ev.ExitCode = &code
	}
	ev.CreatedAt = parseTime(nullString(createdAt))
	ev.PayloadJSON = []byte(nullString(payload))
	return ev, nil
}

// ListTerminalSessionEvents returns events with created_at >= since, newest first.
func (s *Store) ListTerminalSessionEvents(host string, since time.Time) ([]TerminalSessionEvent, error) {
	query := `SELECT id, host, session_id, command_class, exit_code, duration_ms, status, payload, created_at
		FROM terminal_session_events WHERE created_at >= ?`
	args := []any{formatTime(since)}
	if host != "" {
		query += ` AND host = ?`
		args = append(args, host)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TerminalSessionEvent
	for rows.Next() {
		var ev TerminalSessionEvent
		var createdAt, payload sql.NullString
		var exitCode sql.NullInt64
		if err := rows.Scan(&ev.ID, &ev.Host, &ev.SessionID, &ev.CommandClass, &exitCode, &ev.DurationMs, &ev.Status, &payload, &createdAt); err != nil {
			return nil, err
		}
		if exitCode.Valid {
			code := int(exitCode.Int64)
			ev.ExitCode = &code
		}
		ev.CreatedAt = parseTime(nullString(createdAt))
		ev.PayloadJSON = []byte(nullString(payload))
		out = append(out, ev)
	}
	return out, rows.Err()
}
