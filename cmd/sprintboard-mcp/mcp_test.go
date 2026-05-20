package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func buildBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "sprintboard-mcp")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = filepath.Join(findModRoot(t), "cmd", "sprintboard-mcp")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return bin
}

func findModRoot(t *testing.T) string {
	t.Helper()
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find go.mod")
		}
		dir = parent
	}
}

type mcpClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Scanner
	nextID int
}

func startMCP(t *testing.T, bin string) *mcpClient {
	t.Helper()

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(),
		"HOME="+dbDir,
		fmt.Sprintf("XDG_CONFIG_HOME=%s", filepath.Join(dbDir, ".config")),
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}

	_ = dbPath
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		stdin.Close()
		cmd.Wait()
	})

	return &mcpClient{cmd: cmd, stdin: stdin, reader: bufio.NewScanner(stdout), nextID: 1}
}

func (c *mcpClient) call(t *testing.T, method string, params interface{}) json.RawMessage {
	t.Helper()

	id := c.nextID
	c.nextID++

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}

	data, _ := json.Marshal(req)
	_, err := fmt.Fprintf(c.stdin, "%s\n", data)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	if !c.reader.Scan() {
		t.Fatal("no response from server")
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal(c.reader.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nraw: %s", err, c.reader.Text())
	}

	if resp.Error != nil {
		t.Fatalf("RPC error: %d %s", resp.Error.Code, resp.Error.Message)
	}

	raw, _ := json.Marshal(resp.Result)
	return raw
}

func (c *mcpClient) callTool(t *testing.T, name string, args interface{}) string {
	t.Helper()
	argsJSON, _ := json.Marshal(args)
	result := c.call(t, "tools/call", map[string]interface{}{
		"name":      name,
		"arguments": json.RawMessage(argsJSON),
	})

	var tr ToolResult
	json.Unmarshal(result, &tr)
	if tr.IsError {
		t.Fatalf("tool %q returned error: %s", name, tr.Content[0].Text)
	}
	if len(tr.Content) == 0 {
		return ""
	}
	return tr.Content[0].Text
}

func TestE2EInitialize(t *testing.T) {
	bin := buildBinary(t)
	client := startMCP(t, bin)

	result := client.call(t, "initialize", map[string]interface{}{})
	var initResult map[string]interface{}
	json.Unmarshal(result, &initResult)

	if initResult["protocolVersion"] != "2024-11-05" {
		t.Errorf("unexpected protocol version: %v", initResult["protocolVersion"])
	}
}

func TestE2EToolsList(t *testing.T) {
	bin := buildBinary(t)
	client := startMCP(t, bin)

	client.call(t, "initialize", map[string]interface{}{})
	result := client.call(t, "tools/list", map[string]interface{}{})

	var toolsResult map[string]interface{}
	json.Unmarshal(result, &toolsResult)

	tools, ok := toolsResult["tools"].([]interface{})
	if !ok {
		t.Fatal("expected tools array")
	}
	// 23 base tools + 5 DAG tools (ticket_depend_add/remove, ticket_blocked_by, ticket_ready_list, sprint_topo_sort)
	if len(tools) != 28 {
		t.Errorf("expected 28 tools, got %d", len(tools))
	}
}

func TestE2EFullWorkflow(t *testing.T) {
	bin := buildBinary(t)
	client := startMCP(t, bin)

	client.call(t, "initialize", map[string]interface{}{})

	client.callTool(t, "sprint_create", map[string]string{"id": "e2e-sprint", "name": "E2E Test Sprint", "theme": "testing"})
	client.callTool(t, "ticket_create", map[string]interface{}{"id": "e2e-t1", "sprint_id": "e2e-sprint", "title": "First ticket", "priority": 5})
	client.callTool(t, "ticket_create", map[string]interface{}{"id": "e2e-t2", "sprint_id": "e2e-sprint", "title": "Second ticket", "priority": 3})

	client.callTool(t, "ticket_assign", map[string]string{"id": "e2e-t1", "agent": "codex-ec"})
	client.callTool(t, "ticket_update", map[string]string{"id": "e2e-t1", "status": "in_progress", "note": "codex starting"})
	client.callTool(t, "ticket_update", map[string]string{"id": "e2e-t1", "status": "done", "note": "codex completed"})

	client.callTool(t, "handoff_create", map[string]string{"ticket_id": "e2e-t2", "to_agent": "claude-code", "context_path": "session-handoffs/test.md"})

	statusJSON := client.callTool(t, "sprint_status", map[string]string{"sprint_id": "e2e-sprint"})

	var summary struct {
		TotalTickets    int            `json:"total_tickets"`
		TicketsByStatus map[string]int `json:"tickets_by_status"`
	}
	json.Unmarshal([]byte(statusJSON), &summary)

	if summary.TotalTickets != 2 {
		t.Errorf("expected 2 total tickets, got %d", summary.TotalTickets)
	}
	if summary.TicketsByStatus["done"] != 1 {
		t.Errorf("expected 1 done, got %d", summary.TicketsByStatus["done"])
	}

	client.callTool(t, "sprint_close", map[string]string{"sprint_id": "e2e-sprint"})

	sprintsJSON := client.callTool(t, "sprint_list", map[string]string{})
	if sprintsJSON == "null" || sprintsJSON == "[]" {
		t.Error("expected at least one sprint in list")
	}
}

func TestE2EConcurrentAgents(t *testing.T) {
	bin := buildBinary(t)

	client := startMCP(t, bin)
	client.call(t, "initialize", map[string]interface{}{})
	client.callTool(t, "sprint_create", map[string]string{"id": "shared-sprint", "name": "Shared Work"})

	for i := 0; i < 5; i++ {
		client.callTool(t, "ticket_create", map[string]interface{}{
			"id":        fmt.Sprintf("concurrent-%d", i),
			"sprint_id": "shared-sprint",
			"title":     fmt.Sprintf("Concurrent task %d", i),
		})
	}

	statusJSON := client.callTool(t, "sprint_status", map[string]string{"sprint_id": "shared-sprint"})
	var summary struct {
		TotalTickets int `json:"total_tickets"`
	}
	json.Unmarshal([]byte(statusJSON), &summary)

	if summary.TotalTickets != 5 {
		t.Errorf("expected 5 concurrent tickets, got %d", summary.TotalTickets)
	}
}

func TestE2EHandoffWorkflow(t *testing.T) {
	bin := buildBinary(t)
	client := startMCP(t, bin)
	client.call(t, "initialize", map[string]interface{}{})

	client.callTool(t, "sprint_create", map[string]string{"id": "handoff-sprint", "name": "Handoff Test"})
	client.callTool(t, "ticket_create", map[string]interface{}{"id": "h-t1", "sprint_id": "handoff-sprint", "title": "Handoff ticket"})
	client.callTool(t, "ticket_update", map[string]string{"id": "h-t1", "status": "ready_for_handoff", "note": "ready for next agent"})
	client.callTool(t, "handoff_create", map[string]string{"ticket_id": "h-t1", "to_agent": "codex", "context_path": "session-handoffs/2026-05-18-test.md"})

	handoffsJSON := client.callTool(t, "handoff_list", map[string]string{"ticket_id": "h-t1"})
	var handoffs []map[string]interface{}
	json.Unmarshal([]byte(handoffsJSON), &handoffs)

	if len(handoffs) != 1 {
		t.Errorf("expected 1 handoff, got %d", len(handoffs))
	}
}

func TestE2EErrorCases(t *testing.T) {
	bin := buildBinary(t)
	client := startMCP(t, bin)
	client.call(t, "initialize", map[string]interface{}{})

	argsJSON, _ := json.Marshal(map[string]string{"sprint_id": "nonexistent"})
	result := client.call(t, "tools/call", map[string]interface{}{
		"name":      "sprint_status",
		"arguments": json.RawMessage(argsJSON),
	})

	var tr ToolResult
	json.Unmarshal(result, &tr)
	if !tr.IsError {
		t.Error("expected error for nonexistent sprint")
	}
}
