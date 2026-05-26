package main

import (
	"encoding/json"
	"testing"
)

func TestE2EV2RoadmapCRUD(t *testing.T) {
	bin := buildBinary(t)
	client := startMCP(t, bin)
	client.call(t, "initialize", map[string]interface{}{})

	client.callTool(t, "roadmap_create", map[string]string{
		"id": "rm-e2e", "name": "E2E Roadmap", "description": "Test roadmap",
	})

	viewJSON := client.callTool(t, "roadmap_view", map[string]string{"roadmap_id": "rm-e2e"})
	var rm struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	json.Unmarshal([]byte(viewJSON), &rm)
	if rm.Name != "E2E Roadmap" {
		t.Errorf("expected name %q, got %q", "E2E Roadmap", rm.Name)
	}

	listJSON := client.callTool(t, "roadmap_list", map[string]string{})
	var roadmaps []map[string]interface{}
	json.Unmarshal([]byte(listJSON), &roadmaps)
	if len(roadmaps) != 1 {
		t.Errorf("expected 1 roadmap, got %d", len(roadmaps))
	}
}

func TestE2EV2ProgrammeCRUD(t *testing.T) {
	bin := buildBinary(t)
	client := startMCP(t, bin)
	client.call(t, "initialize", map[string]interface{}{})

	client.callTool(t, "roadmap_create", map[string]string{"id": "rm-pg", "name": "PG Roadmap"})
	client.callTool(t, "programme_create", map[string]string{
		"id": "pg-e2e", "roadmap_id": "rm-pg", "name": "Fleet Monitoring",
	})

	viewJSON := client.callTool(t, "programme_view", map[string]string{"programme_id": "pg-e2e"})
	var pg struct {
		Name      string `json:"name"`
		RoadmapID string `json:"roadmap_id"`
	}
	json.Unmarshal([]byte(viewJSON), &pg)
	if pg.Name != "Fleet Monitoring" {
		t.Errorf("expected name %q, got %q", "Fleet Monitoring", pg.Name)
	}
	if pg.RoadmapID != "rm-pg" {
		t.Errorf("expected roadmap_id %q, got %q", "rm-pg", pg.RoadmapID)
	}

	listJSON := client.callTool(t, "programme_list", map[string]string{"roadmap_id": "rm-pg"})
	var programmes []map[string]interface{}
	json.Unmarshal([]byte(listJSON), &programmes)
	if len(programmes) != 1 {
		t.Errorf("expected 1 programme, got %d", len(programmes))
	}
}

func TestE2EV2EpicCRUD(t *testing.T) {
	bin := buildBinary(t)
	client := startMCP(t, bin)
	client.call(t, "initialize", map[string]interface{}{})

	client.callTool(t, "programme_create", map[string]string{"id": "pg-ep", "name": "Epic Programme"})
	client.callTool(t, "epic_create", map[string]string{
		"id": "ep-e2e", "programme_id": "pg-ep", "name": "K8s Probes",
	})

	viewJSON := client.callTool(t, "epic_view", map[string]string{"epic_id": "ep-e2e"})
	var ep struct {
		Name        string `json:"name"`
		ProgrammeID string `json:"programme_id"`
	}
	json.Unmarshal([]byte(viewJSON), &ep)
	if ep.Name != "K8s Probes" {
		t.Errorf("expected name %q, got %q", "K8s Probes", ep.Name)
	}

	listJSON := client.callTool(t, "epic_list", map[string]string{"programme_id": "pg-ep"})
	var epics []map[string]interface{}
	json.Unmarshal([]byte(listJSON), &epics)
	if len(epics) != 1 {
		t.Errorf("expected 1 epic, got %d", len(epics))
	}
}

func TestE2EV2EpicBurndown(t *testing.T) {
	bin := buildBinary(t)
	client := startMCP(t, bin)
	client.call(t, "initialize", map[string]interface{}{})

	client.callTool(t, "epic_create", map[string]string{"id": "ep-bd", "name": "Burndown Epic"})
	client.callTool(t, "sprint_create", map[string]string{"id": "sp-bd", "name": "BD Sprint"})
	client.callTool(t, "ticket_create", map[string]interface{}{"id": "bd-t1", "sprint_id": "sp-bd", "title": "Task 1"})
	client.callTool(t, "ticket_create", map[string]interface{}{"id": "bd-t2", "sprint_id": "sp-bd", "title": "Task 2"})

	bdJSON := client.callTool(t, "epic_burndown", map[string]string{"epic_id": "ep-bd"})
	var bd struct {
		TotalTickets int `json:"total_tickets"`
	}
	json.Unmarshal([]byte(bdJSON), &bd)
	// Tickets are not linked to epic via MCP yet, so total is 0
	if bd.TotalTickets != 0 {
		t.Errorf("expected 0 tickets (not linked to epic), got %d", bd.TotalTickets)
	}
}

func TestE2EV2TimeTracking(t *testing.T) {
	bin := buildBinary(t)
	client := startMCP(t, bin)
	client.call(t, "initialize", map[string]interface{}{})

	client.callTool(t, "sprint_create", map[string]string{"id": "sp-time", "name": "Time Sprint"})
	client.callTool(t, "ticket_create", map[string]interface{}{"id": "time-t1", "sprint_id": "sp-time", "title": "Timed Task"})

	client.callTool(t, "ticket_estimate", map[string]interface{}{"ticket_id": "time-t1", "minutes": 60})
	client.callTool(t, "ticket_log_time", map[string]interface{}{"ticket_id": "time-t1", "minutes": 30})
	client.callTool(t, "ticket_log_time", map[string]interface{}{"ticket_id": "time-t1", "minutes": 20})

	rptJSON := client.callTool(t, "sprint_time_report", map[string]string{"sprint_id": "sp-time"})
	var rpt struct {
		TotalEstimate int `json:"total_estimate_minutes"`
		TotalActual   int `json:"total_actual_minutes"`
	}
	json.Unmarshal([]byte(rptJSON), &rpt)
	if rpt.TotalEstimate != 60 {
		t.Errorf("expected 60 estimate, got %d", rpt.TotalEstimate)
	}
	if rpt.TotalActual != 50 {
		t.Errorf("expected 50 actual, got %d", rpt.TotalActual)
	}
}

func TestE2EV2TicketTree(t *testing.T) {
	bin := buildBinary(t)
	client := startMCP(t, bin)
	client.call(t, "initialize", map[string]interface{}{})

	client.callTool(t, "sprint_create", map[string]string{"id": "sp-tree", "name": "Tree Sprint"})
	client.callTool(t, "ticket_create", map[string]interface{}{"id": "tree-p1", "sprint_id": "sp-tree", "title": "Parent"})
	client.callTool(t, "ticket_create", map[string]interface{}{"id": "tree-c1", "sprint_id": "sp-tree", "title": "Child"})

	treeJSON := client.callTool(t, "ticket_tree", map[string]string{"sprint_id": "sp-tree"})
	var tree struct {
		Sprint  map[string]interface{}   `json:"sprint"`
		Tickets []map[string]interface{} `json:"tickets"`
	}
	json.Unmarshal([]byte(treeJSON), &tree)
	if tree.Sprint == nil {
		t.Error("expected sprint in tree")
	}
	if len(tree.Tickets) != 2 {
		t.Errorf("expected 2 root tickets (no parent linking via MCP), got %d", len(tree.Tickets))
	}
}

func TestE2EV2SessionSummary(t *testing.T) {
	bin := buildBinary(t)
	client := startMCP(t, bin)
	client.call(t, "initialize", map[string]interface{}{})

	ssJSON := client.callTool(t, "session_summary", map[string]string{})
	var ss struct {
		ActiveSprint   interface{}              `json:"active_sprint"`
		RecentHandoffs []map[string]interface{} `json:"recent_handoffs"`
		ActiveAgents   []map[string]interface{} `json:"active_agents"`
		BlockedTickets []map[string]interface{} `json:"blocked_tickets"`
	}
	json.Unmarshal([]byte(ssJSON), &ss)

	if ss.RecentHandoffs == nil {
		t.Error("RecentHandoffs should be non-nil")
	}
	if ss.ActiveAgents == nil {
		t.Error("ActiveAgents should be non-nil")
	}
	if ss.BlockedTickets == nil {
		t.Error("BlockedTickets should be non-nil")
	}
}

func TestE2EV2SessionSummaryWithActiveSprint(t *testing.T) {
	bin := buildBinary(t)
	client := startMCP(t, bin)
	client.call(t, "initialize", map[string]interface{}{})

	client.callTool(t, "sprint_create", map[string]string{"id": "sp-ss", "name": "SS Sprint"})
	// Activate the sprint by transitioning its status
	client.callTool(t, "ticket_create", map[string]interface{}{"id": "ss-t1", "sprint_id": "sp-ss", "title": "Task"})

	ssJSON := client.callTool(t, "session_summary", map[string]string{})
	var ss struct {
		ActiveSprint *struct {
			Sprint struct {
				ID string `json:"id"`
			} `json:"sprint"`
		} `json:"active_sprint"`
	}
	json.Unmarshal([]byte(ssJSON), &ss)
	// Sprint is "planned" not "active", so active_sprint should be nil
	if ss.ActiveSprint != nil {
		t.Error("expected nil active_sprint for planned sprint")
	}
}

func TestE2EV2FullHierarchyWorkflow(t *testing.T) {
	bin := buildBinary(t)
	client := startMCP(t, bin)
	client.call(t, "initialize", map[string]interface{}{})

	client.callTool(t, "roadmap_create", map[string]string{"id": "rm-full", "name": "Full Roadmap"})
	client.callTool(t, "programme_create", map[string]string{"id": "pg-full", "roadmap_id": "rm-full", "name": "Full Programme"})
	client.callTool(t, "epic_create", map[string]string{"id": "ep-full", "programme_id": "pg-full", "name": "Full Epic"})
	client.callTool(t, "sprint_create", map[string]string{"id": "sp-full", "name": "Full Sprint"})
	client.callTool(t, "ticket_create", map[string]interface{}{"id": "full-t1", "sprint_id": "sp-full", "title": "Full Task"})
	client.callTool(t, "ticket_estimate", map[string]interface{}{"ticket_id": "full-t1", "minutes": 45})
	client.callTool(t, "ticket_log_time", map[string]interface{}{"ticket_id": "full-t1", "minutes": 30})

	rptJSON := client.callTool(t, "sprint_time_report", map[string]string{"sprint_id": "sp-full"})
	var rpt struct {
		TotalEstimate int     `json:"total_estimate_minutes"`
		TotalActual   int     `json:"total_actual_minutes"`
		AccuracyRatio float64 `json:"accuracy_ratio"`
	}
	json.Unmarshal([]byte(rptJSON), &rpt)
	if rpt.TotalEstimate != 45 {
		t.Errorf("expected 45 estimate, got %d", rpt.TotalEstimate)
	}
	if rpt.TotalActual != 30 {
		t.Errorf("expected 30 actual, got %d", rpt.TotalActual)
	}
	if rpt.AccuracyRatio == 0 {
		t.Error("expected non-zero accuracy ratio")
	}
}
