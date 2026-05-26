package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestE2ESprintGoalSetGet(t *testing.T) {
	bin := buildBinary(t)
	client := startMCP(t, bin)
	client.call(t, "initialize", map[string]interface{}{})

	client.callTool(t, "sprint_create", map[string]string{
		"id": "v17300", "name": "Progressive Disclosure", "theme": "token-saving",
	})

	result := client.callTool(t, "sprint_goal_set", map[string]string{
		"sprint_id": "v17300", "goal": "Token-efficient agent startup",
	})
	if !strings.Contains(result, "Goal set") {
		t.Fatalf("unexpected goal set response: %s", result)
	}

	goalJSON := client.callTool(t, "sprint_goal_get", map[string]string{
		"sprint_id": "v17300",
	})
	var goalResp map[string]string
	if err := json.Unmarshal([]byte(goalJSON), &goalResp); err != nil {
		t.Fatalf("unmarshal goal: %v\nraw: %s", err, goalJSON)
	}
	if goalResp["goal"] != "Token-efficient agent startup" {
		t.Errorf("goal = %q, want %q", goalResp["goal"], "Token-efficient agent startup")
	}
}

func TestE2EContextSummaryDepth1(t *testing.T) {
	bin := buildBinary(t)
	client := startMCP(t, bin)
	client.call(t, "initialize", map[string]interface{}{})

	client.callTool(t, "roadmap_create", map[string]string{
		"id": "rm-cs", "name": "Test Roadmap",
	})

	result := client.callTool(t, "context_summary", map[string]interface{}{"depth": 1})
	if !strings.Contains(result, "rm-cs") {
		t.Errorf("depth=1 should contain roadmap ID: %s", result)
	}
	if !strings.Contains(result, "Test Roadmap") {
		t.Errorf("depth=1 should contain roadmap name: %s", result)
	}
}

func TestE2EContextSummaryDepth3(t *testing.T) {
	bin := buildBinary(t)
	client := startMCP(t, bin)
	client.call(t, "initialize", map[string]interface{}{})

	result := client.callTool(t, "context_summary", map[string]interface{}{"depth": 3})
	if !strings.Contains(result, "roadmaps") {
		t.Errorf("depth=3 should contain roadmaps key: %s", result)
	}
	if !strings.Contains(result, "tickets") {
		t.Errorf("depth=3 should contain tickets key: %s", result)
	}
}

func TestE2EContextDetail(t *testing.T) {
	bin := buildBinary(t)
	client := startMCP(t, bin)
	client.call(t, "initialize", map[string]interface{}{})

	client.callTool(t, "roadmap_create", map[string]string{
		"id": "rm-cd", "name": "Detail Roadmap",
	})
	client.callTool(t, "programme_create", map[string]string{
		"id": "pg-cd", "roadmap_id": "rm-cd", "name": "Child Programme",
	})

	result := client.callTool(t, "context_detail", map[string]string{
		"entity_id": "rm-cd",
	})
	if !strings.Contains(result, `"entity_type": "roadmap"`) {
		t.Errorf("expected entity_type=roadmap: %s", result)
	}
	if !strings.Contains(result, "pg-cd") {
		t.Errorf("expected child programme in result: %s", result)
	}
}

func TestE2EContextDetailNotFound(t *testing.T) {
	bin := buildBinary(t)
	client := startMCP(t, bin)
	client.call(t, "initialize", map[string]interface{}{})

	argsJSON, _ := json.Marshal(map[string]string{"entity_id": "nonexistent"})
	result := client.call(t, "tools/call", map[string]interface{}{
		"name":      "context_detail",
		"arguments": json.RawMessage(argsJSON),
	})
	var tr ToolResult
	json.Unmarshal(result, &tr)
	if !tr.IsError {
		t.Error("expected error for nonexistent entity")
	}
}

func TestE2ESessionHandoffArchive(t *testing.T) {
	bin := buildBinary(t)
	client := startMCP(t, bin)
	client.call(t, "initialize", map[string]interface{}{})

	client.callTool(t, "session_handoff_store", map[string]string{
		"id": "h-arch-1", "session_id": "s1", "agent_id": "cursor-parent",
		"summary": "test handoff for archive",
	})

	result := client.callTool(t, "session_handoff_archive", map[string]string{
		"id": "h-arch-1",
	})
	if !strings.Contains(result, "archived") {
		t.Errorf("expected archived confirmation: %s", result)
	}

	latestJSON := client.callTool(t, "session_handoff_latest", map[string]interface{}{
		"limit": 10,
	})
	if strings.Contains(latestJSON, "h-arch-1") {
		t.Errorf("archived handoff should not appear by default: %s", latestJSON)
	}

	latestAllJSON := client.callTool(t, "session_handoff_latest", map[string]interface{}{
		"limit": 10, "include_archived": true,
	})
	if !strings.Contains(latestAllJSON, "h-arch-1") {
		t.Errorf("archived handoff should appear with include_archived: %s", latestAllJSON)
	}
}

func TestE2EStartupContext(t *testing.T) {
	bin := buildBinary(t)
	client := startMCP(t, bin)
	client.call(t, "initialize", map[string]interface{}{})

	client.callTool(t, "roadmap_create", map[string]string{
		"id": "rm-sc", "name": "Startup Roadmap",
	})
	client.callTool(t, "session_handoff_store", map[string]string{
		"id": "h-sc-1", "session_id": "s1", "agent_id": "cursor-parent",
		"summary": "previous work done",
	})

	result := client.callTool(t, "startup_context", map[string]string{})
	if !strings.Contains(result, "previous work done") {
		t.Errorf("startup_context should contain handoff: %s", result)
	}
	if !strings.Contains(result, "context_summary") {
		t.Errorf("startup_context should contain context_summary: %s", result)
	}
	if !strings.Contains(result, "rm-sc") {
		t.Errorf("startup_context should include roadmap from context_summary: %s", result)
	}
}
