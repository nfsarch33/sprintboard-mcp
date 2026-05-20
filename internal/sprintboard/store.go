package sprintboard

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type TicketStatus string

const (
	StatusBacklog      TicketStatus = "backlog"
	StatusReady        TicketStatus = "ready"
	StatusInProgress   TicketStatus = "in_progress"
	StatusReview       TicketStatus = "review"
	StatusDone         TicketStatus = "done"
	StatusBlocked      TicketStatus = "blocked"
	StatusReadyHandoff TicketStatus = "ready_for_handoff"
)

type SprintStatus string

const (
	SprintPlanned SprintStatus = "planned"
	SprintActive  SprintStatus = "active"
	SprintClosed  SprintStatus = "closed"
)

type Sprint struct {
	ID         string       `json:"id"`
	Name       string       `json:"name"`
	Status     SprintStatus `json:"status"`
	OwnerAgent string       `json:"owner_agent"`
	Theme      string       `json:"theme,omitempty"`
	StartAt    time.Time    `json:"start_at,omitempty"`
	EndAt      time.Time    `json:"end_at,omitempty"`
	CreatedAt  time.Time    `json:"created_at"`
}

type Ticket struct {
	ID                 string       `json:"id"`
	SprintID           string       `json:"sprint_id,omitempty"`
	Title              string       `json:"title"`
	Description        string       `json:"description,omitempty"`
	Status             TicketStatus `json:"status"`
	OwnerAgent         string       `json:"owner_agent,omitempty"`
	Priority           int          `json:"priority"`
	AcceptanceCriteria string       `json:"acceptance_criteria,omitempty"`
	HandoffDocPath     string       `json:"handoff_doc_path,omitempty"`
	CreatedAt          time.Time    `json:"created_at"`
	UpdatedAt          time.Time    `json:"updated_at"`
}

type Transition struct {
	ID         int64        `json:"id"`
	TicketID   string       `json:"ticket_id"`
	FromStatus TicketStatus `json:"from_status"`
	ToStatus   TicketStatus `json:"to_status"`
	AgentID    string       `json:"agent_id"`
	Note       string       `json:"note,omitempty"`
	Timestamp  time.Time    `json:"timestamp"`
}

type Handoff struct {
	ID          int64     `json:"id"`
	TicketID    string    `json:"ticket_id"`
	FromAgent   string    `json:"from_agent"`
	ToAgent     string    `json:"to_agent"`
	ContextPath string    `json:"context_path,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type Store struct {
	db *sql.DB
}

func DefaultDBPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "helix-dev-tools", "sprintboard.db")
}

func Open(dbPath string) (*Store, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL: %w", err)
	}

	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set busy_timeout: %w", err)
	}

	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS sprints (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'planned',
		owner_agent TEXT,
		theme TEXT,
		start_at TEXT,
		end_at TEXT,
		created_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS tickets (
		id TEXT PRIMARY KEY,
		sprint_id TEXT,
		title TEXT NOT NULL,
		description TEXT,
		status TEXT NOT NULL DEFAULT 'backlog',
		owner_agent TEXT,
		priority INTEGER DEFAULT 0,
		acceptance_criteria TEXT,
		handoff_doc_path TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		FOREIGN KEY (sprint_id) REFERENCES sprints(id)
	);

	CREATE TABLE IF NOT EXISTS ticket_transitions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ticket_id TEXT NOT NULL,
		from_status TEXT NOT NULL,
		to_status TEXT NOT NULL,
		agent_id TEXT,
		note TEXT,
		timestamp TEXT NOT NULL,
		FOREIGN KEY (ticket_id) REFERENCES tickets(id)
	);

	CREATE TABLE IF NOT EXISTS handoffs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ticket_id TEXT NOT NULL,
		from_agent TEXT NOT NULL,
		to_agent TEXT NOT NULL,
		context_path TEXT,
		created_at TEXT NOT NULL,
		FOREIGN KEY (ticket_id) REFERENCES tickets(id)
	);

	CREATE INDEX IF NOT EXISTS idx_tickets_sprint ON tickets(sprint_id);
	CREATE INDEX IF NOT EXISTS idx_tickets_status ON tickets(status);
	CREATE INDEX IF NOT EXISTS idx_tickets_owner ON tickets(owner_agent);
	CREATE INDEX IF NOT EXISTS idx_transitions_ticket ON ticket_transitions(ticket_id);
	CREATE INDEX IF NOT EXISTS idx_handoffs_ticket ON handoffs(ticket_id);
	`

	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	if err := s.migrateVectors(); err != nil {
		return err
	}
	if err := s.migrateAgents(); err != nil {
		return err
	}
	if err := s.migrateClaiming(); err != nil {
		return err
	}
	return s.migrateDAG()
}

func (s *Store) CreateSprint(sp Sprint) error {
	if sp.CreatedAt.IsZero() {
		sp.CreatedAt = time.Now()
	}
	if sp.Status == "" {
		sp.Status = SprintPlanned
	}

	_, err := s.db.Exec(
		`INSERT INTO sprints (id, name, status, owner_agent, theme, start_at, end_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sp.ID, sp.Name, sp.Status, sp.OwnerAgent, sp.Theme,
		formatTime(sp.StartAt), formatTime(sp.EndAt), formatTime(sp.CreatedAt),
	)
	return err
}

func (s *Store) ListSprints() ([]Sprint, error) {
	rows, err := s.db.Query(`SELECT id, name, status, owner_agent, theme, start_at, end_at, created_at FROM sprints ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sprints []Sprint
	for rows.Next() {
		var sp Sprint
		var startAt, endAt, createdAt sql.NullString
		err := rows.Scan(&sp.ID, &sp.Name, &sp.Status, &sp.OwnerAgent, &sp.Theme, &startAt, &endAt, &createdAt)
		if err != nil {
			return nil, err
		}
		sp.StartAt = parseTime(startAt.String)
		sp.EndAt = parseTime(endAt.String)
		sp.CreatedAt = parseTime(createdAt.String)
		sprints = append(sprints, sp)
	}
	return sprints, rows.Err()
}

func (s *Store) GetSprint(id string) (Sprint, error) {
	var sp Sprint
	var startAt, endAt, createdAt sql.NullString
	err := s.db.QueryRow(
		`SELECT id, name, status, owner_agent, theme, start_at, end_at, created_at FROM sprints WHERE id = ?`, id,
	).Scan(&sp.ID, &sp.Name, &sp.Status, &sp.OwnerAgent, &sp.Theme, &startAt, &endAt, &createdAt)
	if err != nil {
		return Sprint{}, fmt.Errorf("sprint %q not found: %w", id, err)
	}
	sp.StartAt = parseTime(startAt.String)
	sp.EndAt = parseTime(endAt.String)
	sp.CreatedAt = parseTime(createdAt.String)
	return sp, nil
}

func (s *Store) CreateTicket(t Ticket) error {
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}
	if t.UpdatedAt.IsZero() {
		t.UpdatedAt = t.CreatedAt
	}
	if t.Status == "" {
		t.Status = StatusBacklog
	}

	_, err := s.db.Exec(
		`INSERT INTO tickets (id, sprint_id, title, description, status, owner_agent, priority, acceptance_criteria, handoff_doc_path, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.SprintID, t.Title, t.Description, t.Status, t.OwnerAgent,
		t.Priority, t.AcceptanceCriteria, t.HandoffDocPath,
		formatTime(t.CreatedAt), formatTime(t.UpdatedAt),
	)
	return err
}

func (s *Store) ListTickets(sprintID string) ([]Ticket, error) {
	query := `SELECT id, sprint_id, title, description, status, owner_agent, priority, acceptance_criteria, handoff_doc_path, created_at, updated_at FROM tickets`
	var args []interface{}
	if sprintID != "" {
		query += ` WHERE sprint_id = ?`
		args = append(args, sprintID)
	}
	query += ` ORDER BY priority DESC, created_at ASC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickets []Ticket
	for rows.Next() {
		var t Ticket
		var createdAt, updatedAt string
		err := rows.Scan(&t.ID, &t.SprintID, &t.Title, &t.Description, &t.Status,
			&t.OwnerAgent, &t.Priority, &t.AcceptanceCriteria, &t.HandoffDocPath,
			&createdAt, &updatedAt)
		if err != nil {
			return nil, err
		}
		t.CreatedAt = parseTime(createdAt)
		t.UpdatedAt = parseTime(updatedAt)
		tickets = append(tickets, t)
	}
	return tickets, rows.Err()
}

func (s *Store) UpdateTicket(id string, status TicketStatus, agentID string, note string) error {
	var oldStatus string
	err := s.db.QueryRow(`SELECT status FROM tickets WHERE id = ?`, id).Scan(&oldStatus)
	if err != nil {
		return fmt.Errorf("ticket %q not found: %w", id, err)
	}

	now := time.Now()
	_, err = s.db.Exec(`UPDATE tickets SET status = ?, updated_at = ? WHERE id = ?`,
		status, formatTime(now), id)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(
		`INSERT INTO ticket_transitions (ticket_id, from_status, to_status, agent_id, note, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, oldStatus, status, agentID, note, formatTime(now),
	)
	return err
}

func (s *Store) AssignTicket(id string, agent string) error {
	res, err := s.db.Exec(`UPDATE tickets SET owner_agent = ?, updated_at = ? WHERE id = ?`,
		agent, formatTime(time.Now()), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("ticket %q not found", id)
	}
	return nil
}

func (s *Store) CreateHandoff(h Handoff) error {
	if h.CreatedAt.IsZero() {
		h.CreatedAt = time.Now()
	}
	_, err := s.db.Exec(
		`INSERT INTO handoffs (ticket_id, from_agent, to_agent, context_path, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		h.TicketID, h.FromAgent, h.ToAgent, h.ContextPath, formatTime(h.CreatedAt),
	)
	return err
}

func (s *Store) ListHandoffs(ticketID string) ([]Handoff, error) {
	rows, err := s.db.Query(
		`SELECT id, ticket_id, from_agent, to_agent, context_path, created_at FROM handoffs WHERE ticket_id = ? ORDER BY created_at DESC`,
		ticketID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var handoffs []Handoff
	for rows.Next() {
		var h Handoff
		var createdAt string
		err := rows.Scan(&h.ID, &h.TicketID, &h.FromAgent, &h.ToAgent, &h.ContextPath, &createdAt)
		if err != nil {
			return nil, err
		}
		h.CreatedAt = parseTime(createdAt)
		handoffs = append(handoffs, h)
	}
	return handoffs, rows.Err()
}

func (s *Store) ListHandoffsByAgent(agentID string) ([]Handoff, error) {
	rows, err := s.db.Query(
		`SELECT id, ticket_id, from_agent, to_agent, context_path, created_at FROM handoffs WHERE to_agent = ? OR from_agent = ? ORDER BY created_at DESC`,
		agentID, agentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var handoffs []Handoff
	for rows.Next() {
		var h Handoff
		var createdAt string
		err := rows.Scan(&h.ID, &h.TicketID, &h.FromAgent, &h.ToAgent, &h.ContextPath, &createdAt)
		if err != nil {
			return nil, err
		}
		h.CreatedAt = parseTime(createdAt)
		handoffs = append(handoffs, h)
	}
	return handoffs, rows.Err()
}

type SprintSummary struct {
	Sprint          Sprint               `json:"sprint"`
	TicketsByStatus map[TicketStatus]int `json:"tickets_by_status"`
	TotalTickets    int                  `json:"total_tickets"`
}

func (s *Store) UpdateSprint(id string, status SprintStatus) error {
	res, err := s.db.Exec(`UPDATE sprints SET status = ? WHERE id = ?`, status, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("sprint %q not found", id)
	}
	return nil
}

func (s *Store) DeleteSprint(id string) error {
	var ticketCount int
	s.db.QueryRow(`SELECT COUNT(*) FROM tickets WHERE sprint_id = ?`, id).Scan(&ticketCount)
	if ticketCount > 0 {
		return fmt.Errorf("sprint %q has %d tickets; remove them first", id, ticketCount)
	}

	res, err := s.db.Exec(`DELETE FROM sprints WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("sprint %q not found", id)
	}
	return nil
}

func (s *Store) DeleteTicket(id string) error {
	res, err := s.db.Exec(`DELETE FROM tickets WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("ticket %q not found", id)
	}
	s.db.Exec(`DELETE FROM ticket_transitions WHERE ticket_id = ?`, id)
	s.db.Exec(`DELETE FROM handoffs WHERE ticket_id = ?`, id)
	return nil
}

func (s *Store) SprintSummary(id string) (SprintSummary, error) {
	sp, err := s.GetSprint(id)
	if err != nil {
		return SprintSummary{}, err
	}

	rows, err := s.db.Query(`SELECT status, COUNT(*) FROM tickets WHERE sprint_id = ? GROUP BY status`, id)
	if err != nil {
		return SprintSummary{}, err
	}
	defer rows.Close()

	summary := SprintSummary{
		Sprint:          sp,
		TicketsByStatus: make(map[TicketStatus]int),
	}

	for rows.Next() {
		var status TicketStatus
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return SprintSummary{}, err
		}
		summary.TicketsByStatus[status] = count
		summary.TotalTickets += count
	}
	return summary, rows.Err()
}

func (s *Store) ListTransitions(ticketID string) ([]Transition, error) {
	rows, err := s.db.Query(
		`SELECT id, ticket_id, from_status, to_status, agent_id, note, timestamp
		 FROM ticket_transitions WHERE ticket_id = ? ORDER BY timestamp ASC`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transitions []Transition
	for rows.Next() {
		var t Transition
		var ts string
		if err := rows.Scan(&t.ID, &t.TicketID, &t.FromStatus, &t.ToStatus, &t.AgentID, &t.Note, &ts); err != nil {
			return nil, err
		}
		t.Timestamp = parseTime(ts)
		transitions = append(transitions, t)
	}
	return transitions, rows.Err()
}

func (s *Store) GetTicket(id string) (Ticket, error) {
	var t Ticket
	var createdAt, updatedAt string
	err := s.db.QueryRow(
		`SELECT id, sprint_id, title, description, status, owner_agent, priority, acceptance_criteria, handoff_doc_path, created_at, updated_at
		 FROM tickets WHERE id = ?`, id,
	).Scan(&t.ID, &t.SprintID, &t.Title, &t.Description, &t.Status,
		&t.OwnerAgent, &t.Priority, &t.AcceptanceCriteria, &t.HandoffDocPath,
		&createdAt, &updatedAt)
	if err != nil {
		return Ticket{}, fmt.Errorf("ticket %q not found: %w", id, err)
	}
	t.CreatedAt = parseTime(createdAt)
	t.UpdatedAt = parseTime(updatedAt)
	return t, nil
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339, s)
	return t
}
