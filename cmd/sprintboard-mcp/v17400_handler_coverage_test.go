package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestDispatch_V17400HandlerCoverage exercises the cmd/sprintboard-mcp
// handlers that were left at 0% coverage by the existing dispatch tests.
// Each sub-test is built on top of the shared newTestServer fixture and
// drives a single tool through s.dispatch so the coverage uplift is
// targeted and incremental.
//
// Coverage targets (post v17400 PG migration):
//   - ticketList, ticketUpdate, ticketAssign
//   - handoffCreate, handoffList (ticket + agent paths)
//   - handoffPublish, handoffSubscribe
//   - ticketSearch, sprintSearch
//   - sprintKickoffPrompt, sprintHandoffTemplate
//   - ticketCommentAdd, ticketCommentList
func TestDispatch_V17400HandlerCoverage(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)

	// ---- bootstrap sprint + tickets ----
	if _, isErr := s.dispatch("sprint_create", mustArgs(t, map[string]string{
		"id":    "v17400",
		"name":  "PG Migration Coverage",
		"theme": "handler coverage gates",
	})); isErr {
		t.Fatalf("sprint_create bootstrap failed")
	}

	for _, id := range []string{"TKT-1", "TKT-2"} {
		if _, isErr := s.dispatch("ticket_create", mustArgs(t, map[string]any{
			"id":          id,
			"sprint_id":   "v17400",
			"title":       "Coverage " + id,
			"description": "ticket for handler coverage tests",
			"priority":    1,
		})); isErr {
			t.Fatalf("ticket_create %s failed", id)
		}
	}
	for _, id := range []string{"recv", "publisher"} {
		if _, isErr := s.dispatch("agent_register", mustArgs(t, map[string]string{
			"agent_id":     id,
			"surface":      "test",
			"capabilities": "go",
		})); isErr {
			t.Fatalf("agent_register %s failed", id)
		}
	}

	// ---- ticket_list (no filter, status filter, owner filter) ----
	t.Run("ticket_list_paths", func(t *testing.T) {
		out, isErr := s.dispatch("ticket_list", mustArgs(t, map[string]string{"sprint_id": "v17400"}))
		mustOK(t, out, isErr, "ticket_list (sprint)")
		if !strings.Contains(out, "TKT-1") || !strings.Contains(out, "TKT-2") {
			t.Fatalf("ticket_list missing tickets: %q", out)
		}

		if out, isErr := s.dispatch("ticket_list", mustArgs(t, map[string]string{
			"sprint_id": "v17400", "status": "ready",
		})); isErr {
			t.Fatalf("ticket_list status filter: %s", out)
		}
		if out, isErr := s.dispatch("ticket_list", mustArgs(t, map[string]string{
			"sprint_id": "v17400", "owner": "nobody",
		})); isErr {
			t.Fatalf("ticket_list owner filter: %s", out)
		}
	})

	// ---- ticket_update ----
	t.Run("ticket_update", func(t *testing.T) {
		out, isErr := s.dispatch("ticket_update", mustArgs(t, map[string]string{
			"id": "TKT-1", "status": "in_progress", "note": "starting work",
		}))
		mustOK(t, out, isErr, "ticket_update")
		if !strings.Contains(out, "TKT-1") {
			t.Fatalf("ticket_update result missing id: %q", out)
		}

		if _, isErr := s.dispatch("ticket_update", json.RawMessage(`{not-json`)); !isErr {
			t.Fatal("ticket_update should reject malformed JSON")
		}
	})

	// ---- ticket_assign ----
	t.Run("ticket_assign", func(t *testing.T) {
		out, isErr := s.dispatch("ticket_assign", mustArgs(t, map[string]string{
			"id": "TKT-2", "agent": "recv",
		}))
		mustOK(t, out, isErr, "ticket_assign")
		if !strings.Contains(out, "recv") {
			t.Fatalf("ticket_assign result missing agent: %q", out)
		}

		if _, isErr := s.dispatch("ticket_assign", json.RawMessage(`{`)); !isErr {
			t.Fatal("ticket_assign should reject malformed JSON")
		}
	})

	// ---- handoff_create ----
	t.Run("handoff_create", func(t *testing.T) {
		out, isErr := s.dispatch("handoff_create", mustArgs(t, map[string]string{
			"ticket_id":    "TKT-1",
			"to_agent":     "recv",
			"context_path": "/tmp/handoff-context.md",
		}))
		mustOK(t, out, isErr, "handoff_create")
		if !strings.Contains(out, "TKT-1") || !strings.Contains(out, "recv") {
			t.Fatalf("handoff_create result missing fields: %q", out)
		}

		if _, isErr := s.dispatch("handoff_create", json.RawMessage(`{`)); !isErr {
			t.Fatal("handoff_create should reject malformed JSON")
		}
	})

	// ---- handoff_list (ticket scope and agent scope) ----
	t.Run("handoff_list_paths", func(t *testing.T) {
		if out, isErr := s.dispatch("handoff_list", mustArgs(t, map[string]string{
			"ticket_id": "TKT-1",
		})); isErr {
			t.Fatalf("handoff_list ticket: %s", out)
		}
		if out, isErr := s.dispatch("handoff_list", mustArgs(t, map[string]string{
			"agent_id": "recv",
		})); isErr {
			t.Fatalf("handoff_list agent: %s", out)
		}

		if _, isErr := s.dispatch("handoff_list", json.RawMessage(`{`)); !isErr {
			t.Fatal("handoff_list should reject malformed JSON")
		}
	})

	// ---- handoff_publish + handoff_subscribe ----
	t.Run("handoff_publish_subscribe", func(t *testing.T) {
		out, isErr := s.dispatch("handoff_publish", mustArgs(t, map[string]string{
			"ticket_id": "TKT-1",
			"to_agent":  "recv",
			"summary":   "PG migration complete; await deploy",
			"branch":    "feat/v17400-pg-migration",
		}))
		mustOK(t, out, isErr, "handoff_publish")
		if !strings.Contains(out, "recv") {
			t.Fatalf("handoff_publish missing target: %q", out)
		}

		// default since (24h ago)
		if out, isErr := s.dispatch("handoff_subscribe", mustArgs(t, map[string]string{
			"agent_id": "recv",
		})); isErr {
			t.Fatalf("handoff_subscribe default: %s", out)
		}
		// explicit RFC3339 since
		if out, isErr := s.dispatch("handoff_subscribe", mustArgs(t, map[string]string{
			"agent_id": "recv", "since": "2020-01-01T00:00:00Z",
		})); isErr {
			t.Fatalf("handoff_subscribe explicit since: %s", out)
		}
		// invalid since string falls through to default 24h window
		if out, isErr := s.dispatch("handoff_subscribe", mustArgs(t, map[string]string{
			"agent_id": "recv", "since": "not-a-date",
		})); isErr {
			t.Fatalf("handoff_subscribe invalid since: %s", out)
		}
		// blank agent_id auto-fills from server
		if out, isErr := s.dispatch("handoff_subscribe", json.RawMessage(`{}`)); isErr {
			t.Fatalf("handoff_subscribe blank agent: %s", out)
		}

		if _, isErr := s.dispatch("handoff_publish", json.RawMessage(`{`)); !isErr {
			t.Fatal("handoff_publish should reject malformed JSON")
		}
		if _, isErr := s.dispatch("handoff_subscribe", json.RawMessage(`{`)); !isErr {
			t.Fatal("handoff_subscribe should reject malformed JSON")
		}
	})

	// ---- ticket_search + sprint_search ----
	t.Run("search_paths", func(t *testing.T) {
		// ticket_search default limit
		if _, isErr := s.dispatch("ticket_search", mustArgs(t, map[string]any{
			"query": "coverage",
		})); isErr {
			t.Fatal("ticket_search default limit failed")
		}
		// ticket_search explicit limit
		if _, isErr := s.dispatch("ticket_search", mustArgs(t, map[string]any{
			"query": "coverage", "limit": 3,
		})); isErr {
			t.Fatal("ticket_search explicit limit failed")
		}
		// sprint_search default + explicit limit
		if _, isErr := s.dispatch("sprint_search", mustArgs(t, map[string]any{
			"query": "PG Migration",
		})); isErr {
			t.Fatal("sprint_search default failed")
		}
		if _, isErr := s.dispatch("sprint_search", mustArgs(t, map[string]any{
			"query": "PG Migration", "limit": 2,
		})); isErr {
			t.Fatal("sprint_search explicit limit failed")
		}

		if _, isErr := s.dispatch("ticket_search", json.RawMessage(`{`)); !isErr {
			t.Fatal("ticket_search should reject malformed JSON")
		}
		if _, isErr := s.dispatch("sprint_search", json.RawMessage(`{`)); !isErr {
			t.Fatal("sprint_search should reject malformed JSON")
		}
	})

	// ---- sprint_kickoff_prompt ----
	t.Run("sprint_kickoff_prompt", func(t *testing.T) {
		out, isErr := s.dispatch("sprint_kickoff_prompt", mustArgs(t, map[string]string{
			"sprint_id": "v17400", "agent_id": "recv",
		}))
		mustOK(t, out, isErr, "sprint_kickoff_prompt")
		if !strings.Contains(out, "v17400") || !strings.Contains(out, "Race Prevention") {
			t.Fatalf("kickoff prompt missing expected sections: %q", out)
		}

		if out, isErr := s.dispatch("sprint_kickoff_prompt", mustArgs(t, map[string]string{
			"sprint_id": "missing-sprint",
		})); !isErr || !strings.Contains(out, "not found") {
			t.Fatalf("kickoff missing sprint should be error: out=%q isErr=%v", out, isErr)
		}
		if _, isErr := s.dispatch("sprint_kickoff_prompt", json.RawMessage(`{`)); !isErr {
			t.Fatal("sprint_kickoff_prompt should reject malformed JSON")
		}
	})

	// ---- sprint_handoff_template ----
	t.Run("sprint_handoff_template", func(t *testing.T) {
		out, isErr := s.dispatch("sprint_handoff_template", mustArgs(t, map[string]string{
			"sprint_id": "v17400",
		}))
		mustOK(t, out, isErr, "sprint_handoff_template")
		if !strings.Contains(out, "v17400") || !strings.Contains(out, "Operator Actions Required") {
			t.Fatalf("handoff template missing sections: %q", out)
		}

		// explicit agent_id branch
		if _, isErr := s.dispatch("sprint_handoff_template", mustArgs(t, map[string]string{
			"sprint_id": "v17400", "agent_id": "publisher",
		})); isErr {
			t.Fatal("sprint_handoff_template explicit agent failed")
		}
		// missing sprint -> error
		if out, isErr := s.dispatch("sprint_handoff_template", mustArgs(t, map[string]string{
			"sprint_id": "no-such",
		})); !isErr || !strings.Contains(out, "not found") {
			t.Fatalf("template missing sprint should be error: out=%q isErr=%v", out, isErr)
		}
		if _, isErr := s.dispatch("sprint_handoff_template", json.RawMessage(`{`)); !isErr {
			t.Fatal("sprint_handoff_template should reject malformed JSON")
		}
	})

	// ---- ticket_comment_add + ticket_comment_list ----
	t.Run("ticket_comment_paths", func(t *testing.T) {
		out, isErr := s.dispatch("ticket_comment_add", mustArgs(t, map[string]string{
			"ticket_id": "TKT-1", "author": "recv", "body": "PG smoke test green",
		}))
		mustOK(t, out, isErr, "ticket_comment_add")
		if !strings.Contains(out, "PG smoke test") {
			t.Fatalf("comment_add result missing body: %q", out)
		}
		// blank author -> auto-fills server agent ID
		if _, isErr := s.dispatch("ticket_comment_add", mustArgs(t, map[string]string{
			"ticket_id": "TKT-1", "body": "auto-author",
		})); isErr {
			t.Fatal("ticket_comment_add auto-author failed")
		}

		out, isErr = s.dispatch("ticket_comment_list", mustArgs(t, map[string]string{
			"ticket_id": "TKT-1",
		}))
		mustOK(t, out, isErr, "ticket_comment_list")
		if !strings.Contains(out, "PG smoke test") {
			t.Fatalf("comment_list missing earlier comment: %q", out)
		}
		// list a ticket with no comments returns []
		if out, isErr := s.dispatch("ticket_comment_list", mustArgs(t, map[string]string{
			"ticket_id": "TKT-2",
		})); isErr || !strings.Contains(out, "[]") {
			t.Fatalf("comment_list empty: out=%q isErr=%v", out, isErr)
		}

		if _, isErr := s.dispatch("ticket_comment_add", json.RawMessage(`{`)); !isErr {
			t.Fatal("ticket_comment_add should reject malformed JSON")
		}
		if _, isErr := s.dispatch("ticket_comment_list", json.RawMessage(`{`)); !isErr {
			t.Fatal("ticket_comment_list should reject malformed JSON")
		}
	})
}
