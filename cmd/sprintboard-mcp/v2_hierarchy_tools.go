package main

import (
	"encoding/json"
	"fmt"

	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

// --- Schemas ---

func roadmapCreateSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id":          map[string]string{"type": "string", "description": "Unique roadmap ID"},
			"name":        map[string]string{"type": "string", "description": "Roadmap name"},
			"description": map[string]string{"type": "string", "description": "Roadmap description"},
		},
		"required": []string{"id", "name"},
	}
}

func roadmapListSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func roadmapViewSchema() map[string]interface{} {
	return idOnlySchema("roadmap_id")
}

func programmeCreateSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id":          map[string]string{"type": "string", "description": "Unique programme ID"},
			"roadmap_id":  map[string]string{"type": "string", "description": "Parent roadmap ID (optional)"},
			"name":        map[string]string{"type": "string", "description": "Programme name"},
			"description": map[string]string{"type": "string", "description": "Programme description"},
		},
		"required": []string{"id", "name"},
	}
}

func programmeListSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"roadmap_id": map[string]string{"type": "string", "description": "Filter by parent roadmap ID"},
		},
	}
}

func programmeViewSchema() map[string]interface{} {
	return idOnlySchema("programme_id")
}

func epicCreateSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id":           map[string]string{"type": "string", "description": "Unique epic ID"},
			"programme_id": map[string]string{"type": "string", "description": "Parent programme ID (optional)"},
			"name":         map[string]string{"type": "string", "description": "Epic name"},
			"description":  map[string]string{"type": "string", "description": "Epic description"},
		},
		"required": []string{"id", "name"},
	}
}

func epicListSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"programme_id": map[string]string{"type": "string", "description": "Filter by parent programme ID"},
		},
	}
}

func epicViewSchema() map[string]interface{} {
	return idOnlySchema("epic_id")
}

func epicBurndownSchema() map[string]interface{} {
	return idOnlySchema("epic_id")
}

func ticketLogTimeSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"ticket_id": map[string]string{"type": "string", "description": "Ticket to log time against"},
			"minutes":   map[string]string{"type": "integer", "description": "Minutes to add to actual_minutes"},
		},
		"required": []string{"ticket_id", "minutes"},
	}
}

func ticketEstimateSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"ticket_id": map[string]string{"type": "string", "description": "Ticket to set estimate for"},
			"minutes":   map[string]string{"type": "integer", "description": "Estimated minutes for the ticket"},
		},
		"required": []string{"ticket_id", "minutes"},
	}
}

func sprintTimeReportSchema() map[string]interface{} {
	return idOnlySchema("sprint_id")
}

func ticketTreeSchema() map[string]interface{} {
	return idOnlySchema("sprint_id")
}

func sessionSummarySchema() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

// --- Handlers ---

func (s *Server) roadmapCreate(args json.RawMessage) (string, bool) {
	var p struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return err.Error(), true
	}
	if p.ID == "" {
		return "id is required", true
	}
	if p.Name == "" {
		return "name is required", true
	}
	err := s.store.CreateRoadmap(sprintboard.Roadmap{
		ID: p.ID, Name: p.Name, Description: p.Description,
	})
	if err != nil {
		return err.Error(), true
	}
	return fmt.Sprintf("Roadmap %q created", p.ID), false
}

func (s *Server) roadmapList(args json.RawMessage) (string, bool) {
	roadmaps, err := s.store.ListRoadmaps()
	if err != nil {
		return err.Error(), true
	}
	if roadmaps == nil {
		roadmaps = []sprintboard.Roadmap{}
	}
	data, _ := json.MarshalIndent(roadmaps, "", "  ")
	return string(data), false
}

func (s *Server) roadmapView(args json.RawMessage) (string, bool) {
	var p struct {
		RoadmapID string `json:"roadmap_id"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return err.Error(), true
	}
	r, err := s.store.GetRoadmap(p.RoadmapID)
	if err != nil {
		return err.Error(), true
	}
	data, _ := json.MarshalIndent(r, "", "  ")
	return string(data), false
}

func (s *Server) programmeCreate(args json.RawMessage) (string, bool) {
	var p struct {
		ID          string `json:"id"`
		RoadmapID   string `json:"roadmap_id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return err.Error(), true
	}
	if p.ID == "" {
		return "id is required", true
	}
	if p.Name == "" {
		return "name is required", true
	}
	err := s.store.CreateProgramme(sprintboard.Programme{
		ID: p.ID, RoadmapID: p.RoadmapID, Name: p.Name, Description: p.Description,
	})
	if err != nil {
		return err.Error(), true
	}
	return fmt.Sprintf("Programme %q created", p.ID), false
}

func (s *Server) programmeList(args json.RawMessage) (string, bool) {
	var p struct {
		RoadmapID string `json:"roadmap_id"`
	}
	if len(args) > 0 {
		json.Unmarshal(args, &p)
	}
	programmes, err := s.store.ListProgrammes(p.RoadmapID)
	if err != nil {
		return err.Error(), true
	}
	if programmes == nil {
		programmes = []sprintboard.Programme{}
	}
	data, _ := json.MarshalIndent(programmes, "", "  ")
	return string(data), false
}

func (s *Server) programmeView(args json.RawMessage) (string, bool) {
	var p struct {
		ProgrammeID string `json:"programme_id"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return err.Error(), true
	}
	prog, err := s.store.GetProgramme(p.ProgrammeID)
	if err != nil {
		return err.Error(), true
	}
	data, _ := json.MarshalIndent(prog, "", "  ")
	return string(data), false
}

func (s *Server) epicCreate(args json.RawMessage) (string, bool) {
	var p struct {
		ID          string `json:"id"`
		ProgrammeID string `json:"programme_id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return err.Error(), true
	}
	if p.ID == "" {
		return "id is required", true
	}
	if p.Name == "" {
		return "name is required", true
	}
	err := s.store.CreateEpic(sprintboard.Epic{
		ID: p.ID, ProgrammeID: p.ProgrammeID, Name: p.Name, Description: p.Description,
	})
	if err != nil {
		return err.Error(), true
	}
	return fmt.Sprintf("Epic %q created", p.ID), false
}

func (s *Server) epicList(args json.RawMessage) (string, bool) {
	var p struct {
		ProgrammeID string `json:"programme_id"`
	}
	if len(args) > 0 {
		json.Unmarshal(args, &p)
	}
	epics, err := s.store.ListEpics(p.ProgrammeID)
	if err != nil {
		return err.Error(), true
	}
	if epics == nil {
		epics = []sprintboard.Epic{}
	}
	data, _ := json.MarshalIndent(epics, "", "  ")
	return string(data), false
}

func (s *Server) epicView(args json.RawMessage) (string, bool) {
	var p struct {
		EpicID string `json:"epic_id"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return err.Error(), true
	}
	e, err := s.store.GetEpic(p.EpicID)
	if err != nil {
		return err.Error(), true
	}
	data, _ := json.MarshalIndent(e, "", "  ")
	return string(data), false
}

func (s *Server) epicBurndownTool(args json.RawMessage) (string, bool) {
	var p struct {
		EpicID string `json:"epic_id"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return err.Error(), true
	}
	bd, err := s.store.EpicBurndown(p.EpicID)
	if err != nil {
		return err.Error(), true
	}
	data, _ := json.MarshalIndent(bd, "", "  ")
	return string(data), false
}

func (s *Server) ticketLogTime(args json.RawMessage) (string, bool) {
	var p struct {
		TicketID string `json:"ticket_id"`
		Minutes  int    `json:"minutes"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return err.Error(), true
	}
	if p.Minutes <= 0 {
		return "minutes must be positive", true
	}
	if err := s.store.LogTicketTime(p.TicketID, p.Minutes); err != nil {
		return err.Error(), true
	}
	return fmt.Sprintf("Logged %d minutes on ticket %q", p.Minutes, p.TicketID), false
}

func (s *Server) ticketEstimateTool(args json.RawMessage) (string, bool) {
	var p struct {
		TicketID string `json:"ticket_id"`
		Minutes  int    `json:"minutes"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return err.Error(), true
	}
	if p.Minutes <= 0 {
		return "minutes must be positive", true
	}
	if err := s.store.SetTicketEstimateMinutes(p.TicketID, p.Minutes); err != nil {
		return err.Error(), true
	}
	return fmt.Sprintf("Estimate set to %d minutes on ticket %q", p.Minutes, p.TicketID), false
}

func (s *Server) sprintTimeReportTool(args json.RawMessage) (string, bool) {
	var p struct {
		SprintID string `json:"sprint_id"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return err.Error(), true
	}
	rpt, err := s.store.SprintTimeReport(p.SprintID)
	if err != nil {
		return err.Error(), true
	}
	data, _ := json.MarshalIndent(rpt, "", "  ")
	return string(data), false
}

func (s *Server) ticketTreeTool(args json.RawMessage) (string, bool) {
	var p struct {
		SprintID string `json:"sprint_id"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return err.Error(), true
	}
	tree, err := s.store.TicketTree(p.SprintID)
	if err != nil {
		return err.Error(), true
	}
	data, _ := json.MarshalIndent(tree, "", "  ")
	return string(data), false
}

func (s *Server) sessionSummaryTool(args json.RawMessage) (string, bool) {
	ss, err := s.store.SessionSummary()
	if err != nil {
		return err.Error(), true
	}
	data, _ := json.MarshalIndent(ss, "", "  ")
	return string(data), false
}
