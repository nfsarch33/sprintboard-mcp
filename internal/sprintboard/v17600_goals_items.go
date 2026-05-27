package sprintboard

import (
	"fmt"
	"time"
)

type SprintGoal struct {
	ID        int64  `json:"id"`
	SprintID  string `json:"sprint_id"`
	GoalText  string `json:"goal_text"`
	Status    string `json:"status"`
	Priority  int    `json:"priority"`
	CreatedAt string `json:"created_at"`
}

type RoadmapItem struct {
	ID          int64  `json:"id"`
	RoadmapID   string `json:"roadmap_id"`
	EpicID      string `json:"epic_id,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
	Priority    int    `json:"priority"`
	CreatedAt   string `json:"created_at"`
}

func (s *Store) migrateV17600GoalsItems() error {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS sprint_goals (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			sprint_id TEXT NOT NULL REFERENCES sprints(id),
			goal_text TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			priority INTEGER DEFAULT 0,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS roadmap_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			roadmap_id TEXT NOT NULL REFERENCES roadmaps(id),
			epic_id TEXT REFERENCES epics(id),
			title TEXT NOT NULL,
			description TEXT,
			status TEXT NOT NULL DEFAULT 'planned',
			priority INTEGER DEFAULT 0,
			created_at TEXT NOT NULL
		)`,
	}
	for _, stmt := range tables {
		if _, err := s.db.ExecDDL(stmt); err != nil {
			return fmt.Errorf("migrate v17600 tables: %w", err)
		}
	}

	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_sprint_goals_sprint ON sprint_goals(sprint_id)`,
		`CREATE INDEX IF NOT EXISTS idx_roadmap_items_roadmap ON roadmap_items(roadmap_id)`,
		`CREATE INDEX IF NOT EXISTS idx_roadmap_items_epic ON roadmap_items(epic_id)`,
	}
	for _, stmt := range indexes {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate v17600 indexes: %w", err)
		}
	}
	return nil
}

// --- Sprint Goal CRUD ---

func (s *Store) CreateSprintGoal(g SprintGoal) (int64, error) {
	if g.SprintID == "" {
		return 0, fmt.Errorf("sprint_id required")
	}
	if g.GoalText == "" {
		return 0, fmt.Errorf("goal_text required")
	}
	if g.Status == "" {
		g.Status = "pending"
	}
	now := time.Now().Format(time.RFC3339)

	res, err := s.db.Exec(
		`INSERT INTO sprint_goals (sprint_id, goal_text, status, priority, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		g.SprintID, g.GoalText, g.Status, g.Priority, now,
	)
	if err != nil {
		return 0, fmt.Errorf("create sprint goal: %w", err)
	}
	return res.LastInsertId()
}

func (s *Store) ListSprintGoals(sprintID string) ([]SprintGoal, error) {
	if sprintID == "" {
		return nil, fmt.Errorf("sprint_id required")
	}
	rows, err := s.db.Query(
		`SELECT id, sprint_id, goal_text, status, priority, created_at
		 FROM sprint_goals WHERE sprint_id = ? ORDER BY priority DESC, id ASC`,
		sprintID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SprintGoal
	for rows.Next() {
		var g SprintGoal
		if err := rows.Scan(&g.ID, &g.SprintID, &g.GoalText, &g.Status, &g.Priority, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) UpdateSprintGoalStatus(goalID int64, status string) error {
	res, err := s.db.Exec(
		`UPDATE sprint_goals SET status = ? WHERE id = ?`, status, goalID,
	)
	if err != nil {
		return fmt.Errorf("update sprint goal: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("sprint goal %d not found", goalID)
	}
	return nil
}

// --- Roadmap Item CRUD ---

func (s *Store) CreateRoadmapItem(item RoadmapItem) (int64, error) {
	if item.RoadmapID == "" {
		return 0, fmt.Errorf("roadmap_id required")
	}
	if item.Title == "" {
		return 0, fmt.Errorf("title required")
	}
	if item.Status == "" {
		item.Status = "planned"
	}
	now := time.Now().Format(time.RFC3339)

	res, err := s.db.Exec(
		`INSERT INTO roadmap_items (roadmap_id, epic_id, title, description, status, priority, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		item.RoadmapID, nilIfEmpty(item.EpicID), item.Title, item.Description, item.Status, item.Priority, now,
	)
	if err != nil {
		return 0, fmt.Errorf("create roadmap item: %w", err)
	}
	return res.LastInsertId()
}

func (s *Store) ListRoadmapItems(roadmapID string) ([]RoadmapItem, error) {
	if roadmapID == "" {
		return nil, fmt.Errorf("roadmap_id required")
	}
	rows, err := s.db.Query(
		`SELECT id, roadmap_id, COALESCE(epic_id, ''), title, COALESCE(description, ''), status, priority, created_at
		 FROM roadmap_items WHERE roadmap_id = ? ORDER BY priority DESC, id ASC`,
		roadmapID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RoadmapItem
	for rows.Next() {
		var item RoadmapItem
		if err := rows.Scan(&item.ID, &item.RoadmapID, &item.EpicID, &item.Title,
			&item.Description, &item.Status, &item.Priority, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// --- Epic-based Ticket Tree ---

type EpicTicketTree struct {
	Epic    *Epic            `json:"epic"`
	Tickets []TicketTreeNode `json:"tickets"`
}

func (s *Store) TicketTreeByEpic(epicID string) (EpicTicketTree, error) {
	e, err := s.GetEpic(epicID)
	if err != nil {
		return EpicTicketTree{}, err
	}
	tree := EpicTicketTree{Epic: &e}

	rows, err := s.db.Query(
		`SELECT id, title, status, COALESCE(story_type, 'task'), COALESCE(estimate_minutes, 0),
		        COALESCE(actual_minutes, 0), COALESCE(parent_ticket_id, '')
		 FROM tickets WHERE epic_id = ? ORDER BY priority DESC, created_at ASC`, epicID,
	)
	if err != nil {
		return EpicTicketTree{}, err
	}
	defer rows.Close()

	type flatTicket struct {
		node     TicketTreeNode
		parentID string
	}
	var flat []flatTicket
	for rows.Next() {
		var ft flatTicket
		if err := rows.Scan(&ft.node.ID, &ft.node.Title, &ft.node.Status,
			&ft.node.StoryType, &ft.node.EstimateMinutes, &ft.node.ActualMinutes,
			&ft.parentID); err != nil {
			return EpicTicketTree{}, err
		}
		flat = append(flat, ft)
	}
	if err := rows.Err(); err != nil {
		return EpicTicketTree{}, err
	}

	nodeMap := make(map[string]*TicketTreeNode)
	for i := range flat {
		nodeMap[flat[i].node.ID] = &flat[i].node
	}
	childSet := make(map[string]bool)
	for i := range flat {
		if flat[i].parentID != "" {
			if parent, ok := nodeMap[flat[i].parentID]; ok {
				parent.Children = append(parent.Children, *nodeMap[flat[i].node.ID])
				childSet[flat[i].node.ID] = true
			}
		}
	}
	for i := range flat {
		if !childSet[flat[i].node.ID] {
			tree.Tickets = append(tree.Tickets, *nodeMap[flat[i].node.ID])
		}
	}
	return tree, nil
}

// --- Full-Text Search for Session Handoffs (PostgreSQL tsvector, SQLite LIKE fallback) ---

func (s *Store) SearchSessionHandoffsFTS(query string, limit int) ([]SessionHandoff, error) {
	if query == "" {
		return nil, fmt.Errorf("query required")
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	if s.db.dialect == DialectPostgres {
		return s.searchHandoffsPgFTS(query, limit)
	}
	return s.searchHandoffsLIKE(query, limit)
}

func (s *Store) searchHandoffsPgFTS(query string, limit int) ([]SessionHandoff, error) {
	rows, err := s.db.Query(
		`SELECT id, session_id, agent_id, COALESCE(sprint_id,''), summary,
		        COALESCE(carry_forward,''), COALESCE(blockers,''), COALESCE(commits,''),
		        COALESCE(repos_pushed,''), created_at,
		        ts_rank(to_tsvector('english', COALESCE(summary,'') || ' ' || COALESCE(carry_forward,'') || ' ' || COALESCE(blockers,'')),
		                plainto_tsquery('english', ?)) AS rank
		 FROM session_handoffs
		 WHERE to_tsvector('english', COALESCE(summary,'') || ' ' || COALESCE(carry_forward,'') || ' ' || COALESCE(blockers,''))
		       @@ plainto_tsquery('english', ?)
		   AND (archived IS NULL OR archived = 0)
		 ORDER BY rank DESC, created_at DESC
		 LIMIT ?`,
		query, query, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var handoffs []SessionHandoff
	for rows.Next() {
		var h SessionHandoff
		var createdStr string
		var rank float64
		if err := rows.Scan(&h.ID, &h.SessionID, &h.AgentID, &h.SprintID, &h.Summary,
			&h.CarryForward, &h.Blockers, &h.Commits, &h.ReposPushed, &createdStr, &rank); err != nil {
			return nil, err
		}
		h.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		handoffs = append(handoffs, h)
	}
	return handoffs, rows.Err()
}

func (s *Store) searchHandoffsLIKE(query string, limit int) ([]SessionHandoff, error) {
	q := "%" + query + "%"
	rows, err := s.db.Query(
		`SELECT id, session_id, agent_id, COALESCE(sprint_id,''), summary,
		        COALESCE(carry_forward,''), COALESCE(blockers,''), COALESCE(commits,''),
		        COALESCE(repos_pushed,''), created_at
		 FROM session_handoffs
		 WHERE (summary LIKE ? OR carry_forward LIKE ? OR blockers LIKE ?)
		   AND (archived IS NULL OR archived = 0)
		 ORDER BY created_at DESC
		 LIMIT ?`,
		q, q, q, limit,
	)
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
