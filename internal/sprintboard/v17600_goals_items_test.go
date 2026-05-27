package sprintboard

import (
	"testing"
)

func TestCreateSprintGoal(t *testing.T) {
	store := testStore(t)

	store.CreateSprint(Sprint{ID: "goal-sprint", Name: "Goal Sprint", Status: SprintPlanned})

	tests := []struct {
		name    string
		goal    SprintGoal
		wantErr bool
	}{
		{
			name:    "valid goal",
			goal:    SprintGoal{SprintID: "goal-sprint", GoalText: "Ship feature X", Priority: 1},
			wantErr: false,
		},
		{
			name:    "missing sprint_id",
			goal:    SprintGoal{GoalText: "Ship feature Y"},
			wantErr: true,
		},
		{
			name:    "missing goal_text",
			goal:    SprintGoal{SprintID: "goal-sprint"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := store.CreateSprintGoal(tt.goal)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateSprintGoal() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && id <= 0 {
				t.Errorf("CreateSprintGoal() returned id = %d, want > 0", id)
			}
		})
	}
}

func TestListSprintGoals(t *testing.T) {
	store := testStore(t)

	store.CreateSprint(Sprint{ID: "list-goals", Name: "List Goals Sprint", Status: SprintPlanned})
	store.CreateSprintGoal(SprintGoal{SprintID: "list-goals", GoalText: "Goal A", Priority: 2})
	store.CreateSprintGoal(SprintGoal{SprintID: "list-goals", GoalText: "Goal B", Priority: 1})
	store.CreateSprintGoal(SprintGoal{SprintID: "list-goals", GoalText: "Goal C", Priority: 0})

	goals, err := store.ListSprintGoals("list-goals")
	if err != nil {
		t.Fatalf("ListSprintGoals() error = %v", err)
	}
	if len(goals) != 3 {
		t.Fatalf("ListSprintGoals() returned %d goals, want 3", len(goals))
	}
	if goals[0].GoalText != "Goal A" {
		t.Errorf("highest priority goal should be first, got %q", goals[0].GoalText)
	}
}

func TestListSprintGoals_EmptySprintID(t *testing.T) {
	store := testStore(t)
	_, err := store.ListSprintGoals("")
	if err == nil {
		t.Error("ListSprintGoals('') should return error")
	}
}

func TestUpdateSprintGoalStatus(t *testing.T) {
	store := testStore(t)

	store.CreateSprint(Sprint{ID: "update-goal", Name: "Update Goal Sprint", Status: SprintPlanned})
	id, _ := store.CreateSprintGoal(SprintGoal{SprintID: "update-goal", GoalText: "Goal X"})

	if err := store.UpdateSprintGoalStatus(id, "achieved"); err != nil {
		t.Fatalf("UpdateSprintGoalStatus() error = %v", err)
	}

	goals, _ := store.ListSprintGoals("update-goal")
	if len(goals) != 1 || goals[0].Status != "achieved" {
		t.Errorf("goal status should be 'achieved', got %q", goals[0].Status)
	}
}

func TestUpdateSprintGoalStatus_NotFound(t *testing.T) {
	store := testStore(t)
	if err := store.UpdateSprintGoalStatus(999999, "achieved"); err == nil {
		t.Error("UpdateSprintGoalStatus(nonexistent) should return error")
	}
}

func TestCreateRoadmapItem(t *testing.T) {
	store := testStore(t)

	store.CreateRoadmap(Roadmap{ID: "rm-items", Name: "Item Roadmap"})

	tests := []struct {
		name    string
		item    RoadmapItem
		wantErr bool
	}{
		{
			name:    "valid item without epic",
			item:    RoadmapItem{RoadmapID: "rm-items", Title: "Item A", Priority: 1},
			wantErr: false,
		},
		{
			name:    "valid item with description",
			item:    RoadmapItem{RoadmapID: "rm-items", Title: "Item B", Description: "Detailed desc"},
			wantErr: false,
		},
		{
			name:    "missing roadmap_id",
			item:    RoadmapItem{Title: "Item C"},
			wantErr: true,
		},
		{
			name:    "missing title",
			item:    RoadmapItem{RoadmapID: "rm-items"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := store.CreateRoadmapItem(tt.item)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateRoadmapItem() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && id <= 0 {
				t.Errorf("CreateRoadmapItem() returned id = %d, want > 0", id)
			}
		})
	}
}

func TestListRoadmapItems(t *testing.T) {
	store := testStore(t)

	store.CreateRoadmap(Roadmap{ID: "rm-list", Name: "List Roadmap"})
	store.CreateRoadmapItem(RoadmapItem{RoadmapID: "rm-list", Title: "Item X", Priority: 3})
	store.CreateRoadmapItem(RoadmapItem{RoadmapID: "rm-list", Title: "Item Y", Priority: 1})

	items, err := store.ListRoadmapItems("rm-list")
	if err != nil {
		t.Fatalf("ListRoadmapItems() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("ListRoadmapItems() returned %d items, want 2", len(items))
	}
	if items[0].Title != "Item X" {
		t.Errorf("highest priority item should be first, got %q", items[0].Title)
	}
}

func TestListRoadmapItems_EmptyRoadmapID(t *testing.T) {
	store := testStore(t)
	_, err := store.ListRoadmapItems("")
	if err == nil {
		t.Error("ListRoadmapItems('') should return error")
	}
}

func TestTicketTreeByEpic(t *testing.T) {
	store := testStore(t)

	store.CreateRoadmap(Roadmap{ID: "rm-tree", Name: "Tree Roadmap"})
	store.CreateProgramme(Programme{ID: "prog-tree", RoadmapID: "rm-tree", Name: "Tree Programme"})
	store.CreateEpic(Epic{ID: "epic-tree", ProgrammeID: "prog-tree", Name: "Tree Epic"})
	store.CreateSprint(Sprint{ID: "sp-tree-17600", Name: "Tree Sprint", Status: SprintPlanned})

	store.CreateTicket(Ticket{ID: "parent-t", SprintID: "sp-tree-17600", Title: "Parent task", Status: StatusBacklog})
	store.CreateTicket(Ticket{ID: "child-t", SprintID: "sp-tree-17600", Title: "Child task", Status: StatusBacklog})

	store.db.Exec(`UPDATE tickets SET epic_id = ? WHERE id = ?`, "epic-tree", "parent-t")
	store.db.Exec(`UPDATE tickets SET epic_id = ?, parent_ticket_id = ? WHERE id = ?`, "epic-tree", "parent-t", "child-t")

	tree, err := store.TicketTreeByEpic("epic-tree")
	if err != nil {
		t.Fatalf("TicketTreeByEpic() error = %v", err)
	}
	if tree.Epic == nil || tree.Epic.ID != "epic-tree" {
		t.Error("TicketTreeByEpic() should include the epic")
	}
	if len(tree.Tickets) != 1 {
		t.Fatalf("TicketTreeByEpic() should return 1 root ticket, got %d", len(tree.Tickets))
	}
	if tree.Tickets[0].ID != "parent-t" {
		t.Errorf("root ticket should be parent-t, got %q", tree.Tickets[0].ID)
	}
	if len(tree.Tickets[0].Children) != 1 {
		t.Errorf("parent should have 1 child, got %d", len(tree.Tickets[0].Children))
	}
}

func TestTicketTreeByEpic_NotFound(t *testing.T) {
	store := testStore(t)
	_, err := store.TicketTreeByEpic("nonexistent-epic")
	if err == nil {
		t.Error("TicketTreeByEpic(nonexistent) should return error")
	}
}

func TestSearchSessionHandoffsFTS(t *testing.T) {
	store := testStore(t)

	store.StoreSessionHandoff(SessionHandoff{
		ID: "fts-1", SessionID: "s1", AgentID: "cursor-parent",
		Summary: "Completed the PostgreSQL migration with zero downtime",
	})
	store.StoreSessionHandoff(SessionHandoff{
		ID: "fts-2", SessionID: "s2", AgentID: "codex",
		Summary: "Fixed memory leak in the router",
	})
	store.StoreSessionHandoff(SessionHandoff{
		ID: "fts-3", SessionID: "s3", AgentID: "cursor-parent",
		Summary:      "Updated CI pipeline for Go tests",
		CarryForward: "Need to verify PostgreSQL integration tests",
	})

	results, err := store.SearchSessionHandoffsFTS("PostgreSQL", 10)
	if err != nil {
		t.Fatalf("SearchSessionHandoffsFTS() error = %v", err)
	}
	if len(results) < 1 {
		t.Fatal("SearchSessionHandoffsFTS('PostgreSQL') should return at least 1 result")
	}

	found := false
	for _, h := range results {
		if h.ID == "fts-1" {
			found = true
		}
	}
	if !found {
		t.Error("SearchSessionHandoffsFTS('PostgreSQL') should include fts-1")
	}
}

func TestSearchSessionHandoffsFTS_Empty(t *testing.T) {
	store := testStore(t)
	_, err := store.SearchSessionHandoffsFTS("", 10)
	if err == nil {
		t.Error("SearchSessionHandoffsFTS('') should return error")
	}
}

func TestSearchSessionHandoffsFTS_LimitBounds(t *testing.T) {
	store := testStore(t)
	store.StoreSessionHandoff(SessionHandoff{
		ID: "limit-1", SessionID: "s1", AgentID: "a1", Summary: "test limit bounds",
	})

	results, err := store.SearchSessionHandoffsFTS("limit", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = results

	results, err = store.SearchSessionHandoffsFTS("limit", 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = results
}
