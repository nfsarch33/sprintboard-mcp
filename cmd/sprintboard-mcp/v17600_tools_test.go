package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

func TestSprintGoalCreateTool(t *testing.T) {
	srv := newTestServer(t)

	_ = srv.store.CreateSprint(sprintboard.Sprint{ID: "tool-goal", Name: "Tool Goal Sprint"})

	result, isErr := srv.sprintGoalCreate(json.RawMessage(`{
		"sprint_id": "tool-goal",
		"goal_text": "Ship v17600 schema extension",
		"priority": 1
	}`))
	if isErr {
		t.Fatalf("sprintGoalCreate returned error: %s", result)
	}
	if !strings.Contains(result, `"status":"created"`) {
		t.Errorf("unexpected result: %s", result)
	}
}

func TestSprintGoalCreateTool_MissingSprint(t *testing.T) {
	srv := newTestServer(t)
	_, isErr := srv.sprintGoalCreate(json.RawMessage(`{"goal_text": "orphan"}`))
	if !isErr {
		t.Error("sprintGoalCreate with no sprint_id should error")
	}
}

func TestSprintGoalListTool(t *testing.T) {
	srv := newTestServer(t)

	srv.store.CreateSprint(sprintboard.Sprint{ID: "tool-list-goal", Name: "List Goal Sprint"})
	srv.store.CreateSprintGoal(sprintboard.SprintGoal{
		SprintID: "tool-list-goal", GoalText: "Goal Alpha", Priority: 2,
	})
	srv.store.CreateSprintGoal(sprintboard.SprintGoal{
		SprintID: "tool-list-goal", GoalText: "Goal Beta", Priority: 1,
	})

	result, isErr := srv.sprintGoalList(json.RawMessage(`{"sprint_id": "tool-list-goal"}`))
	if isErr {
		t.Fatalf("sprintGoalList returned error: %s", result)
	}
	if !strings.Contains(result, "Goal Alpha") || !strings.Contains(result, "Goal Beta") {
		t.Errorf("expected both goals in result: %s", result)
	}
}

func TestRoadmapItemCreateTool(t *testing.T) {
	srv := newTestServer(t)

	srv.store.CreateRoadmap(sprintboard.Roadmap{ID: "tool-rm", Name: "Tool Roadmap"})

	result, isErr := srv.roadmapItemCreate(json.RawMessage(`{
		"roadmap_id": "tool-rm",
		"title": "Implement FTS",
		"description": "Add PostgreSQL full-text search",
		"priority": 5
	}`))
	if isErr {
		t.Fatalf("roadmapItemCreate returned error: %s", result)
	}
	if !strings.Contains(result, `"status":"created"`) {
		t.Errorf("unexpected result: %s", result)
	}
}

func TestRoadmapItemCreateTool_MissingTitle(t *testing.T) {
	srv := newTestServer(t)
	srv.store.CreateRoadmap(sprintboard.Roadmap{ID: "tool-rm2", Name: "RM2"})

	_, isErr := srv.roadmapItemCreate(json.RawMessage(`{"roadmap_id": "tool-rm2"}`))
	if !isErr {
		t.Error("roadmapItemCreate with no title should error")
	}
}

func TestRoadmapItemListTool(t *testing.T) {
	srv := newTestServer(t)

	srv.store.CreateRoadmap(sprintboard.Roadmap{ID: "tool-rm-list", Name: "List RM"})
	srv.store.CreateRoadmapItem(sprintboard.RoadmapItem{
		RoadmapID: "tool-rm-list", Title: "Item One", Priority: 3,
	})

	result, isErr := srv.roadmapItemList(json.RawMessage(`{"roadmap_id": "tool-rm-list"}`))
	if isErr {
		t.Fatalf("roadmapItemList returned error: %s", result)
	}
	if !strings.Contains(result, "Item One") {
		t.Errorf("expected item in result: %s", result)
	}
}

func TestTicketTreeByEpicTool(t *testing.T) {
	srv := newTestServer(t)

	srv.store.CreateRoadmap(sprintboard.Roadmap{ID: "rm-ttbe", Name: "TTBE RM"})
	srv.store.CreateProgramme(sprintboard.Programme{ID: "prog-ttbe", RoadmapID: "rm-ttbe", Name: "TTBE Prog"})
	srv.store.CreateEpic(sprintboard.Epic{ID: "epic-ttbe", ProgrammeID: "prog-ttbe", Name: "TTBE Epic"})
	srv.store.CreateSprint(sprintboard.Sprint{ID: "sp-ttbe", Name: "TTBE Sprint"})
	srv.store.CreateTicket(sprintboard.Ticket{ID: "t-ttbe", SprintID: "sp-ttbe", Title: "TTBE Ticket"})
	srv.store.RawDB().Exec(`UPDATE tickets SET epic_id = 'epic-ttbe' WHERE id = 't-ttbe'`)

	result, isErr := srv.ticketTreeByEpic(json.RawMessage(`{"epic_id": "epic-ttbe"}`))
	if isErr {
		t.Fatalf("ticketTreeByEpic returned error: %s", result)
	}
	if !strings.Contains(result, "TTBE Epic") || !strings.Contains(result, "TTBE Ticket") {
		t.Errorf("unexpected result: %s", result)
	}
}

func TestSessionHandoffFTSTool(t *testing.T) {
	srv := newTestServer(t)

	srv.store.StoreSessionHandoff(sprintboard.SessionHandoff{
		ID: "fts-tool-1", SessionID: "s1", AgentID: "cursor-parent",
		Summary: "Completed the PostgreSQL migration",
	})

	result, isErr := srv.sessionHandoffFTS(json.RawMessage(`{"query": "PostgreSQL", "limit": 5}`))
	if isErr {
		t.Fatalf("sessionHandoffFTS returned error: %s", result)
	}
	if !strings.Contains(result, "PostgreSQL") {
		t.Errorf("expected match in result: %s", result)
	}
}

func TestSessionHandoffFTSTool_EmptyQuery(t *testing.T) {
	srv := newTestServer(t)
	_, isErr := srv.sessionHandoffFTS(json.RawMessage(`{"query": ""}`))
	if !isErr {
		t.Error("sessionHandoffFTS with empty query should error")
	}
}

func TestNewToolsRegistered(t *testing.T) {
	srv := newTestServer(t)

	req := JSONRPCRequest{JSONRPC: "2.0", ID: float64(1), Method: "tools/list"}
	resp := srv.handleRequest(req)
	data, _ := json.Marshal(resp.Result)

	v17600Tools := []string{
		"sprint_goal_create", "sprint_goal_list",
		"roadmap_item_create", "roadmap_item_list",
		"ticket_tree_by_epic", "session_handoff_fts",
	}
	for _, name := range v17600Tools {
		if !strings.Contains(string(data), name) {
			t.Errorf("tools/list missing %q", name)
		}
	}
}
