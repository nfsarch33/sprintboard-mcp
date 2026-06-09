package main

import (
	"encoding/json"

	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

func fleetReportHistorySchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"kind": map[string]string{
				"type":        "string",
				"description": "fleet_report, eval_run, terminal_session, pr_outcome, or all (default)",
			},
			"host": map[string]string{"type": "string", "description": "Optional host filter (wsl1, wsl2, etc.)"},
			"days": map[string]string{"type": "integer", "description": "Lookback window in days (default 7, max 90)"},
		},
	}
}

func (s *Server) fleetReportHistory(args json.RawMessage) (string, bool) {
	var p struct {
		Kind string `json:"kind"`
		Host string `json:"host"`
		Days int    `json:"days"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &p); err != nil {
			return err.Error(), true
		}
	}
	kind := sprintboard.FleetHistoryKind(p.Kind)
	if kind == "" {
		kind = sprintboard.FleetHistoryAll
	}
	days := p.Days
	if days <= 0 {
		days = 7
	}
	items, err := s.store.ListFleetStatsHistory(kind, p.Host, days)
	if err != nil {
		return err.Error(), true
	}
	if items == nil {
		items = []sprintboard.FleetHistoryItem{}
	}
	out, err := json.Marshal(map[string]any{
		"kind":  kind,
		"host":  p.Host,
		"days":  days,
		"count": len(items),
		"items": items,
	})
	if err != nil {
		return err.Error(), true
	}
	return string(out), false
}
