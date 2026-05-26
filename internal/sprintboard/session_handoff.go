package sprintboard

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type SessionHandoff struct {
	ID           string    `json:"id"`
	SessionID    string    `json:"session_id"`
	AgentID      string    `json:"agent_id"`
	SprintID     string    `json:"sprint_id,omitempty"`
	Summary      string    `json:"summary"`
	CarryForward string    `json:"carry_forward,omitempty"`
	Blockers     string    `json:"blockers,omitempty"`
	Commits      string    `json:"commits,omitempty"`
	ReposPushed  string    `json:"repos_pushed,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

func (s *Store) migrateSessionHandoffs() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS session_handoffs (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			sprint_id TEXT,
			summary TEXT NOT NULL,
			carry_forward TEXT,
			blockers TEXT,
			commits TEXT,
			repos_pushed TEXT,
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_session_handoffs_agent ON session_handoffs(agent_id)`,
		`CREATE INDEX IF NOT EXISTS idx_session_handoffs_created ON session_handoffs(created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_session_handoffs_sprint ON session_handoffs(sprint_id)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate session_handoffs: %w", err)
		}
	}
	return nil
}

func (s *Store) StoreSessionHandoff(h SessionHandoff) error {
	if h.ID == "" {
		return fmt.Errorf("handoff id required")
	}
	if h.SessionID == "" {
		return fmt.Errorf("session_id required")
	}
	if h.AgentID == "" {
		return fmt.Errorf("agent_id required")
	}
	if h.Summary == "" {
		return fmt.Errorf("summary required")
	}
	now := time.Now().Format(time.RFC3339)
	_, err := s.db.Exec(
		`INSERT INTO session_handoffs (id, session_id, agent_id, sprint_id, summary, carry_forward, blockers, commits, repos_pushed, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		h.ID, h.SessionID, h.AgentID, h.SprintID, h.Summary, h.CarryForward, h.Blockers, h.Commits, h.ReposPushed, now,
	)
	return err
}

func (s *Store) LatestSessionHandoffs(agentID string, limit int) ([]SessionHandoff, error) {
	if limit <= 0 {
		limit = 3
	}
	if limit > 20 {
		limit = 20
	}

	var rows *sql.Rows
	var err error
	if agentID != "" {
		rows, err = s.db.Query(
			`SELECT id, session_id, agent_id, COALESCE(sprint_id,''), summary, COALESCE(carry_forward,''), COALESCE(blockers,''), COALESCE(commits,''), COALESCE(repos_pushed,''), created_at
			 FROM session_handoffs WHERE agent_id = ? ORDER BY created_at DESC LIMIT ?`,
			agentID, limit,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT id, session_id, agent_id, COALESCE(sprint_id,''), summary, COALESCE(carry_forward,''), COALESCE(blockers,''), COALESCE(commits,''), COALESCE(repos_pushed,''), created_at
			 FROM session_handoffs ORDER BY created_at DESC LIMIT ?`,
			limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var handoffs []SessionHandoff
	for rows.Next() {
		var h SessionHandoff
		var createdStr string
		if err := rows.Scan(&h.ID, &h.SessionID, &h.AgentID, &h.SprintID, &h.Summary, &h.CarryForward, &h.Blockers, &h.Commits, &h.ReposPushed, &createdStr); err != nil {
			return nil, err
		}
		h.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		handoffs = append(handoffs, h)
	}
	return handoffs, rows.Err()
}

func (s *Store) SearchSessionHandoffs(query string, sinceDate string) ([]SessionHandoff, error) {
	if query == "" {
		return nil, fmt.Errorf("query required")
	}

	q := "%" + strings.ToLower(query) + "%"
	var rows *sql.Rows
	var err error

	if sinceDate != "" {
		rows, err = s.db.Query(
			`SELECT id, session_id, agent_id, COALESCE(sprint_id,''), summary, COALESCE(carry_forward,''), COALESCE(blockers,''), COALESCE(commits,''), COALESCE(repos_pushed,''), created_at
			 FROM session_handoffs
			 WHERE (LOWER(summary) LIKE ? OR LOWER(carry_forward) LIKE ? OR LOWER(blockers) LIKE ?)
			   AND created_at >= ?
			 ORDER BY created_at DESC LIMIT 20`,
			q, q, q, sinceDate,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT id, session_id, agent_id, COALESCE(sprint_id,''), summary, COALESCE(carry_forward,''), COALESCE(blockers,''), COALESCE(commits,''), COALESCE(repos_pushed,''), created_at
			 FROM session_handoffs
			 WHERE LOWER(summary) LIKE ? OR LOWER(carry_forward) LIKE ? OR LOWER(blockers) LIKE ?
			 ORDER BY created_at DESC LIMIT 20`,
			q, q, q,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var handoffs []SessionHandoff
	for rows.Next() {
		var h SessionHandoff
		var createdStr string
		if err := rows.Scan(&h.ID, &h.SessionID, &h.AgentID, &h.SprintID, &h.Summary, &h.CarryForward, &h.Blockers, &h.Commits, &h.ReposPushed, &createdStr); err != nil {
			return nil, err
		}
		h.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		handoffs = append(handoffs, h)
	}
	return handoffs, rows.Err()
}
