package sprintboard

import (
	"testing"
	"time"
)

func setupTestStoreForDisclosure(t *testing.T) *Store {
	t.Helper()
	store, err := Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestSetSprintGoal(t *testing.T) {
	store := setupTestStoreForDisclosure(t)

	if err := store.CreateSprint(Sprint{ID: "v17300", Name: "Progressive Disclosure"}); err != nil {
		t.Fatalf("create sprint: %v", err)
	}

	if err := store.SetSprintGoal("v17300", "Token-efficient agent startup"); err != nil {
		t.Fatalf("set goal: %v", err)
	}

	goal, err := store.GetSprintGoal("v17300")
	if err != nil {
		t.Fatalf("get goal: %v", err)
	}
	if goal != "Token-efficient agent startup" {
		t.Errorf("got %q, want %q", goal, "Token-efficient agent startup")
	}
}

func TestSetSprintGoal_NotFound(t *testing.T) {
	store := setupTestStoreForDisclosure(t)

	err := store.SetSprintGoal("nonexistent", "goal")
	if err == nil {
		t.Fatal("expected error for nonexistent sprint")
	}
}

func TestGetSprintGoal_Empty(t *testing.T) {
	store := setupTestStoreForDisclosure(t)

	if err := store.CreateSprint(Sprint{ID: "v17300", Name: "Test"}); err != nil {
		t.Fatalf("create sprint: %v", err)
	}

	goal, err := store.GetSprintGoal("v17300")
	if err != nil {
		t.Fatalf("get goal: %v", err)
	}
	if goal != "" {
		t.Errorf("expected empty goal, got %q", goal)
	}
}

func TestContextSummary_Depth1(t *testing.T) {
	store := setupTestStoreForDisclosure(t)

	store.CreateRoadmap(Roadmap{ID: "rm-1", Name: "Personal Stack"})
	store.CreateSprint(Sprint{ID: "v17300", Name: "Progressive", Status: SprintActive})
	store.SetSprintGoal("v17300", "Token savings")

	result, err := store.ContextSummary(1)
	if err != nil {
		t.Fatalf("context summary: %v", err)
	}

	d1, ok := result.(ContextSummaryDepth1)
	if !ok {
		t.Fatalf("expected ContextSummaryDepth1, got %T", result)
	}
	if len(d1.Roadmaps) != 1 || d1.Roadmaps[0].Name != "Personal Stack" {
		t.Errorf("roadmaps: %+v", d1.Roadmaps)
	}
	if d1.ActiveSprint == nil || d1.ActiveSprint.Goal != "Token savings" {
		t.Errorf("active sprint: %+v", d1.ActiveSprint)
	}
}

func TestContextSummary_Depth2(t *testing.T) {
	store := setupTestStoreForDisclosure(t)

	store.CreateSprint(Sprint{ID: "v17300", Name: "Test", Status: SprintActive})
	store.CreateEpic(Epic{ID: "ep-1", Name: "Test Epic", Status: EpicInProgress})
	store.CreateTicket(Ticket{ID: "t-1", SprintID: "v17300", Title: "Story 1"})
	store.CreateTicket(Ticket{ID: "t-2", SprintID: "v17300", Title: "Story 2", Status: StatusDone})

	result, err := store.ContextSummary(2)
	if err != nil {
		t.Fatalf("context summary depth 2: %v", err)
	}

	d2, ok := result.(ContextSummaryDepth2)
	if !ok {
		t.Fatalf("expected ContextSummaryDepth2, got %T", result)
	}
	if len(d2.ActiveEpics) != 1 {
		t.Errorf("expected 1 active epic, got %d", len(d2.ActiveEpics))
	}
	if d2.TotalTickets != 2 {
		t.Errorf("expected 2 tickets, got %d", d2.TotalTickets)
	}
}

func TestContextSummary_Depth3(t *testing.T) {
	store := setupTestStoreForDisclosure(t)

	store.CreateSprint(Sprint{ID: "v17300", Name: "Test", Status: SprintActive})
	store.CreateTicket(Ticket{ID: "t-1", SprintID: "v17300", Title: "Story 1", Description: "Full detail"})

	result, err := store.ContextSummary(3)
	if err != nil {
		t.Fatalf("context summary depth 3: %v", err)
	}

	d3, ok := result.(ContextSummaryDepth3)
	if !ok {
		t.Fatalf("expected ContextSummaryDepth3, got %T", result)
	}
	if len(d3.Tickets) != 1 || d3.Tickets[0].Description != "Full detail" {
		t.Errorf("tickets: %+v", d3.Tickets)
	}
}

func TestContextSummary_NoActiveSprint(t *testing.T) {
	store := setupTestStoreForDisclosure(t)

	result, err := store.ContextSummary(1)
	if err != nil {
		t.Fatalf("context summary: %v", err)
	}

	d1, ok := result.(ContextSummaryDepth1)
	if !ok {
		t.Fatalf("expected ContextSummaryDepth1, got %T", result)
	}
	if d1.ActiveSprint != nil {
		t.Errorf("expected nil active sprint, got %+v", d1.ActiveSprint)
	}
}

func TestContextDetail_Roadmap(t *testing.T) {
	store := setupTestStoreForDisclosure(t)

	store.CreateRoadmap(Roadmap{ID: "rm-1", Name: "Personal Stack"})
	store.CreateProgramme(Programme{ID: "pg-1", RoadmapID: "rm-1", Name: "SprintBoard"})

	detail, err := store.ContextDetail("rm-1")
	if err != nil {
		t.Fatalf("context detail: %v", err)
	}
	if detail.EntityType != "roadmap" {
		t.Errorf("entity_type: %q", detail.EntityType)
	}
	children, ok := detail.Children.([]Programme)
	if !ok {
		t.Fatalf("children type: %T", detail.Children)
	}
	if len(children) != 1 || children[0].ID != "pg-1" {
		t.Errorf("children: %+v", children)
	}
}

func TestContextDetail_Sprint(t *testing.T) {
	store := setupTestStoreForDisclosure(t)

	store.CreateSprint(Sprint{ID: "v17300", Name: "Test"})
	store.CreateTicket(Ticket{ID: "t-1", SprintID: "v17300", Title: "Story"})

	detail, err := store.ContextDetail("v17300")
	if err != nil {
		t.Fatalf("context detail: %v", err)
	}
	if detail.EntityType != "sprint" {
		t.Errorf("entity_type: %q", detail.EntityType)
	}
}

func TestContextDetail_Ticket(t *testing.T) {
	store := setupTestStoreForDisclosure(t)

	store.CreateSprint(Sprint{ID: "v17300", Name: "Test"})
	store.CreateTicket(Ticket{ID: "t-1", SprintID: "v17300", Title: "Story"})

	detail, err := store.ContextDetail("t-1")
	if err != nil {
		t.Fatalf("context detail: %v", err)
	}
	if detail.EntityType != "ticket" {
		t.Errorf("entity_type: %q", detail.EntityType)
	}
}

func TestContextDetail_NotFound(t *testing.T) {
	store := setupTestStoreForDisclosure(t)

	_, err := store.ContextDetail("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent entity")
	}
}

func TestAutoArchiveOldHandoffs(t *testing.T) {
	store := setupTestStoreForDisclosure(t)

	old := time.Now().Add(-8 * 24 * time.Hour).Format(time.RFC3339)
	store.db.Exec(
		`INSERT INTO session_handoffs (id, session_id, agent_id, summary, created_at, archived)
		 VALUES (?, ?, ?, ?, ?, 0)`,
		"old-1", "sess-1", "cursor-parent", "old handoff", old,
	)
	store.StoreSessionHandoff(SessionHandoff{
		ID: "new-1", SessionID: "sess-2", AgentID: "cursor-parent", Summary: "fresh handoff",
	})

	if err := store.AutoArchiveOldHandoffs(); err != nil {
		t.Fatalf("auto archive: %v", err)
	}

	nonArchived, err := store.LatestSessionHandoffsFiltered("", 10, false)
	if err != nil {
		t.Fatalf("list filtered: %v", err)
	}
	for _, h := range nonArchived {
		if h.ID == "old-1" {
			t.Error("old-1 should be archived but still appears in non-archived results")
		}
	}

	all, err := store.LatestSessionHandoffsFiltered("", 10, true)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	found := false
	for _, h := range all {
		if h.ID == "old-1" {
			found = true
		}
	}
	if !found {
		t.Error("old-1 should appear in include_archived results")
	}
}

func TestArchiveSessionHandoff(t *testing.T) {
	store := setupTestStoreForDisclosure(t)

	store.StoreSessionHandoff(SessionHandoff{
		ID: "h-1", SessionID: "sess-1", AgentID: "cursor-parent", Summary: "test",
	})

	if err := store.ArchiveSessionHandoff("h-1"); err != nil {
		t.Fatalf("archive: %v", err)
	}

	nonArchived, _ := store.LatestSessionHandoffsFiltered("", 10, false)
	for _, h := range nonArchived {
		if h.ID == "h-1" {
			t.Error("h-1 should be archived")
		}
	}
}

func TestArchiveSessionHandoff_NotFound(t *testing.T) {
	store := setupTestStoreForDisclosure(t)

	err := store.ArchiveSessionHandoff("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent handoff")
	}
}

func TestStartupContext(t *testing.T) {
	store := setupTestStoreForDisclosure(t)

	store.CreateSprint(Sprint{ID: "v17300", Name: "Test Sprint", Status: SprintActive})
	store.SetSprintGoal("v17300", "Ship progressive disclosure")
	store.CreateRoadmap(Roadmap{ID: "rm-1", Name: "Personal Stack"})
	store.StoreSessionHandoff(SessionHandoff{
		ID: "h-1", SessionID: "sess-1", AgentID: "cursor-parent", Summary: "Previous work",
	})

	sc, err := store.StartupContext()
	if err != nil {
		t.Fatalf("startup context: %v", err)
	}

	if len(sc.LatestHandoffs) != 1 {
		t.Errorf("expected 1 handoff, got %d", len(sc.LatestHandoffs))
	}
	if sc.ActiveSprint == nil || sc.ActiveSprint.Goal != "Ship progressive disclosure" {
		t.Errorf("active sprint: %+v", sc.ActiveSprint)
	}
	if sc.ContextSummary == nil {
		t.Error("context summary should not be nil")
	}
}

func TestStartupContext_Empty(t *testing.T) {
	store := setupTestStoreForDisclosure(t)

	sc, err := store.StartupContext()
	if err != nil {
		t.Fatalf("startup context: %v", err)
	}

	if len(sc.LatestHandoffs) != 0 {
		t.Errorf("expected 0 handoffs, got %d", len(sc.LatestHandoffs))
	}
	if sc.ActiveSprint != nil {
		t.Errorf("expected nil active sprint, got %+v", sc.ActiveSprint)
	}
}

func TestContextSummary_DepthClamp(t *testing.T) {
	store := setupTestStoreForDisclosure(t)

	r1, err := store.ContextSummary(0)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r1.(ContextSummaryDepth1); !ok {
		t.Errorf("depth 0 should clamp to 1, got %T", r1)
	}

	r2, err := store.ContextSummary(99)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r2.(ContextSummaryDepth3); !ok {
		t.Errorf("depth 99 should clamp to 3, got %T", r2)
	}
}
