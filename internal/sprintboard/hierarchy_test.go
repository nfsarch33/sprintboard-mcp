package sprintboard

import (
	"fmt"
	"testing"
)

func TestMigrateV2Hierarchy_CreatesNewTables(t *testing.T) {
	s := testStore(t)

	tables := []string{"roadmaps", "programmes", "epics"}
	for _, tbl := range tables {
		var name string
		err := s.db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found after migration: %v", tbl, err)
		}
	}
}

func TestMigrateV2Hierarchy_AltersSprintsColumns(t *testing.T) {
	s := testStore(t)

	_, err := s.db.Exec(`SELECT programme_id, goal FROM sprints LIMIT 0`)
	if err != nil {
		t.Fatalf("sprints missing v2 columns: %v", err)
	}
}

func TestMigrateV2Hierarchy_AltersTicketsColumns(t *testing.T) {
	s := testStore(t)

	_, err := s.db.Exec(
		`SELECT epic_id, story_type, estimate_minutes, actual_minutes, parent_ticket_id
		 FROM tickets LIMIT 0`,
	)
	if err != nil {
		t.Fatalf("tickets missing v2 columns: %v", err)
	}
}

func TestMigrateV2Hierarchy_Idempotent(t *testing.T) {
	s := testStore(t)
	if err := s.migrateV2Hierarchy(); err != nil {
		t.Fatalf("second migration call should be idempotent: %v", err)
	}
}

func TestMigrateV2Hierarchy_BackwardCompat(t *testing.T) {
	s := testStore(t)

	err := s.CreateSprint(Sprint{ID: "v100", Name: "Legacy Sprint"})
	if err != nil {
		t.Fatalf("CreateSprint on migrated schema: %v", err)
	}
	err = s.CreateTicket(Ticket{ID: "v100-001", SprintID: "v100", Title: "Legacy Ticket"})
	if err != nil {
		t.Fatalf("CreateTicket on migrated schema: %v", err)
	}

	sp, err := s.GetSprint("v100")
	if err != nil {
		t.Fatalf("GetSprint: %v", err)
	}
	if sp.Name != "Legacy Sprint" {
		t.Errorf("expected name %q, got %q", "Legacy Sprint", sp.Name)
	}
}

// --- Roadmap CRUD ---

func TestRoadmapCRUD(t *testing.T) {
	s := testStore(t)

	err := s.CreateRoadmap(Roadmap{ID: "rm-1", Name: "2026 Product"})
	if err != nil {
		t.Fatalf("CreateRoadmap: %v", err)
	}

	r, err := s.GetRoadmap("rm-1")
	if err != nil {
		t.Fatalf("GetRoadmap: %v", err)
	}
	if r.Name != "2026 Product" {
		t.Errorf("expected name %q, got %q", "2026 Product", r.Name)
	}
	if r.Status != RoadmapActive {
		t.Errorf("expected status %q, got %q", RoadmapActive, r.Status)
	}

	list, err := s.ListRoadmaps()
	if err != nil {
		t.Fatalf("ListRoadmaps: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 roadmap, got %d", len(list))
	}
}

func TestRoadmapRequiresID(t *testing.T) {
	s := testStore(t)
	err := s.CreateRoadmap(Roadmap{Name: "No ID"})
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}

func TestRoadmapRequiresName(t *testing.T) {
	s := testStore(t)
	err := s.CreateRoadmap(Roadmap{ID: "rm-noname"})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

// --- Programme CRUD ---

func TestProgrammeCRUD(t *testing.T) {
	s := testStore(t)

	s.CreateRoadmap(Roadmap{ID: "rm-1", Name: "2026 Product"})

	err := s.CreateProgramme(Programme{
		ID: "pg-1", RoadmapID: "rm-1", Name: "Fleet Monitoring",
	})
	if err != nil {
		t.Fatalf("CreateProgramme: %v", err)
	}

	p, err := s.GetProgramme("pg-1")
	if err != nil {
		t.Fatalf("GetProgramme: %v", err)
	}
	if p.Name != "Fleet Monitoring" {
		t.Errorf("expected name %q, got %q", "Fleet Monitoring", p.Name)
	}
	if p.RoadmapID != "rm-1" {
		t.Errorf("expected roadmap_id %q, got %q", "rm-1", p.RoadmapID)
	}
	if p.Status != ProgrammePlanning {
		t.Errorf("expected status %q, got %q", ProgrammePlanning, p.Status)
	}

	list, err := s.ListProgrammes("rm-1")
	if err != nil {
		t.Fatalf("ListProgrammes: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 programme, got %d", len(list))
	}

	allList, err := s.ListProgrammes("")
	if err != nil {
		t.Fatalf("ListProgrammes all: %v", err)
	}
	if len(allList) != 1 {
		t.Errorf("expected 1 programme, got %d", len(allList))
	}
}

func TestProgrammeWithoutRoadmap(t *testing.T) {
	s := testStore(t)
	err := s.CreateProgramme(Programme{ID: "pg-orphan", Name: "Standalone"})
	if err != nil {
		t.Fatalf("CreateProgramme without roadmap should succeed: %v", err)
	}
	p, err := s.GetProgramme("pg-orphan")
	if err != nil {
		t.Fatalf("GetProgramme: %v", err)
	}
	if p.RoadmapID != "" {
		t.Errorf("expected empty roadmap_id, got %q", p.RoadmapID)
	}
}

// --- Epic CRUD ---

func TestEpicCRUD(t *testing.T) {
	s := testStore(t)

	s.CreateRoadmap(Roadmap{ID: "rm-1", Name: "2026 Product"})
	s.CreateProgramme(Programme{ID: "pg-1", RoadmapID: "rm-1", Name: "Fleet Monitoring"})

	err := s.CreateEpic(Epic{
		ID: "ep-1", ProgrammeID: "pg-1", Name: "K8s Probes",
	})
	if err != nil {
		t.Fatalf("CreateEpic: %v", err)
	}

	e, err := s.GetEpic("ep-1")
	if err != nil {
		t.Fatalf("GetEpic: %v", err)
	}
	if e.Name != "K8s Probes" {
		t.Errorf("expected name %q, got %q", "K8s Probes", e.Name)
	}
	if e.Status != EpicBacklog {
		t.Errorf("expected status %q, got %q", EpicBacklog, e.Status)
	}

	list, err := s.ListEpics("pg-1")
	if err != nil {
		t.Fatalf("ListEpics: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 epic, got %d", len(list))
	}
}

// --- Epic Burndown ---

func TestEpicBurndown(t *testing.T) {
	s := testStore(t)

	s.CreateEpic(Epic{ID: "ep-burn", Name: "Burndown Epic"})
	s.CreateSprint(Sprint{ID: "sp-1", Name: "Sprint 1"})

	for i, status := range []TicketStatus{StatusBacklog, StatusInProgress, StatusDone, StatusDone} {
		id := fmt.Sprintf("t-burn-%d", i)
		s.CreateTicket(Ticket{ID: id, SprintID: "sp-1", Title: id})
		s.db.Exec(`UPDATE tickets SET epic_id = ?, status = ? WHERE id = ?`, "ep-burn", status, id)
	}

	bd, err := s.EpicBurndown("ep-burn")
	if err != nil {
		t.Fatalf("EpicBurndown: %v", err)
	}
	if bd.TotalTickets != 4 {
		t.Errorf("expected 4 total, got %d", bd.TotalTickets)
	}
	if bd.ByStatus["done"] != 2 {
		t.Errorf("expected 2 done, got %d", bd.ByStatus["done"])
	}
	if bd.ByStatus["in_progress"] != 1 {
		t.Errorf("expected 1 in_progress, got %d", bd.ByStatus["in_progress"])
	}
}

// --- Time Tracking ---

func TestLogTicketTime(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "sp-1", Name: "Sprint 1"})
	s.CreateTicket(Ticket{ID: "t-time-1", SprintID: "sp-1", Title: "Time Task"})

	if err := s.LogTicketTime("t-time-1", 30); err != nil {
		t.Fatalf("LogTicketTime: %v", err)
	}
	if err := s.LogTicketTime("t-time-1", 15); err != nil {
		t.Fatalf("LogTicketTime second call: %v", err)
	}

	var actual int
	s.db.QueryRow(`SELECT actual_minutes FROM tickets WHERE id = ?`, "t-time-1").Scan(&actual)
	if actual != 45 {
		t.Errorf("expected 45 actual_minutes, got %d", actual)
	}
}

func TestLogTicketTime_NotFound(t *testing.T) {
	s := testStore(t)
	err := s.LogTicketTime("nonexistent", 10)
	if err == nil {
		t.Fatal("expected error for nonexistent ticket")
	}
}

func TestSetTicketEstimateMinutes(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "sp-1", Name: "Sprint 1"})
	s.CreateTicket(Ticket{ID: "t-est-1", SprintID: "sp-1", Title: "Est Task"})

	if err := s.SetTicketEstimateMinutes("t-est-1", 120); err != nil {
		t.Fatalf("SetTicketEstimateMinutes: %v", err)
	}

	var est int
	s.db.QueryRow(`SELECT estimate_minutes FROM tickets WHERE id = ?`, "t-est-1").Scan(&est)
	if est != 120 {
		t.Errorf("expected 120 estimate_minutes, got %d", est)
	}
}

func TestSprintTimeReport(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "sp-rpt", Name: "Report Sprint"})

	s.CreateTicket(Ticket{ID: "t-rpt-1", SprintID: "sp-rpt", Title: "Task 1"})
	s.CreateTicket(Ticket{ID: "t-rpt-2", SprintID: "sp-rpt", Title: "Task 2"})
	s.SetTicketEstimateMinutes("t-rpt-1", 60)
	s.SetTicketEstimateMinutes("t-rpt-2", 30)
	s.LogTicketTime("t-rpt-1", 45)
	s.LogTicketTime("t-rpt-2", 40)

	rpt, err := s.SprintTimeReport("sp-rpt")
	if err != nil {
		t.Fatalf("SprintTimeReport: %v", err)
	}
	if rpt.TotalEstimate != 90 {
		t.Errorf("expected 90 total estimate, got %d", rpt.TotalEstimate)
	}
	if rpt.TotalActual != 85 {
		t.Errorf("expected 85 total actual, got %d", rpt.TotalActual)
	}
	if len(rpt.TicketBreakdown) != 2 {
		t.Errorf("expected 2 ticket breakdowns, got %d", len(rpt.TicketBreakdown))
	}
	if rpt.AccuracyRatio == 0 {
		t.Error("expected non-zero accuracy ratio")
	}
}

// --- Ticket Tree ---

func TestTicketTree(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "sp-tree", Name: "Tree Sprint"})

	s.CreateTicket(Ticket{ID: "parent-1", SprintID: "sp-tree", Title: "Parent"})
	s.CreateTicket(Ticket{ID: "child-1", SprintID: "sp-tree", Title: "Child"})
	s.db.Exec(`UPDATE tickets SET parent_ticket_id = ? WHERE id = ?`, "parent-1", "child-1")

	tree, err := s.TicketTree("sp-tree")
	if err != nil {
		t.Fatalf("TicketTree: %v", err)
	}
	if tree.Sprint == nil {
		t.Fatal("expected sprint in tree")
	}
	if len(tree.Tickets) != 1 {
		t.Fatalf("expected 1 root ticket, got %d", len(tree.Tickets))
	}
	if tree.Tickets[0].ID != "parent-1" {
		t.Errorf("expected root ticket parent-1, got %s", tree.Tickets[0].ID)
	}
	if len(tree.Tickets[0].Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(tree.Tickets[0].Children))
	}
	if tree.Tickets[0].Children[0].ID != "child-1" {
		t.Errorf("expected child child-1, got %s", tree.Tickets[0].Children[0].ID)
	}
}

// --- Session Summary ---

func TestSessionSummary(t *testing.T) {
	s := testStore(t)

	s.CreateSprint(Sprint{ID: "sp-active", Name: "Active Sprint", Status: SprintActive})
	s.CreateTicket(Ticket{ID: "t-blocked", SprintID: "sp-active", Title: "Blocked"})
	s.UpdateTicket("t-blocked", StatusBlocked, "test-agent", "waiting on dep")

	ss, err := s.SessionSummary()
	if err != nil {
		t.Fatalf("SessionSummary: %v", err)
	}
	if ss.ActiveSprint == nil {
		t.Fatal("expected an active sprint")
	}
	if ss.ActiveSprint.Sprint.ID != "sp-active" {
		t.Errorf("expected sprint sp-active, got %s", ss.ActiveSprint.Sprint.ID)
	}
	if ss.ActiveAgents == nil {
		t.Error("ActiveAgents should not be nil")
	}
	if ss.RecentHandoffs == nil {
		t.Error("RecentHandoffs should not be nil")
	}
	if len(ss.BlockedTickets) != 1 {
		t.Errorf("expected 1 blocked ticket, got %d", len(ss.BlockedTickets))
	}
}

func TestSessionSummary_NoActiveSprint(t *testing.T) {
	s := testStore(t)

	ss, err := s.SessionSummary()
	if err != nil {
		t.Fatalf("SessionSummary: %v", err)
	}
	if ss.ActiveSprint != nil {
		t.Error("expected nil active sprint when none exist")
	}
	if ss.ActiveAgents == nil {
		t.Error("ActiveAgents should be non-nil empty slice")
	}
}
