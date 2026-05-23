package main

import (
	"encoding/json"

	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

// v8900 MCP tools layer the new REST endpoints (B16-B18) into the JSON-RPC
// surface. These are deliberately separate from the existing ticket_search
// (which performs semantic vector similarity) so callers can keep using
// either: ticket_search_filter for structured SQL filters, sprint_history
// for paged sprint lists, sprint_metrics for the rollup payload.

func ticketSearchFilterSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"q":            map[string]string{"type": "string", "description": "Free-text fragment matched against title/description/acceptance"},
			"status":       map[string]string{"type": "string"},
			"owner":        map[string]string{"type": "string"},
			"sprint_id":    map[string]string{"type": "string"},
			"priority_min": map[string]string{"type": "integer"},
			"label":        map[string]string{"type": "string", "description": "Single label; tickets with this label match"},
			"limit":        map[string]string{"type": "integer"},
		},
	}
}

func sprintHistorySchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"status": map[string]string{"type": "string", "description": "Optional sprint status filter (planned/active/closed)"},
		},
	}
}

func sprintMetricsSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"sprint_id": map[string]string{"type": "string"},
		},
		"required": []string{"sprint_id"},
	}
}

func (s *Server) ticketSearchFilter(args json.RawMessage) (string, bool) {
	var p struct {
		Q           string `json:"q"`
		Status      string `json:"status"`
		Owner       string `json:"owner"`
		SprintID    string `json:"sprint_id"`
		PriorityMin int    `json:"priority_min"`
		Label       string `json:"label"`
		Limit       int    `json:"limit"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &p); err != nil {
			return err.Error(), true
		}
	}
	filter := sprintboard.TicketFilter{
		Query:       p.Q,
		Status:      sprintboard.TicketStatus(p.Status),
		Owner:       p.Owner,
		SprintID:    p.SprintID,
		PriorityMin: p.PriorityMin,
		Limit:       p.Limit,
	}
	if p.Label != "" {
		filter.Labels = []string{p.Label}
	}
	tickets, err := s.store.SearchTickets(filter)
	if err != nil {
		return err.Error(), true
	}
	if tickets == nil {
		tickets = []sprintboard.Ticket{}
	}
	data, _ := json.MarshalIndent(tickets, "", "  ")
	return string(data), false
}

func (s *Server) sprintHistory(args json.RawMessage) (string, bool) {
	var p struct {
		Status string `json:"status"`
	}
	if len(args) > 0 {
		_ = json.Unmarshal(args, &p)
	}
	sprints, err := s.store.ListSprints()
	if err != nil {
		return err.Error(), true
	}
	if p.Status != "" {
		filtered := make([]sprintboard.Sprint, 0, len(sprints))
		for _, sp := range sprints {
			if string(sp.Status) == p.Status {
				filtered = append(filtered, sp)
			}
		}
		sprints = filtered
	}
	if sprints == nil {
		sprints = []sprintboard.Sprint{}
	}
	data, _ := json.MarshalIndent(sprints, "", "  ")
	return string(data), false
}

func (s *Server) sprintMetrics(args json.RawMessage) (string, bool) {
	var p struct {
		SprintID string `json:"sprint_id"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return err.Error(), true
	}
	if p.SprintID == "" {
		return "sprint_id is required", true
	}
	summary, err := s.store.SprintSummary(p.SprintID)
	if err != nil {
		return err.Error(), true
	}
	slas, err := s.store.SprintSLAs(p.SprintID)
	if err != nil {
		return err.Error(), true
	}
	velocity, err := s.store.SprintVelocity(p.SprintID)
	if err != nil {
		return err.Error(), true
	}
	burndown, err := s.store.SprintBurndown(p.SprintID)
	if err != nil {
		return err.Error(), true
	}
	out := map[string]interface{}{
		"sprint":            summary.Sprint,
		"tickets_by_status": summary.TicketsByStatus,
		"total_tickets":     summary.TotalTickets,
		"slas":              slas,
		"velocity":          velocity,
		"burndown":          burndown,
	}
	data, _ := json.MarshalIndent(out, "", "  ")
	return string(data), false
}
