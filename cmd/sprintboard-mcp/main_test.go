package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nfsarch33/sprintboard-mcp/internal/mcptelemetry"
	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

// newTestServer constructs a Server backed by an on-disk SQLite store and a
// disabled telemetry recorder. Callers get a ready-to-dispatch Server that
// shares no state with other tests (each test owns a temp dir).
func newTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	store, err := sprintboard.NewStore(filepath.Join(dir, "sprintboard.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	rec, err := mcptelemetry.New(mcptelemetry.Config{Enabled: false})
	if err != nil {
		t.Fatalf("telemetry.New: %v", err)
	}

	emb := sprintboard.NewEmbedder(sprintboard.EmbedderConfig{Dimension: 8})
	return &Server{
		store:     store,
		agentID:   "test-agent",
		telemetry: rec,
		embedder:  emb,
	}
}

func mustArgs(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	return b
}

func mustOK(t *testing.T, msg string, isErr bool, where string) {
	t.Helper()
	if isErr {
		t.Fatalf("%s: dispatch returned error: %s", where, msg)
	}
}

// TestDispatch_MiniJiraFlow drives the full mini-jira evolution surface
// (sprint -> tickets -> agent register -> dependencies -> ready list ->
// topo sort -> claim -> recommend -> distribute -> complete) through the
// real Server.dispatch entry point. Hits 0%-coverage handlers in main.go
// without spinning the JSON-RPC stdio loop.
func TestDispatch_MiniJiraFlow(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)

	// 1. sprint_create
	out, isErr := s.dispatch("sprint_create", mustArgs(t, map[string]string{
		"id":    "v9000",
		"name":  "Mini-Jira Evolution",
		"theme": "dependencies + capabilities + ready-list",
	}))
	mustOK(t, out, isErr, "sprint_create")
	if !strings.Contains(out, "v9000") {
		t.Fatalf("sprint_create result missing id: %q", out)
	}

	// 2. sprint_list and sprint_status
	if out, isErr := s.dispatch("sprint_list", json.RawMessage(`{}`)); isErr || !strings.Contains(out, "v9000") {
		t.Fatalf("sprint_list = %q (err=%v)", out, isErr)
	}
	if out, isErr := s.dispatch("sprint_status", mustArgs(t, map[string]string{"sprint_id": "v9000"})); isErr || !strings.Contains(out, "v9000") {
		t.Fatalf("sprint_status = %q (err=%v)", out, isErr)
	}

	// 3. ticket_create x3 (T-A, T-B, T-C with T-B depends on T-A, T-C on T-B)
	for _, id := range []string{"T-A", "T-B", "T-C"} {
		out, isErr := s.dispatch("ticket_create", mustArgs(t, map[string]any{
			"id":          id,
			"sprint_id":   "v9000",
			"title":       "Ticket " + id,
			"description": "desc " + id,
			"priority":    1,
		}))
		mustOK(t, out, isErr, "ticket_create "+id)
	}

	// 4. agent_register (capability matching)
	out, isErr = s.dispatch("agent_register", mustArgs(t, map[string]string{
		"agent_id":     "alpha",
		"surface":      "test",
		"capabilities": "go,sqlite",
	}))
	mustOK(t, out, isErr, "agent_register alpha")
	if _, isErr := s.dispatch("agent_register", mustArgs(t, map[string]string{
		"agent_id": "beta", "surface": "test", "capabilities": "go",
	})); isErr {
		t.Fatal("agent_register beta failed")
	}

	// 5. agent_heartbeat + agent_list
	if out, isErr := s.dispatch("agent_heartbeat", mustArgs(t, map[string]string{"agent_id": "alpha"})); isErr {
		t.Fatalf("agent_heartbeat: %s", out)
	}
	if out, isErr := s.dispatch("agent_list", json.RawMessage(`{}`)); isErr || !strings.Contains(out, "alpha") {
		t.Fatalf("agent_list = %q", out)
	}

	// 6. ticket_depend_add: T-B depends on T-A, T-C depends on T-B
	if _, isErr := s.dispatch("ticket_depend_add", mustArgs(t, map[string]string{
		"ticket_id": "T-B", "depends_on": "T-A",
	})); isErr {
		t.Fatal("ticket_depend_add T-B->T-A failed")
	}
	if _, isErr := s.dispatch("ticket_depend_add", mustArgs(t, map[string]string{
		"ticket_id": "T-C", "depends_on": "T-B",
	})); isErr {
		t.Fatal("ticket_depend_add T-C->T-B failed")
	}

	// 7. ticket_blocked_by: T-B should be blocked by [T-A]
	out, isErr = s.dispatch("ticket_blocked_by", mustArgs(t, map[string]string{"ticket_id": "T-B"}))
	mustOK(t, out, isErr, "ticket_blocked_by")
	if !strings.Contains(out, "T-A") {
		t.Fatalf("blocked_by(T-B) = %q, want T-A", out)
	}

	// 8. ticket_ready_list: only T-A is unblocked
	out, isErr = s.dispatch("ticket_ready_list", mustArgs(t, map[string]string{"sprint_id": "v9000"}))
	mustOK(t, out, isErr, "ticket_ready_list")
	if !strings.Contains(out, "T-A") || strings.Contains(out, "T-C") {
		t.Fatalf("ready_list unexpected: %q", out)
	}

	// 9. sprint_topo_sort: must order T-A < T-B < T-C
	out, isErr = s.dispatch("sprint_topo_sort", mustArgs(t, map[string]string{"sprint_id": "v9000"}))
	mustOK(t, out, isErr, "sprint_topo_sort")
	posA, posB, posC := strings.Index(out, "T-A"), strings.Index(out, "T-B"), strings.Index(out, "T-C")
	if posA == -1 || posB == -1 || posC == -1 || !(posA < posB && posB < posC) {
		t.Fatalf("topo_sort wrong order: %q (positions A=%d B=%d C=%d)", out, posA, posB, posC)
	}

	// 10. task_claim (T-A by alpha) and task_complete with evidence
	if out, isErr := s.dispatch("task_claim", mustArgs(t, map[string]string{
		"ticket_id": "T-A", "agent_id": "alpha",
	})); isErr {
		t.Fatalf("task_claim: %s", out)
	}
	if out, isErr := s.dispatch("task_complete", mustArgs(t, map[string]string{
		"ticket_id": "T-A", "agent_id": "alpha", "evidence": "tests green",
	})); isErr {
		t.Fatalf("task_complete: %s", out)
	}

	// 11. ticket_depend_remove: drop T-C->T-B so T-B and T-C show ready (post T-A done)
	if _, isErr := s.dispatch("ticket_depend_remove", mustArgs(t, map[string]string{
		"ticket_id": "T-C", "depends_on": "T-B",
	})); isErr {
		t.Fatal("ticket_depend_remove failed")
	}

	// 12. task_recommend for alpha (sprint scoped)
	out, isErr = s.dispatch("task_recommend", mustArgs(t, map[string]any{
		"agent_id": "alpha", "sprint_id": "v9000", "limit": 5,
	}))
	mustOK(t, out, isErr, "task_recommend")

	// 13. sprint_distribute (round-robin across alpha + beta over remaining tickets)
	out, isErr = s.dispatch("sprint_distribute", mustArgs(t, map[string]string{"sprint_id": "v9000"}))
	mustOK(t, out, isErr, "sprint_distribute")
	if !strings.Contains(out, `"assigned"`) {
		t.Fatalf("sprint_distribute missing assigned key: %q", out)
	}
}

// TestDispatch_UnknownTool exercises the default switch arm.
func TestDispatch_UnknownTool(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)
	out, isErr := s.dispatch("does_not_exist", json.RawMessage(`{}`))
	if !isErr || !strings.Contains(out, "unknown tool") {
		t.Fatalf("unknown tool: out=%q isErr=%v", out, isErr)
	}
}

// TestDispatch_ErrorPaths covers invalid JSON and missing required fields
// to lift the negative-branch coverage of dispatchInner.
func TestDispatch_ErrorPaths(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)

	// Invalid JSON should never panic and should be classified as an error.
	if _, isErr := s.dispatch("ticket_create", json.RawMessage(`{`)); !isErr {
		t.Fatal("expected error on invalid ticket_create json")
	}

	// Missing id/ticket_id is rejected.
	if out, isErr := s.dispatch("ticket_create", mustArgs(t, map[string]string{
		"sprint_id": "v9000", "title": "x",
	})); !isErr || !strings.Contains(out, "id or ticket_id") {
		t.Fatalf("ticket_create without id: out=%q isErr=%v", out, isErr)
	}

	// sprint_distribute requires sprint_id.
	if out, isErr := s.dispatch("sprint_distribute", json.RawMessage(`{}`)); !isErr || !strings.Contains(out, "sprint_id is required") {
		t.Fatalf("sprint_distribute without id: out=%q isErr=%v", out, isErr)
	}

	// task_recommend with unknown agent surfaces the GetAgent error.
	if out, isErr := s.dispatch("task_recommend", mustArgs(t, map[string]string{
		"agent_id": "nobody",
	})); !isErr || !strings.Contains(out, "not registered") {
		t.Fatalf("task_recommend unknown agent: out=%q isErr=%v", out, isErr)
	}
}

// TestServe_InitializeAndToolsList round-trips two JSON-RPC messages through
// serve() to lock the handshake contract: initialize then tools/list.
func TestServe_InitializeAndToolsList(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)

	in := bytes.NewBufferString("" +
		`{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n",
	)
	var out bytes.Buffer
	s.serve(in, &out)

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 response lines, got %d: %q", len(lines), out.String())
	}

	var initResp JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[0]), &initResp); err != nil {
		t.Fatalf("initialize decode: %v", err)
	}
	if initResp.ID == nil {
		t.Fatal("initialize missing id")
	}

	var listResp JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[1]), &listResp); err != nil {
		t.Fatalf("tools/list decode: %v", err)
	}
	resultMap, ok := listResp.Result.(map[string]any)
	if !ok {
		t.Fatalf("tools/list result not a map: %T", listResp.Result)
	}
	tools, ok := resultMap["tools"].([]any)
	if !ok || len(tools) == 0 {
		t.Fatalf("tools/list returned no tools: %v", resultMap)
	}
	// Spot-check that mini-jira evolution tools are registered.
	want := map[string]bool{
		"ticket_depend_add":    false,
		"ticket_depend_remove": false,
		"ticket_blocked_by":    false,
		"ticket_ready_list":    false,
		"sprint_topo_sort":     false,
		"task_recommend":       false,
		"sprint_distribute":    false,
	}
	for _, raw := range tools {
		td, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if name, ok := td["name"].(string); ok {
			if _, tracked := want[name]; tracked {
				want[name] = true
			}
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("tool %q missing from tools/list response", name)
		}
	}
}

// TestServe_IgnoresMalformedAndNotifications locks two transport contracts:
// (a) blank lines and unparseable JSON are silently skipped (no response),
// (b) requests without an id (notifications) produce no response.
func TestServe_IgnoresMalformedAndNotifications(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)

	in := bytes.NewBufferString("\n" +
		`{not json}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
		`{"jsonrpc":"2.0","id":7,"method":"initialize"}` + "\n",
	)
	var out bytes.Buffer
	s.serve(in, &out)

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected exactly 1 response (the initialize), got %d: %q", len(lines), out.String())
	}
	if !strings.Contains(lines[0], `"id":7`) {
		t.Fatalf("unexpected response line: %q", lines[0])
	}
}
