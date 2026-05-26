package main

import (
	"encoding/json"
	"fmt"
)

// --- Story 1: Sprint Goal Schemas ---

func sprintGoalSetSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"sprint_id": map[string]string{"type": "string", "description": "Sprint ID to set goal for"},
			"goal":      map[string]string{"type": "string", "description": "Sprint goal text"},
		},
		"required": []string{"sprint_id", "goal"},
	}
}

func sprintGoalGetSchema() map[string]interface{} {
	return idOnlySchema("sprint_id")
}

// --- Story 2: Context Summary Schema ---

func contextSummarySchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"depth": map[string]string{"type": "integer", "description": "Detail level: 1=roadmaps+active sprint (~100 tokens), 2=+epics+ticket counts (~500 tokens), 3=+full ticket list (~2000 tokens)"},
		},
	}
}

// --- Story 3: Context Detail Schema ---

func contextDetailSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"entity_id": map[string]string{"type": "string", "description": "ID of any entity (roadmap, programme, epic, sprint, or ticket) to drill into"},
		},
		"required": []string{"entity_id"},
	}
}

// --- Story 4: Session Handoff Archive Schema ---

func sessionHandoffArchiveSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]string{"type": "string", "description": "Session handoff ID to archive"},
		},
		"required": []string{"id"},
	}
}

// --- Story 5: Startup Context Schema ---

func startupContextSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

// --- Handlers ---

func (s *Server) sprintGoalSet(args json.RawMessage) (string, bool) {
	var p struct {
		SprintID string `json:"sprint_id"`
		Goal     string `json:"goal"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return err.Error(), true
	}
	if err := s.store.SetSprintGoal(p.SprintID, p.Goal); err != nil {
		return err.Error(), true
	}
	return fmt.Sprintf("Goal set for sprint %q", p.SprintID), false
}

func (s *Server) sprintGoalGet(args json.RawMessage) (string, bool) {
	var p struct {
		SprintID string `json:"sprint_id"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return err.Error(), true
	}
	goal, err := s.store.GetSprintGoal(p.SprintID)
	if err != nil {
		return err.Error(), true
	}
	out, _ := json.Marshal(map[string]string{"sprint_id": p.SprintID, "goal": goal})
	return string(out), false
}

func (s *Server) contextSummary(args json.RawMessage) (string, bool) {
	var p struct {
		Depth int `json:"depth"`
	}
	if len(args) > 0 {
		json.Unmarshal(args, &p)
	}
	if p.Depth == 0 {
		p.Depth = 1
	}
	result, err := s.store.ContextSummary(p.Depth)
	if err != nil {
		return err.Error(), true
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	return string(data), false
}

func (s *Server) contextDetail(args json.RawMessage) (string, bool) {
	var p struct {
		EntityID string `json:"entity_id"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return err.Error(), true
	}
	detail, err := s.store.ContextDetail(p.EntityID)
	if err != nil {
		return err.Error(), true
	}
	data, _ := json.MarshalIndent(detail, "", "  ")
	return string(data), false
}

func (s *Server) sessionHandoffArchive(args json.RawMessage) (string, bool) {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return err.Error(), true
	}
	if err := s.store.ArchiveSessionHandoff(p.ID); err != nil {
		return err.Error(), true
	}
	return fmt.Sprintf("Session handoff %q archived", p.ID), false
}

func (s *Server) startupContext(args json.RawMessage) (string, bool) {
	sc, err := s.store.StartupContext()
	if err != nil {
		return err.Error(), true
	}
	data, _ := json.MarshalIndent(sc, "", "  ")
	return string(data), false
}
