package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestE2E_v8900_ToolsRoundTrip exercises the v8900-B19 MCP tools end-to-end:
// build the binary, register a sprint+ticket, and call each new tool over the
// stdio JSON-RPC surface. This pins the dispatch wiring so a future refactor
// of dispatchInner can't silently drop one of the new tools.
func TestE2E_v8900_ToolsRoundTrip(t *testing.T) {
	bin := buildBinary(t)
	client := startMCP(t, bin)
	client.call(t, "initialize", map[string]interface{}{})

	client.callTool(t, "sprint_create", map[string]string{"id": "v8900-sp", "name": "v8900 Sprint", "theme": "platform"})
	client.callTool(t, "ticket_create", map[string]interface{}{
		"id":          "v8900-tk1",
		"sprint_id":   "v8900-sp",
		"title":       "Wire LLM provider",
		"description": "platform server provider seam",
		"priority":    9,
	})

	out := client.callTool(t, "ticket_search_filter", map[string]interface{}{
		"q":            "platform",
		"priority_min": 5,
	})
	if !strings.Contains(out, `"v8900-tk1"`) {
		t.Fatalf("ticket_search_filter missed seeded ticket: %s", out)
	}

	hist := client.callTool(t, "sprint_history", map[string]interface{}{})
	if !strings.Contains(hist, `"v8900-sp"`) {
		t.Fatalf("sprint_history missed seeded sprint: %s", hist)
	}

	metrics := client.callTool(t, "sprint_metrics", map[string]string{"sprint_id": "v8900-sp"})
	var got map[string]json.RawMessage
	if err := json.Unmarshal([]byte(metrics), &got); err != nil {
		t.Fatalf("sprint_metrics decode: %v\nraw: %s", err, metrics)
	}
	for _, key := range []string{"sprint", "tickets_by_status", "total_tickets", "slas", "velocity", "burndown"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("metrics missing key %q: %s", key, metrics)
		}
	}
}
