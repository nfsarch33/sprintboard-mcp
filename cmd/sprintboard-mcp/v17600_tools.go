package main

import (
	"encoding/json"
	"fmt"

	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

// --- Sprint Goal Schemas ---

func sprintGoalListSchema() map[string]interface{} {
	return idOnlySchema("sprint_id")
}

func sprintGoalCreateSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"sprint_id": map[string]string{"type": "string", "description": "Sprint to add goal to"},
			"goal_text": map[string]string{"type": "string", "description": "Goal text"},
			"priority":  map[string]string{"type": "integer", "description": "Priority (higher = more important, default 0)"},
		},
		"required": []string{"sprint_id", "goal_text"},
	}
}

// --- Roadmap Item Schemas ---

func roadmapItemCreateSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"roadmap_id":  map[string]string{"type": "string", "description": "Parent roadmap ID"},
			"title":       map[string]string{"type": "string", "description": "Item title"},
			"epic_id":     map[string]string{"type": "string", "description": "Optional linked epic ID"},
			"description": map[string]string{"type": "string", "description": "Item description"},
			"priority":    map[string]string{"type": "integer", "description": "Priority (higher = more important, default 0)"},
		},
		"required": []string{"roadmap_id", "title"},
	}
}

func roadmapItemListSchema() map[string]interface{} {
	return idOnlySchema("roadmap_id")
}

// --- Ticket Tree by Epic Schema ---

func ticketTreeByEpicSchema() map[string]interface{} {
	return idOnlySchema("epic_id")
}

// --- Session Handoff FTS Schema ---

func sessionHandoffFTSSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]string{"type": "string", "description": "Full-text search query (BM25/ts_rank on PG, LIKE fallback on SQLite)"},
			"limit": map[string]string{"type": "integer", "description": "Max results (default 10, max 50)"},
		},
		"required": []string{"query"},
	}
}

// --- Handlers ---

func (s *Server) sprintGoalCreate(args json.RawMessage) (string, bool) {
	var p struct {
		SprintID string `json:"sprint_id"`
		GoalText string `json:"goal_text"`
		Priority int    `json:"priority"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return err.Error(), true
	}
	id, err := s.store.CreateSprintGoal(sprintboard.SprintGoal{
		SprintID: p.SprintID, GoalText: p.GoalText, Priority: p.Priority,
	})
	if err != nil {
		return err.Error(), true
	}
	out, _ := json.Marshal(map[string]interface{}{"id": id, "status": "created"})
	return string(out), false
}

func (s *Server) sprintGoalList(args json.RawMessage) (string, bool) {
	var p struct {
		SprintID string `json:"sprint_id"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return err.Error(), true
	}
	goals, err := s.store.ListSprintGoals(p.SprintID)
	if err != nil {
		return err.Error(), true
	}
	if goals == nil {
		goals = []sprintboard.SprintGoal{}
	}
	data, _ := json.MarshalIndent(goals, "", "  ")
	return string(data), false
}

func (s *Server) roadmapItemCreate(args json.RawMessage) (string, bool) {
	var p struct {
		RoadmapID   string `json:"roadmap_id"`
		Title       string `json:"title"`
		EpicID      string `json:"epic_id"`
		Description string `json:"description"`
		Priority    int    `json:"priority"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return err.Error(), true
	}
	id, err := s.store.CreateRoadmapItem(sprintboard.RoadmapItem{
		RoadmapID:   p.RoadmapID,
		Title:       p.Title,
		EpicID:      p.EpicID,
		Description: p.Description,
		Priority:    p.Priority,
	})
	if err != nil {
		return err.Error(), true
	}
	out, _ := json.Marshal(map[string]interface{}{"id": id, "status": "created"})
	return string(out), false
}

func (s *Server) roadmapItemList(args json.RawMessage) (string, bool) {
	var p struct {
		RoadmapID string `json:"roadmap_id"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return err.Error(), true
	}
	items, err := s.store.ListRoadmapItems(p.RoadmapID)
	if err != nil {
		return err.Error(), true
	}
	if items == nil {
		items = []sprintboard.RoadmapItem{}
	}
	data, _ := json.MarshalIndent(items, "", "  ")
	return string(data), false
}

func (s *Server) ticketTreeByEpic(args json.RawMessage) (string, bool) {
	var p struct {
		EpicID string `json:"epic_id"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return err.Error(), true
	}
	tree, err := s.store.TicketTreeByEpic(p.EpicID)
	if err != nil {
		return fmt.Sprintf("ticket tree by epic: %v", err), true
	}
	data, _ := json.MarshalIndent(tree, "", "  ")
	return string(data), false
}

func (s *Server) sessionHandoffFTS(args json.RawMessage) (string, bool) {
	var p struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return err.Error(), true
	}
	results, err := s.store.SearchSessionHandoffsFTS(p.Query, p.Limit)
	if err != nil {
		return fmt.Sprintf("FTS search: %v", err), true
	}
	if results == nil {
		results = []sprintboard.SessionHandoff{}
	}
	data, _ := json.MarshalIndent(results, "", "  ")
	return string(data), false
}
