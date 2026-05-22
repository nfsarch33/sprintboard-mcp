// T-8800-B13: SprintTemplate persistence + one-shot instantiation.
//
// A template is a named bundle of TicketTemplate rows that can be turned into
// a real Sprint plus its Tickets in one transactional call. Re-using
// existing CreateSprint/CreateTicket would race with other writers, so the
// instantiation runs in a single sql.Tx and rolls back atomically.
package sprintboard

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// SprintTemplate is the operator-facing description of a reusable sprint.
type SprintTemplate struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Theme       string           `json:"theme,omitempty"`
	Tickets     []TicketTemplate `json:"tickets"`
	CreatedAt   time.Time        `json:"created_at"`
}

// TicketTemplate is the per-row blueprint inside a SprintTemplate.
type TicketTemplate struct {
	Title              string   `json:"title"`
	Description        string   `json:"description,omitempty"`
	Priority           int      `json:"priority,omitempty"`
	OwnerAgent         string   `json:"owner_agent,omitempty"`
	AcceptanceCriteria string   `json:"acceptance_criteria,omitempty"`
	Labels             []string `json:"labels,omitempty"`
}

// SprintInstantiation is the response payload of InstantiateSprintFromTemplate
// so callers don't need a second round-trip to read back the new sprint.
type SprintInstantiation struct {
	Sprint  Sprint   `json:"sprint"`
	Tickets []Ticket `json:"tickets"`
}

// migrateTemplates is invoked by migrate() and is idempotent on re-open.
func (s *Store) migrateTemplates() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS sprint_templates (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			theme TEXT,
			tickets_json TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate templates: %w", err)
		}
	}
	return nil
}

// CreateSprintTemplate stores a template. ID and Name are required; CreatedAt
// is stamped automatically when zero.
func (s *Store) CreateSprintTemplate(tmpl SprintTemplate) error {
	if tmpl.ID == "" {
		return fmt.Errorf("template id required")
	}
	if tmpl.Name == "" {
		return fmt.Errorf("template name required")
	}
	if tmpl.CreatedAt.IsZero() {
		tmpl.CreatedAt = time.Now()
	}
	raw, err := json.Marshal(tmpl.Tickets)
	if err != nil {
		return fmt.Errorf("marshal tickets: %w", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO sprint_templates (id, name, description, theme, tickets_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		tmpl.ID, tmpl.Name, tmpl.Description, tmpl.Theme, string(raw), formatTime(tmpl.CreatedAt),
	)
	return err
}

// GetSprintTemplate returns the template for a given id.
func (s *Store) GetSprintTemplate(id string) (SprintTemplate, error) {
	var (
		tmpl                                SprintTemplate
		description, theme, ticketsRaw, ts  sql.NullString
	)
	err := s.db.QueryRow(
		`SELECT id, name, description, theme, tickets_json, created_at FROM sprint_templates WHERE id = ?`, id,
	).Scan(&tmpl.ID, &tmpl.Name, &description, &theme, &ticketsRaw, &ts)
	if err != nil {
		return SprintTemplate{}, fmt.Errorf("template %q not found: %w", id, err)
	}
	tmpl.Description = nullString(description)
	tmpl.Theme = nullString(theme)
	tmpl.CreatedAt = parseTime(nullString(ts))
	if raw := nullString(ticketsRaw); raw != "" {
		if err := json.Unmarshal([]byte(raw), &tmpl.Tickets); err != nil {
			return SprintTemplate{}, fmt.Errorf("unmarshal tickets: %w", err)
		}
	}
	return tmpl, nil
}

// ListSprintTemplates returns templates ordered by most-recent first.
func (s *Store) ListSprintTemplates() ([]SprintTemplate, error) {
	rows, err := s.db.Query(
		`SELECT id, name, description, theme, tickets_json, created_at FROM sprint_templates ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SprintTemplate
	for rows.Next() {
		var (
			tmpl                               SprintTemplate
			description, theme, ticketsRaw, ts sql.NullString
		)
		if err := rows.Scan(&tmpl.ID, &tmpl.Name, &description, &theme, &ticketsRaw, &ts); err != nil {
			return nil, err
		}
		tmpl.Description = nullString(description)
		tmpl.Theme = nullString(theme)
		tmpl.CreatedAt = parseTime(nullString(ts))
		if raw := nullString(ticketsRaw); raw != "" {
			if err := json.Unmarshal([]byte(raw), &tmpl.Tickets); err != nil {
				return nil, err
			}
		}
		out = append(out, tmpl)
	}
	return out, rows.Err()
}

// DeleteSprintTemplate removes a template. Returns an error if no row matches
// so callers can surface a clear 404 to the operator.
func (s *Store) DeleteSprintTemplate(id string) error {
	res, err := s.db.Exec(`DELETE FROM sprint_templates WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("template %q not found", id)
	}
	return nil
}

// InstantiateSprintFromTemplate creates a new Sprint plus one Ticket per
// TicketTemplate in a single transaction. The returned SprintInstantiation
// echoes the persisted records so callers don't need a second read.
func (s *Store) InstantiateSprintFromTemplate(templateID string, sprint Sprint) (SprintInstantiation, error) {
	tmpl, err := s.GetSprintTemplate(templateID)
	if err != nil {
		return SprintInstantiation{}, err
	}
	if sprint.ID == "" {
		return SprintInstantiation{}, fmt.Errorf("sprint id required")
	}
	if sprint.Name == "" {
		sprint.Name = tmpl.Name
	}
	if sprint.Theme == "" {
		sprint.Theme = tmpl.Theme
	}
	if sprint.Status == "" {
		sprint.Status = SprintPlanned
	}
	if sprint.CreatedAt.IsZero() {
		sprint.CreatedAt = time.Now()
	}

	tx, err := s.db.Begin()
	if err != nil {
		return SprintInstantiation{}, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT INTO sprints (id, name, status, owner_agent, theme, start_at, end_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sprint.ID, sprint.Name, sprint.Status, sprint.OwnerAgent, sprint.Theme,
		formatTime(sprint.StartAt), formatTime(sprint.EndAt), formatTime(sprint.CreatedAt),
	); err != nil {
		return SprintInstantiation{}, fmt.Errorf("insert sprint: %w", err)
	}

	now := time.Now()
	out := SprintInstantiation{Sprint: sprint}
	for i, tt := range tmpl.Tickets {
		ticketID := fmt.Sprintf("%s-%03d", sprint.ID, i+1)
		tk := Ticket{
			ID:                 ticketID,
			SprintID:           sprint.ID,
			Title:              tt.Title,
			Description:        tt.Description,
			Status:             StatusBacklog,
			OwnerAgent:         tt.OwnerAgent,
			Priority:           tt.Priority,
			AcceptanceCriteria: tt.AcceptanceCriteria,
			Labels:             tt.Labels,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if _, err := tx.Exec(
			`INSERT INTO tickets (id, sprint_id, title, description, status, owner_agent, priority, acceptance_criteria, handoff_doc_path, due_date, labels, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			tk.ID, tk.SprintID, tk.Title, tk.Description, tk.Status, tk.OwnerAgent,
			tk.Priority, tk.AcceptanceCriteria, tk.HandoffDocPath,
			formatTime(tk.DueDate), encodeLabels(tk.Labels),
			formatTime(tk.CreatedAt), formatTime(tk.UpdatedAt),
		); err != nil {
			return SprintInstantiation{}, fmt.Errorf("insert ticket %s: %w", ticketID, err)
		}
		out.Tickets = append(out.Tickets, tk)
	}

	if err := tx.Commit(); err != nil {
		return SprintInstantiation{}, fmt.Errorf("commit: %w", err)
	}
	return out, nil
}
