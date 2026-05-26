package main

import (
	"encoding/json"
	"fmt"

	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

func sessionHandoffStoreSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id":            map[string]string{"type": "string", "description": "Unique handoff ID (e.g. v15000-cursor-parent-2026-05-26)"},
			"session_id":    map[string]string{"type": "string", "description": "Session identifier (UUID or sprint-scoped)"},
			"agent_id":      map[string]string{"type": "string", "description": "Agent surface (cursor-parent, claude-code, codex)"},
			"sprint_id":     map[string]string{"type": "string", "description": "Associated sprint ID (optional)"},
			"summary":       map[string]string{"type": "string", "description": "Session summary (what was accomplished)"},
			"carry_forward": map[string]string{"type": "string", "description": "Items for next session to pick up"},
			"blockers":      map[string]string{"type": "string", "description": "Current blockers requiring attention"},
			"commits":       map[string]string{"type": "string", "description": "Commit SHAs and subjects from this session"},
			"repos_pushed":  map[string]string{"type": "string", "description": "Repos pushed during this session"},
		},
		"required": []string{"id", "session_id", "agent_id", "summary"},
	}
}

func sessionHandoffLatestSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"agent_id": map[string]string{"type": "string", "description": "Filter by agent surface (optional)"},
			"limit":    map[string]string{"type": "integer", "description": "Number of handoffs to return (default 3, max 20)"},
		},
	}
}

func sessionHandoffSearchSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query":      map[string]string{"type": "string", "description": "Search query (matched against summary, carry_forward, blockers)"},
			"since_date": map[string]string{"type": "string", "description": "ISO date filter (e.g. 2026-05-24) — only return handoffs after this date"},
		},
		"required": []string{"query"},
	}
}

func (s *Server) sessionHandoffStore(args json.RawMessage) (string, bool) {
	var p struct {
		ID           string `json:"id"`
		SessionID    string `json:"session_id"`
		AgentID      string `json:"agent_id"`
		SprintID     string `json:"sprint_id"`
		Summary      string `json:"summary"`
		CarryForward string `json:"carry_forward"`
		Blockers     string `json:"blockers"`
		Commits      string `json:"commits"`
		ReposPushed  string `json:"repos_pushed"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return fmt.Sprintf("invalid args: %v", err), true
	}

	h := sprintboard.SessionHandoff{
		ID:           p.ID,
		SessionID:    p.SessionID,
		AgentID:      p.AgentID,
		SprintID:     p.SprintID,
		Summary:      p.Summary,
		CarryForward: p.CarryForward,
		Blockers:     p.Blockers,
		Commits:      p.Commits,
		ReposPushed:  p.ReposPushed,
	}

	if err := s.store.StoreSessionHandoff(h); err != nil {
		return fmt.Sprintf("store handoff: %v", err), true
	}

	out, _ := json.Marshal(map[string]string{
		"status": "stored",
		"id":     p.ID,
	})
	return string(out), false
}

func (s *Server) sessionHandoffLatest(args json.RawMessage) (string, bool) {
	var p struct {
		AgentID string `json:"agent_id"`
		Limit   int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return fmt.Sprintf("invalid args: %v", err), true
	}

	handoffs, err := s.store.LatestSessionHandoffs(p.AgentID, p.Limit)
	if err != nil {
		return fmt.Sprintf("query handoffs: %v", err), true
	}

	out, _ := json.Marshal(handoffs)
	return string(out), false
}

func (s *Server) sessionHandoffSearch(args json.RawMessage) (string, bool) {
	var p struct {
		Query     string `json:"query"`
		SinceDate string `json:"since_date"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return fmt.Sprintf("invalid args: %v", err), true
	}

	handoffs, err := s.store.SearchSessionHandoffs(p.Query, p.SinceDate)
	if err != nil {
		return fmt.Sprintf("search handoffs: %v", err), true
	}

	out, _ := json.Marshal(handoffs)
	return string(out), false
}
