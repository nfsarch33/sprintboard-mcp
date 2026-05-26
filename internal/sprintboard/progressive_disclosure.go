package sprintboard

import (
	"database/sql"
	"fmt"
	"time"
)

// --- Story 1: Sprint Goals ---

func (s *Store) SetSprintGoal(sprintID, goal string) error {
	res, err := s.db.Exec(`UPDATE sprints SET goal = ? WHERE id = ?`, goal, sprintID)
	if err != nil {
		return fmt.Errorf("set sprint goal: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("sprint %q not found", sprintID)
	}
	return nil
}

func (s *Store) GetSprintGoal(sprintID string) (string, error) {
	var goal sql.NullString
	err := s.db.QueryRow(`SELECT goal FROM sprints WHERE id = ?`, sprintID).Scan(&goal)
	if err != nil {
		return "", fmt.Errorf("sprint %q not found: %w", sprintID, err)
	}
	return nullString(goal), nil
}

// --- Story 2: Context Summary (Progressive Disclosure) ---

type ContextSummaryDepth1 struct {
	Roadmaps     []RoadmapBrief `json:"roadmaps"`
	ActiveSprint *SprintBrief   `json:"active_sprint,omitempty"`
}

type RoadmapBrief struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type SprintBrief struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Goal string `json:"goal,omitempty"`
}

type ContextSummaryDepth2 struct {
	ContextSummaryDepth1
	ActiveEpics   []EpicBrief          `json:"active_epics"`
	TicketSummary map[TicketStatus]int `json:"ticket_summary"`
	TotalTickets  int                  `json:"total_tickets"`
}

type EpicBrief struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type ContextSummaryDepth3 struct {
	ContextSummaryDepth2
	Tickets []TicketBrief `json:"tickets"`
}

type TicketBrief struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	Status      TicketStatus `json:"status"`
	Description string       `json:"description,omitempty"`
	ClaimedBy   string       `json:"claimed_by,omitempty"`
	Priority    int          `json:"priority"`
}

func (s *Store) ContextSummary(depth int) (interface{}, error) {
	if depth < 1 {
		depth = 1
	}
	if depth > 3 {
		depth = 3
	}

	d1 := ContextSummaryDepth1{}

	roadmaps, err := s.ListRoadmaps()
	if err != nil {
		return nil, err
	}
	for _, r := range roadmaps {
		d1.Roadmaps = append(d1.Roadmaps, RoadmapBrief{ID: r.ID, Name: r.Name})
	}
	if d1.Roadmaps == nil {
		d1.Roadmaps = []RoadmapBrief{}
	}

	sprints, err := s.ListSprints()
	if err != nil {
		return nil, err
	}
	for _, sp := range sprints {
		if sp.Status == SprintActive {
			goal, _ := s.GetSprintGoal(sp.ID)
			d1.ActiveSprint = &SprintBrief{ID: sp.ID, Name: sp.Name, Goal: goal}
			break
		}
	}

	if depth == 1 {
		return d1, nil
	}

	d2 := ContextSummaryDepth2{ContextSummaryDepth1: d1}

	epics, err := s.ListEpics("")
	if err == nil {
		for _, e := range epics {
			if e.Status == EpicInProgress {
				d2.ActiveEpics = append(d2.ActiveEpics, EpicBrief{
					ID: e.ID, Name: e.Name, Status: string(e.Status),
				})
			}
		}
	}
	if d2.ActiveEpics == nil {
		d2.ActiveEpics = []EpicBrief{}
	}

	d2.TicketSummary = make(map[TicketStatus]int)
	if d1.ActiveSprint != nil {
		summary, err := s.SprintSummary(d1.ActiveSprint.ID)
		if err == nil {
			d2.TicketSummary = summary.TicketsByStatus
			d2.TotalTickets = summary.TotalTickets
		}
	}

	if depth == 2 {
		return d2, nil
	}

	d3 := ContextSummaryDepth3{ContextSummaryDepth2: d2}
	if d1.ActiveSprint != nil {
		tickets, err := s.ListTickets(d1.ActiveSprint.ID)
		if err == nil {
			for _, t := range tickets {
				d3.Tickets = append(d3.Tickets, TicketBrief{
					ID: t.ID, Title: t.Title, Status: t.Status,
					Description: t.Description, ClaimedBy: t.ClaimedBy,
					Priority: t.Priority,
				})
			}
		}
	}
	if d3.Tickets == nil {
		d3.Tickets = []TicketBrief{}
	}

	return d3, nil
}

// --- Story 3: Context Detail Drill-Down ---

type ContextDetail struct {
	EntityType string      `json:"entity_type"`
	Entity     interface{} `json:"entity"`
	Children   interface{} `json:"children,omitempty"`
}

func (s *Store) ContextDetail(entityID string) (ContextDetail, error) {
	if r, err := s.GetRoadmap(entityID); err == nil {
		programmes, _ := s.ListProgrammes(entityID)
		if programmes == nil {
			programmes = []Programme{}
		}
		return ContextDetail{EntityType: "roadmap", Entity: r, Children: programmes}, nil
	}

	if p, err := s.GetProgramme(entityID); err == nil {
		epics, _ := s.ListEpics(entityID)
		if epics == nil {
			epics = []Epic{}
		}
		return ContextDetail{EntityType: "programme", Entity: p, Children: epics}, nil
	}

	if e, err := s.GetEpic(entityID); err == nil {
		tickets, _ := s.listTicketsByEpic(entityID)
		if tickets == nil {
			tickets = []Ticket{}
		}
		return ContextDetail{EntityType: "epic", Entity: e, Children: tickets}, nil
	}

	if sp, err := s.GetSprint(entityID); err == nil {
		tickets, _ := s.ListTickets(entityID)
		if tickets == nil {
			tickets = []Ticket{}
		}
		return ContextDetail{EntityType: "sprint", Entity: sp, Children: tickets}, nil
	}

	if t, err := s.GetTicket(entityID); err == nil {
		comments, _ := s.ListTicketComments(entityID)
		if comments == nil {
			comments = []TicketComment{}
		}
		return ContextDetail{EntityType: "ticket", Entity: t, Children: comments}, nil
	}

	return ContextDetail{}, fmt.Errorf("entity %q not found in any table", entityID)
}

func (s *Store) listTicketsByEpic(epicID string) ([]Ticket, error) {
	rows, err := s.db.Query(
		`SELECT id, sprint_id, title, description, status, owner_agent, priority,
		        acceptance_criteria, handoff_doc_path, due_date, labels,
		        claimed_by, claimed_at, completed_at, created_at, updated_at
		 FROM tickets WHERE epic_id = ? ORDER BY priority DESC, created_at ASC`, epicID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Ticket
	for rows.Next() {
		var t Ticket
		var createdAt, updatedAt string
		var sprintID, description, ownerAgent, acceptanceCriteria, handoffDocPath sql.NullString
		var dueDate, labelsRaw, claimedBy, claimedAt, completedAt sql.NullString
		if err := rows.Scan(&t.ID, &sprintID, &t.Title, &description, &t.Status,
			&ownerAgent, &t.Priority, &acceptanceCriteria, &handoffDocPath,
			&dueDate, &labelsRaw, &claimedBy, &claimedAt, &completedAt,
			&createdAt, &updatedAt); err != nil {
			return nil, err
		}
		t.SprintID = nullString(sprintID)
		t.Description = nullString(description)
		t.OwnerAgent = nullString(ownerAgent)
		t.AcceptanceCriteria = nullString(acceptanceCriteria)
		t.HandoffDocPath = nullString(handoffDocPath)
		t.DueDate = parseTime(nullString(dueDate))
		t.Labels = decodeLabels(nullString(labelsRaw))
		t.ClaimedBy = nullString(claimedBy)
		t.ClaimedAt = parseTime(nullString(claimedAt))
		t.CompletedAt = parseTime(nullString(completedAt))
		t.CreatedAt = parseTime(createdAt)
		t.UpdatedAt = parseTime(updatedAt)
		out = append(out, t)
	}
	return out, rows.Err()
}

// --- Story 4: Session Handoff Auto-Archive ---

func (s *Store) migrateSessionHandoffArchive() error {
	stmt := `ALTER TABLE session_handoffs ADD COLUMN archived INTEGER DEFAULT 0`
	if _, err := s.db.Exec(stmt); err != nil && !isAlterColumnExists(err) {
		return fmt.Errorf("migrate session_handoff archived: %w", err)
	}
	return nil
}

func (s *Store) AutoArchiveOldHandoffs() error {
	cutoff := time.Now().Add(-7 * 24 * time.Hour).Format(time.RFC3339)
	_, err := s.db.Exec(
		`UPDATE session_handoffs SET archived = 1 WHERE archived = 0 AND created_at < ?`, cutoff,
	)
	return err
}

func (s *Store) ArchiveSessionHandoff(id string) error {
	res, err := s.db.Exec(`UPDATE session_handoffs SET archived = 1 WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("session handoff %q not found", id)
	}
	return nil
}

func (s *Store) LatestSessionHandoffsFiltered(agentID string, limit int, includeArchived bool) ([]SessionHandoff, error) {
	if limit <= 0 {
		limit = 3
	}
	if limit > 20 {
		limit = 20
	}

	archiveClause := ""
	if !includeArchived {
		archiveClause = " AND (archived IS NULL OR archived = 0)"
	}

	var query string
	var args []interface{}
	if agentID != "" {
		query = `SELECT id, session_id, agent_id, COALESCE(sprint_id,''), summary,
		         COALESCE(carry_forward,''), COALESCE(blockers,''), COALESCE(commits,''),
		         COALESCE(repos_pushed,''), created_at
		         FROM session_handoffs WHERE agent_id = ?` + archiveClause +
			` ORDER BY created_at DESC LIMIT ?`
		args = []interface{}{agentID, limit}
	} else {
		query = `SELECT id, session_id, agent_id, COALESCE(sprint_id,''), summary,
		         COALESCE(carry_forward,''), COALESCE(blockers,''), COALESCE(commits,''),
		         COALESCE(repos_pushed,''), created_at
		         FROM session_handoffs WHERE 1=1` + archiveClause +
			` ORDER BY created_at DESC LIMIT ?`
		args = []interface{}{limit}
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var handoffs []SessionHandoff
	for rows.Next() {
		var h SessionHandoff
		var createdStr string
		if err := rows.Scan(&h.ID, &h.SessionID, &h.AgentID, &h.SprintID, &h.Summary,
			&h.CarryForward, &h.Blockers, &h.Commits, &h.ReposPushed, &createdStr); err != nil {
			return nil, err
		}
		h.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		handoffs = append(handoffs, h)
	}
	return handoffs, rows.Err()
}

// --- Story 5: Startup Context ---

type StartupContext struct {
	LatestHandoffs []SessionHandoff `json:"latest_handoffs"`
	ActiveSprint   *SprintBrief     `json:"active_sprint,omitempty"`
	ContextSummary interface{}      `json:"context_summary"`
}

func (s *Store) StartupContext() (StartupContext, error) {
	sc := StartupContext{}

	handoffs, err := s.LatestSessionHandoffsFiltered("", 3, false)
	if err == nil {
		sc.LatestHandoffs = handoffs
	}
	if sc.LatestHandoffs == nil {
		sc.LatestHandoffs = []SessionHandoff{}
	}

	sprints, err := s.ListSprints()
	if err == nil {
		for _, sp := range sprints {
			if sp.Status == SprintActive {
				goal, _ := s.GetSprintGoal(sp.ID)
				sc.ActiveSprint = &SprintBrief{ID: sp.ID, Name: sp.Name, Goal: goal}
				break
			}
		}
	}

	summary, err := s.ContextSummary(1)
	if err == nil {
		sc.ContextSummary = summary
	}

	return sc, nil
}
