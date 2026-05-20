package sprintboard

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type Agent struct {
	ID              string    `json:"id"`
	Surface         string    `json:"surface"`
	Capabilities    string    `json:"capabilities,omitempty"`
	LastSeen        time.Time `json:"last_seen"`
	CurrentTicketID string    `json:"current_ticket_id,omitempty"`
	RegisteredAt    time.Time `json:"registered_at"`
}

const agentHeartbeatExpiry = 30 * time.Minute

func (s *Store) migrateAgents() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS agents (
			id TEXT PRIMARY KEY,
			surface TEXT NOT NULL,
			capabilities TEXT,
			last_seen TEXT NOT NULL,
			current_ticket_id TEXT,
			registered_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_agents_surface ON agents(surface);
	`)
	if err != nil {
		return err
	}
	addCol := `ALTER TABLE agents ADD COLUMN preferences TEXT`
	if _, err := s.db.Exec(addCol); err != nil && !isAlterColumnExists(err) {
		return err
	}
	return nil
}

func (s *Store) RegisterAgent(a Agent) error {
	if a.ID == "" {
		return fmt.Errorf("agent id is required")
	}
	if a.Surface == "" {
		return fmt.Errorf("agent surface is required")
	}
	now := time.Now()
	if a.RegisteredAt.IsZero() {
		a.RegisteredAt = now
	}
	a.LastSeen = now

	_, err := s.db.Exec(
		`INSERT INTO agents (id, surface, capabilities, last_seen, current_ticket_id, registered_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
			surface = excluded.surface,
			capabilities = excluded.capabilities,
			last_seen = excluded.last_seen`,
		a.ID, a.Surface, a.Capabilities, formatTime(a.LastSeen),
		a.CurrentTicketID, formatTime(a.RegisteredAt),
	)
	return err
}

func (s *Store) AgentHeartbeat(agentID string, currentTicketID string) error {
	res, err := s.db.Exec(
		`UPDATE agents SET last_seen = ?, current_ticket_id = ? WHERE id = ?`,
		formatTime(time.Now()), currentTicketID, agentID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("agent %q not registered", agentID)
	}
	return nil
}

func (s *Store) ListActiveAgents() ([]Agent, error) {
	cutoff := time.Now().Add(-agentHeartbeatExpiry)
	rows, err := s.db.Query(
		`SELECT id, surface, capabilities, last_seen, current_ticket_id, registered_at
		 FROM agents WHERE last_seen >= ? ORDER BY last_seen DESC`,
		formatTime(cutoff),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAgents(rows)
}

func (s *Store) ListAllAgents() ([]Agent, error) {
	rows, err := s.db.Query(
		`SELECT id, surface, capabilities, last_seen, current_ticket_id, registered_at
		 FROM agents ORDER BY last_seen DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAgents(rows)
}

func (s *Store) ExpireStaleAgents() (int64, error) {
	cutoff := time.Now().Add(-agentHeartbeatExpiry)
	res, err := s.db.Exec(
		`DELETE FROM agents WHERE last_seen < ?`,
		formatTime(cutoff),
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) GetAgent(id string) (*Agent, error) {
	row := s.db.QueryRow(
		`SELECT id, surface, capabilities, last_seen, current_ticket_id, registered_at
		 FROM agents WHERE id = ?`, id,
	)
	a := &Agent{}
	var lastSeen, regAt string
	var caps, ticket sql.NullString
	err := row.Scan(&a.ID, &a.Surface, &caps, &lastSeen, &ticket, &regAt)
	if err != nil {
		return nil, err
	}
	a.LastSeen, _ = time.Parse(time.RFC3339, lastSeen)
	a.RegisteredAt, _ = time.Parse(time.RFC3339, regAt)
	if caps.Valid {
		a.Capabilities = caps.String
	}
	if ticket.Valid {
		a.CurrentTicketID = ticket.String
	}
	return a, nil
}

func (s *Store) UpdatePreferences(agentID string, prefs map[string]string) error {
	data, err := json.Marshal(prefs)
	if err != nil {
		return fmt.Errorf("marshal preferences: %w", err)
	}
	res, err := s.db.Exec(`UPDATE agents SET preferences = ? WHERE id = ?`, string(data), agentID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("agent %q not registered", agentID)
	}
	return nil
}

func (s *Store) GetPreferences(agentID string) (map[string]string, error) {
	var raw sql.NullString
	err := s.db.QueryRow(`SELECT preferences FROM agents WHERE id = ?`, agentID).Scan(&raw)
	if err != nil {
		return nil, err
	}
	if !raw.Valid || raw.String == "" {
		return map[string]string{}, nil
	}
	var prefs map[string]string
	if err := json.Unmarshal([]byte(raw.String), &prefs); err != nil {
		return nil, err
	}
	return prefs, nil
}

func scanAgents(rows *sql.Rows) ([]Agent, error) {
	var agents []Agent
	for rows.Next() {
		var a Agent
		var lastSeen, regAt string
		var caps, ticket sql.NullString
		if err := rows.Scan(&a.ID, &a.Surface, &caps, &lastSeen, &ticket, &regAt); err != nil {
			return nil, err
		}
		a.LastSeen, _ = time.Parse(time.RFC3339, lastSeen)
		a.RegisteredAt, _ = time.Parse(time.RFC3339, regAt)
		if caps.Valid {
			a.Capabilities = caps.String
		}
		if ticket.Valid {
			a.CurrentTicketID = ticket.String
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}
