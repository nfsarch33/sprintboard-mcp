package sprintboard

import (
	"database/sql"
	"fmt"
	"time"
)

type RoadmapStatus string

const (
	RoadmapActive   RoadmapStatus = "active"
	RoadmapArchived RoadmapStatus = "archived"
)

type ProgrammeStatus string

const (
	ProgrammePlanning  ProgrammeStatus = "planning"
	ProgrammeActive    ProgrammeStatus = "active"
	ProgrammeCompleted ProgrammeStatus = "completed"
)

type EpicStatus string

const (
	EpicBacklog    EpicStatus = "backlog"
	EpicInProgress EpicStatus = "in_progress"
	EpicDone       EpicStatus = "done"
)

type Roadmap struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	Status      RoadmapStatus `json:"status"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

type Programme struct {
	ID          string          `json:"id"`
	RoadmapID   string          `json:"roadmap_id,omitempty"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Status      ProgrammeStatus `json:"status"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type Epic struct {
	ID          string     `json:"id"`
	ProgrammeID string     `json:"programme_id,omitempty"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Status      EpicStatus `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// migrateV2Hierarchy adds roadmap->programme->epic hierarchy tables and
// extends sprints/tickets with foreign-key columns. All statements are
// idempotent (CREATE IF NOT EXISTS / ALTER ADD COLUMN with duplicate check).
func (s *Store) migrateV2Hierarchy() error {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS roadmaps (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			status TEXT DEFAULT 'active',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS programmes (
			id TEXT PRIMARY KEY,
			roadmap_id TEXT REFERENCES roadmaps(id),
			name TEXT NOT NULL,
			description TEXT,
			status TEXT DEFAULT 'planning',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS epics (
			id TEXT PRIMARY KEY,
			programme_id TEXT REFERENCES programmes(id),
			name TEXT NOT NULL,
			description TEXT,
			status TEXT DEFAULT 'backlog',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
	}
	for _, stmt := range tables {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate v2 hierarchy tables: %w", err)
		}
	}

	alters := []string{
		`ALTER TABLE sprints ADD COLUMN programme_id TEXT REFERENCES programmes(id)`,
		`ALTER TABLE sprints ADD COLUMN goal TEXT`,
		`ALTER TABLE tickets ADD COLUMN epic_id TEXT REFERENCES epics(id)`,
		`ALTER TABLE tickets ADD COLUMN story_type TEXT DEFAULT 'task'`,
		`ALTER TABLE tickets ADD COLUMN estimate_minutes INTEGER`,
		`ALTER TABLE tickets ADD COLUMN actual_minutes INTEGER`,
		`ALTER TABLE tickets ADD COLUMN parent_ticket_id TEXT REFERENCES tickets(id)`,
	}
	for _, stmt := range alters {
		if _, err := s.db.Exec(stmt); err != nil && !isAlterColumnExists(err) {
			return fmt.Errorf("migrate v2 hierarchy alters: %w", err)
		}
	}

	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_programmes_roadmap ON programmes(roadmap_id)`,
		`CREATE INDEX IF NOT EXISTS idx_epics_programme ON epics(programme_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tickets_epic ON tickets(epic_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tickets_parent ON tickets(parent_ticket_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sprints_programme ON sprints(programme_id)`,
	}
	for _, stmt := range indexes {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate v2 hierarchy indexes: %w", err)
		}
	}
	return nil
}

// --- Roadmap CRUD ---

func (s *Store) CreateRoadmap(r Roadmap) error {
	if r.ID == "" {
		return fmt.Errorf("roadmap id required")
	}
	if r.Name == "" {
		return fmt.Errorf("roadmap name required")
	}
	now := time.Now()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt = now
	}
	if r.Status == "" {
		r.Status = RoadmapActive
	}
	_, err := s.db.Exec(
		`INSERT INTO roadmaps (id, name, description, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		r.ID, r.Name, r.Description, r.Status,
		formatTime(r.CreatedAt), formatTime(r.UpdatedAt),
	)
	return err
}

func (s *Store) GetRoadmap(id string) (Roadmap, error) {
	var r Roadmap
	var desc, createdAt, updatedAt sql.NullString
	err := s.db.QueryRow(
		`SELECT id, name, description, status, created_at, updated_at FROM roadmaps WHERE id = ?`, id,
	).Scan(&r.ID, &r.Name, &desc, &r.Status, &createdAt, &updatedAt)
	if err != nil {
		return Roadmap{}, fmt.Errorf("roadmap %q not found: %w", id, err)
	}
	r.Description = nullString(desc)
	r.CreatedAt = parseTime(nullString(createdAt))
	r.UpdatedAt = parseTime(nullString(updatedAt))
	return r, nil
}

func (s *Store) ListRoadmaps() ([]Roadmap, error) {
	rows, err := s.db.Query(
		`SELECT id, name, description, status, created_at, updated_at FROM roadmaps ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Roadmap
	for rows.Next() {
		var r Roadmap
		var desc, createdAt, updatedAt sql.NullString
		if err := rows.Scan(&r.ID, &r.Name, &desc, &r.Status, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		r.Description = nullString(desc)
		r.CreatedAt = parseTime(nullString(createdAt))
		r.UpdatedAt = parseTime(nullString(updatedAt))
		out = append(out, r)
	}
	return out, rows.Err()
}

// --- Programme CRUD ---

func (s *Store) CreateProgramme(p Programme) error {
	if p.ID == "" {
		return fmt.Errorf("programme id required")
	}
	if p.Name == "" {
		return fmt.Errorf("programme name required")
	}
	now := time.Now()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = now
	}
	if p.Status == "" {
		p.Status = ProgrammePlanning
	}
	_, err := s.db.Exec(
		`INSERT INTO programmes (id, roadmap_id, name, description, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.ID, nilIfEmpty(p.RoadmapID), p.Name, p.Description, p.Status,
		formatTime(p.CreatedAt), formatTime(p.UpdatedAt),
	)
	return err
}

func (s *Store) GetProgramme(id string) (Programme, error) {
	var p Programme
	var roadmapID, desc, createdAt, updatedAt sql.NullString
	err := s.db.QueryRow(
		`SELECT id, roadmap_id, name, description, status, created_at, updated_at FROM programmes WHERE id = ?`, id,
	).Scan(&p.ID, &roadmapID, &p.Name, &desc, &p.Status, &createdAt, &updatedAt)
	if err != nil {
		return Programme{}, fmt.Errorf("programme %q not found: %w", id, err)
	}
	p.RoadmapID = nullString(roadmapID)
	p.Description = nullString(desc)
	p.CreatedAt = parseTime(nullString(createdAt))
	p.UpdatedAt = parseTime(nullString(updatedAt))
	return p, nil
}

func (s *Store) ListProgrammes(roadmapID string) ([]Programme, error) {
	query := `SELECT id, roadmap_id, name, description, status, created_at, updated_at FROM programmes`
	var args []interface{}
	if roadmapID != "" {
		query += ` WHERE roadmap_id = ?`
		args = append(args, roadmapID)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Programme
	for rows.Next() {
		var p Programme
		var rmID, desc, createdAt, updatedAt sql.NullString
		if err := rows.Scan(&p.ID, &rmID, &p.Name, &desc, &p.Status, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		p.RoadmapID = nullString(rmID)
		p.Description = nullString(desc)
		p.CreatedAt = parseTime(nullString(createdAt))
		p.UpdatedAt = parseTime(nullString(updatedAt))
		out = append(out, p)
	}
	return out, rows.Err()
}

// --- Epic CRUD ---

func (s *Store) CreateEpic(e Epic) error {
	if e.ID == "" {
		return fmt.Errorf("epic id required")
	}
	if e.Name == "" {
		return fmt.Errorf("epic name required")
	}
	now := time.Now()
	if e.CreatedAt.IsZero() {
		e.CreatedAt = now
	}
	if e.UpdatedAt.IsZero() {
		e.UpdatedAt = now
	}
	if e.Status == "" {
		e.Status = EpicBacklog
	}
	_, err := s.db.Exec(
		`INSERT INTO epics (id, programme_id, name, description, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.ID, nilIfEmpty(e.ProgrammeID), e.Name, e.Description, e.Status,
		formatTime(e.CreatedAt), formatTime(e.UpdatedAt),
	)
	return err
}

func (s *Store) GetEpic(id string) (Epic, error) {
	var e Epic
	var progID, desc, createdAt, updatedAt sql.NullString
	err := s.db.QueryRow(
		`SELECT id, programme_id, name, description, status, created_at, updated_at FROM epics WHERE id = ?`, id,
	).Scan(&e.ID, &progID, &e.Name, &desc, &e.Status, &createdAt, &updatedAt)
	if err != nil {
		return Epic{}, fmt.Errorf("epic %q not found: %w", id, err)
	}
	e.ProgrammeID = nullString(progID)
	e.Description = nullString(desc)
	e.CreatedAt = parseTime(nullString(createdAt))
	e.UpdatedAt = parseTime(nullString(updatedAt))
	return e, nil
}

func (s *Store) ListEpics(programmeID string) ([]Epic, error) {
	query := `SELECT id, programme_id, name, description, status, created_at, updated_at FROM epics`
	var args []interface{}
	if programmeID != "" {
		query += ` WHERE programme_id = ?`
		args = append(args, programmeID)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Epic
	for rows.Next() {
		var e Epic
		var progID, desc, createdAt, updatedAt sql.NullString
		if err := rows.Scan(&e.ID, &progID, &e.Name, &desc, &e.Status, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		e.ProgrammeID = nullString(progID)
		e.Description = nullString(desc)
		e.CreatedAt = parseTime(nullString(createdAt))
		e.UpdatedAt = parseTime(nullString(updatedAt))
		out = append(out, e)
	}
	return out, rows.Err()
}

// --- Epic Burndown ---

type EpicBurndown struct {
	EpicID       string         `json:"epic_id"`
	EpicName     string         `json:"epic_name"`
	TotalTickets int            `json:"total_tickets"`
	ByStatus     map[string]int `json:"by_status"`
}

func (s *Store) EpicBurndown(epicID string) (EpicBurndown, error) {
	e, err := s.GetEpic(epicID)
	if err != nil {
		return EpicBurndown{}, err
	}

	rows, err := s.db.Query(
		`SELECT status, COUNT(*) FROM tickets WHERE epic_id = ? GROUP BY status`, epicID,
	)
	if err != nil {
		return EpicBurndown{}, err
	}
	defer rows.Close()

	bd := EpicBurndown{
		EpicID:   epicID,
		EpicName: e.Name,
		ByStatus: make(map[string]int),
	}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return EpicBurndown{}, err
		}
		bd.ByStatus[status] = count
		bd.TotalTickets += count
	}
	return bd, rows.Err()
}

// --- Time Tracking ---

func (s *Store) LogTicketTime(ticketID string, minutes int) error {
	res, err := s.db.Exec(
		`UPDATE tickets SET actual_minutes = COALESCE(actual_minutes, 0) + ?, updated_at = ? WHERE id = ?`,
		minutes, formatTime(time.Now()), ticketID,
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

func (s *Store) SetTicketEstimateMinutes(ticketID string, minutes int) error {
	res, err := s.db.Exec(
		`UPDATE tickets SET estimate_minutes = ?, updated_at = ? WHERE id = ?`,
		minutes, formatTime(time.Now()), ticketID,
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

type SprintTimeReport struct {
	SprintID        string       `json:"sprint_id"`
	TotalEstimate   int          `json:"total_estimate_minutes"`
	TotalActual     int          `json:"total_actual_minutes"`
	AccuracyRatio   float64      `json:"accuracy_ratio"`
	TicketBreakdown []TicketTime `json:"ticket_breakdown"`
}

type TicketTime struct {
	TicketID        string  `json:"ticket_id"`
	Title           string  `json:"title"`
	EstimateMinutes int     `json:"estimate_minutes"`
	ActualMinutes   int     `json:"actual_minutes"`
	Ratio           float64 `json:"ratio"`
}

func (s *Store) SprintTimeReport(sprintID string) (SprintTimeReport, error) {
	rows, err := s.db.Query(
		`SELECT id, title, COALESCE(estimate_minutes, 0), COALESCE(actual_minutes, 0)
		 FROM tickets WHERE sprint_id = ? ORDER BY created_at ASC`, sprintID,
	)
	if err != nil {
		return SprintTimeReport{}, err
	}
	defer rows.Close()

	rpt := SprintTimeReport{SprintID: sprintID}
	for rows.Next() {
		var tt TicketTime
		if err := rows.Scan(&tt.TicketID, &tt.Title, &tt.EstimateMinutes, &tt.ActualMinutes); err != nil {
			return SprintTimeReport{}, err
		}
		if tt.EstimateMinutes > 0 && tt.ActualMinutes > 0 {
			tt.Ratio = float64(tt.ActualMinutes) / float64(tt.EstimateMinutes)
		}
		rpt.TotalEstimate += tt.EstimateMinutes
		rpt.TotalActual += tt.ActualMinutes
		rpt.TicketBreakdown = append(rpt.TicketBreakdown, tt)
	}
	if rpt.TotalEstimate > 0 && rpt.TotalActual > 0 {
		rpt.AccuracyRatio = float64(rpt.TotalActual) / float64(rpt.TotalEstimate)
	}
	return rpt, rows.Err()
}

// --- Ticket Tree (full hierarchy) ---

type TicketTreeNode struct {
	ID              string           `json:"id"`
	Title           string           `json:"title"`
	Status          TicketStatus     `json:"status"`
	StoryType       string           `json:"story_type,omitempty"`
	EstimateMinutes int              `json:"estimate_minutes,omitempty"`
	ActualMinutes   int              `json:"actual_minutes,omitempty"`
	Children        []TicketTreeNode `json:"children,omitempty"`
}

type HierarchyTree struct {
	Roadmap   *Roadmap         `json:"roadmap,omitempty"`
	Programme *Programme       `json:"programme,omitempty"`
	Epic      *Epic            `json:"epic,omitempty"`
	Sprint    *Sprint          `json:"sprint,omitempty"`
	Tickets   []TicketTreeNode `json:"tickets,omitempty"`
}

func (s *Store) TicketTree(sprintID string) (HierarchyTree, error) {
	sp, err := s.GetSprint(sprintID)
	if err != nil {
		return HierarchyTree{}, err
	}
	tree := HierarchyTree{Sprint: &sp}

	rows, err := s.db.Query(
		`SELECT id, title, status, COALESCE(story_type, 'task'), COALESCE(estimate_minutes, 0),
		        COALESCE(actual_minutes, 0), COALESCE(parent_ticket_id, '')
		 FROM tickets WHERE sprint_id = ? ORDER BY priority DESC, created_at ASC`, sprintID,
	)
	if err != nil {
		return HierarchyTree{}, err
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
			return HierarchyTree{}, err
		}
		flat = append(flat, ft)
	}
	if err := rows.Err(); err != nil {
		return HierarchyTree{}, err
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

// --- Session Summary ---

type SessionSummary struct {
	ActiveSprint   *SprintSummary `json:"active_sprint,omitempty"`
	RecentHandoffs []Handoff      `json:"recent_handoffs"`
	ActiveAgents   []Agent        `json:"active_agents"`
	BlockedTickets []Ticket       `json:"blocked_tickets"`
}

func (s *Store) SessionSummary() (SessionSummary, error) {
	var ss SessionSummary

	sprints, err := s.ListSprints()
	if err != nil {
		return ss, err
	}
	for _, sp := range sprints {
		if sp.Status == SprintActive {
			summary, err := s.SprintSummary(sp.ID)
			if err == nil {
				ss.ActiveSprint = &summary
			}
			break
		}
	}

	agents, err := s.ListActiveAgents()
	if err == nil {
		ss.ActiveAgents = agents
	}
	if ss.ActiveAgents == nil {
		ss.ActiveAgents = []Agent{}
	}

	rows, err := s.db.Query(
		`SELECT id, ticket_id, from_agent, to_agent, context_path, created_at
		 FROM handoffs ORDER BY created_at DESC LIMIT 3`,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var h Handoff
			var createdAt string
			if err := rows.Scan(&h.ID, &h.TicketID, &h.FromAgent, &h.ToAgent, &h.ContextPath, &createdAt); err != nil {
				continue
			}
			h.CreatedAt = parseTime(createdAt)
			ss.RecentHandoffs = append(ss.RecentHandoffs, h)
		}
	}
	if ss.RecentHandoffs == nil {
		ss.RecentHandoffs = []Handoff{}
	}

	blocked, err := s.listTicketsByStatus(StatusBlocked)
	if err == nil {
		ss.BlockedTickets = blocked
	}
	if ss.BlockedTickets == nil {
		ss.BlockedTickets = []Ticket{}
	}
	return ss, nil
}

func (s *Store) listTicketsByStatus(status TicketStatus) ([]Ticket, error) {
	rows, err := s.db.Query(
		`SELECT id, sprint_id, title, description, status, owner_agent, priority,
		        acceptance_criteria, handoff_doc_path, due_date, labels,
		        claimed_by, claimed_at, completed_at, created_at, updated_at
		 FROM tickets WHERE status = ? ORDER BY priority DESC`, status,
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

func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
