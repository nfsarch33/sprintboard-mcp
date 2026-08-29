package main

import (
	"strings"
	"testing"
)

// TestDispatch_V18789HandlerCoverage covers the MCP tool handlers still at 0%
// after v17400: the v2 hierarchy tools (roadmap/programme/epic/time), the
// v17300 progressive-disclosure tools, the session-handoff tools, the v8900
// query tools, and ticket_resolve.
//
// Each sub-test drives one tool through s.dispatch, so a failure names the
// tool rather than a line in a shared helper. Bootstrap is done once because
// these tools are hierarchical: an epic needs a programme, a programme needs a
// roadmap, and time reporting needs a ticket that exists.
func TestDispatch_V18789HandlerCoverage(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)

	mustOK := func(t *testing.T, tool string, args any) string {
		t.Helper()
		out, isErr := s.dispatch(tool, mustArgs(t, args))
		if isErr {
			t.Fatalf("%s returned an error result: %s", tool, out)
		}
		return out
	}

	// ---- bootstrap: sprint, ticket, agent ----
	mustOK(t, "sprint_create", map[string]string{
		"id": "v18789", "name": "Handler Coverage", "theme": "coverage",
	})
	mustOK(t, "ticket_create", map[string]any{
		"id": "COV-1", "sprint_id": "v18789", "title": "coverage ticket", "priority": 2,
	})
	mustOK(t, "agent_register", map[string]string{
		"agent_id": "cov-agent", "surface": "test", "capabilities": "go",
	})

	// ---- v2 hierarchy: roadmap -> programme -> epic ----
	t.Run("roadmap", func(t *testing.T) {
		mustOK(t, "roadmap_create", map[string]string{
			"id": "RM-1", "name": "2026 Roadmap", "description": "annual plan",
		})
		if out := mustOK(t, "roadmap_list", map[string]string{}); !strings.Contains(out, "RM-1") {
			t.Errorf("roadmap_list omits RM-1: %s", out)
		}
		if out := mustOK(t, "roadmap_view", map[string]string{"roadmap_id": "RM-1"}); !strings.Contains(out, "RM-1") {
			t.Errorf("roadmap_view omits RM-1: %s", out)
		}
	})

	t.Run("programme", func(t *testing.T) {
		mustOK(t, "programme_create", map[string]string{
			"id": "PR-1", "roadmap_id": "RM-1", "name": "Platform", "description": "programme",
		})
		if out := mustOK(t, "programme_list", map[string]string{}); !strings.Contains(out, "PR-1") {
			t.Errorf("programme_list omits PR-1: %s", out)
		}
		mustOK(t, "programme_view", map[string]string{"programme_id": "PR-1"})
	})

	t.Run("epic", func(t *testing.T) {
		mustOK(t, "epic_create", map[string]string{
			"id": "EP-1", "programme_id": "PR-1", "name": "Board", "description": "epic",
		})
		if out := mustOK(t, "epic_list", map[string]string{}); !strings.Contains(out, "EP-1") {
			t.Errorf("epic_list omits EP-1: %s", out)
		}
		mustOK(t, "epic_view", map[string]string{"epic_id": "EP-1"})
		mustOK(t, "epic_burndown", map[string]string{"epic_id": "EP-1"})
	})

	// ---- time tracking ----
	t.Run("time_tracking", func(t *testing.T) {
		mustOK(t, "ticket_estimate", map[string]any{"ticket_id": "COV-1", "minutes": 120})
		mustOK(t, "ticket_log_time", map[string]any{"ticket_id": "COV-1", "minutes": 45})
		if out := mustOK(t, "sprint_time_report", map[string]string{"sprint_id": "v18789"}); out == "" {
			t.Error("sprint_time_report returned empty output")
		}
	})

	// ---- trees and summaries ----
	t.Run("trees_and_summaries", func(t *testing.T) {
		mustOK(t, "ticket_tree", map[string]string{"sprint_id": "v18789"})
		mustOK(t, "session_summary", map[string]string{"sprint_id": "v18789"})
	})

	// ---- v17300 progressive disclosure ----
	t.Run("progressive_disclosure", func(t *testing.T) {
		mustOK(t, "sprint_goal_set", map[string]string{
			"sprint_id": "v18789", "goal": "reach the coverage floor",
		})
		if out := mustOK(t, "sprint_goal_get", map[string]string{"sprint_id": "v18789"}); !strings.Contains(out, "coverage floor") {
			t.Errorf("sprint_goal_get lost the goal text: %s", out)
		}
		mustOK(t, "context_summary", map[string]string{"sprint_id": "v18789"})
		// context_detail resolves an entity by id across tables, not a sprint.
		mustOK(t, "context_detail", map[string]string{"entity_id": "COV-1"})
		mustOK(t, "startup_context", map[string]string{})
	})

	// ---- session handoffs ----
	t.Run("session_handoffs", func(t *testing.T) {
		mustOK(t, "session_handoff_store", map[string]string{
			"id":         "SH-1",
			"session_id": "sess-1",
			"agent_id":   "cov-agent",
			"sprint_id":  "v18789",
			"summary":    "handoff summary for coverage",
		})
		if out := mustOK(t, "session_handoff_latest", map[string]string{"agent_id": "cov-agent"}); !strings.Contains(out, "SH-1") {
			t.Errorf("session_handoff_latest omits SH-1: %s", out)
		}
		mustOK(t, "session_handoff_search", map[string]string{"query": "coverage"})
		// archive takes the handoff id stored above.
		mustOK(t, "session_handoff_archive", map[string]string{"id": "SH-1"})
	})

	// ---- v8900 query tools ----
	t.Run("v8900_queries", func(t *testing.T) {
		if out := mustOK(t, "ticket_search_filter", map[string]any{
			"sprint_id": "v18789", "status": "backlog",
		}); out == "" {
			t.Error("ticket_search_filter returned empty output")
		}
		mustOK(t, "sprint_history", map[string]any{"limit": 10})
		mustOK(t, "sprint_metrics", map[string]string{"sprint_id": "v18789"})
	})
}

// TestDispatch_TicketResolve covers the MCP surface of the human-resolution
// route. The HTTP route is covered in internal/api; this asserts the tool
// wired to it enforces the same contract -- notably that `reason` is required,
// because a resolution with no reason leaves the agent's escalation comment
// without an answer.
func TestDispatch_TicketResolve(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)

	if _, isErr := s.dispatch("sprint_create", mustArgs(t, map[string]string{
		"id": "v18789-r", "name": "Resolve",
	})); isErr {
		t.Fatal("sprint_create failed")
	}
	if _, isErr := s.dispatch("ticket_create", mustArgs(t, map[string]any{
		"id": "RES-1", "sprint_id": "v18789-r", "title": "escalated ticket",
	})); isErr {
		t.Fatal("ticket_create failed")
	}
	if _, isErr := s.dispatch("task_claim", mustArgs(t, map[string]string{
		"ticket_id": "RES-1", "agent_id": "fleet-agent-01",
	})); isErr {
		t.Fatal("task_claim failed")
	}

	// A non-claimer resolves it -- the whole point of the route.
	out, isErr := s.dispatch("ticket_resolve", mustArgs(t, map[string]string{
		"id": "RES-1", "actor": "operator", "reason": "poison ticket; no action needed",
	}))
	if isErr {
		t.Fatalf("ticket_resolve as a non-claimer failed: %s", out)
	}
	if !strings.Contains(out, "operator") {
		t.Errorf("ticket_resolve output does not name the actor: %s", out)
	}

	// The ticket must now read as resolved, not done.
	got, isErr := s.dispatch("ticket_list", mustArgs(t, map[string]string{"sprint_id": "v18789-r"}))
	if isErr {
		t.Fatalf("ticket_list failed: %s", got)
	}
	if !strings.Contains(got, "resolved_by_human") {
		t.Errorf("ticket is not resolved_by_human after ticket_resolve: %s", got)
	}

	// reason is mandatory: an empty one must be refused, not silently accepted.
	if out, isErr := s.dispatch("ticket_resolve", mustArgs(t, map[string]string{
		"id": "RES-1", "actor": "operator", "reason": "",
	})); !isErr {
		t.Errorf("ticket_resolve accepted an empty reason: %s", out)
	}

	// Already terminal: a second resolve must be refused so the first
	// resolver's name and reason stand.
	if out, isErr := s.dispatch("ticket_resolve", mustArgs(t, map[string]string{
		"id": "RES-1", "actor": "someone-else", "reason": "second attempt",
	})); !isErr {
		t.Errorf("ticket_resolve overwrote an already-resolved ticket: %s", out)
	}
}

// TestDispatch_UnknownToolAndBadArgs covers handleToolsCall's error paths,
// which no existing test reached.
func TestDispatch_UnknownToolAndBadArgs(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)

	if out, isErr := s.dispatch("no_such_tool_exists", mustArgs(t, map[string]string{})); !isErr {
		t.Errorf("unknown tool did not return an error result: %s", out)
	}

	// Malformed JSON for a tool that requires parameters.
	if out, isErr := s.dispatch("sprint_create", []byte(`{"id":`)); !isErr {
		t.Errorf("malformed arguments did not return an error result: %s", out)
	}
}
