package mcptelemetry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRecordEnabled(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.ndjson")

	r, err := New(Config{Enabled: true, LogPath: logPath})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	r.Record("sprint_create", "cursor-parent", 42*time.Millisecond, false, "")
	r.Record("ticket_update", "codex", 150*time.Millisecond, true, "not found")

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	var event1 ToolEvent
	json.Unmarshal([]byte(lines[0]), &event1)
	if event1.Tool != "sprint_create" {
		t.Errorf("expected sprint_create, got %q", event1.Tool)
	}
	if event1.AgentID != "cursor-parent" {
		t.Errorf("expected cursor-parent, got %q", event1.AgentID)
	}
	if event1.DurationMS != 42 {
		t.Errorf("expected 42ms, got %d", event1.DurationMS)
	}
	if !event1.Success {
		t.Error("expected success=true")
	}

	var event2 ToolEvent
	json.Unmarshal([]byte(lines[1]), &event2)
	if event2.Success {
		t.Error("expected success=false")
	}
	if event2.Error != "not found" {
		t.Errorf("expected error 'not found', got %q", event2.Error)
	}
}

func TestRecordDisabled(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "disabled.ndjson")

	r, err := New(Config{Enabled: false, LogPath: logPath})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	r.Record("sprint_create", "agent", 10*time.Millisecond, false, "")

	_, err = os.Stat(logPath)
	if err == nil {
		t.Error("log file should not be created when disabled")
	}
}

func TestConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "concurrent.ndjson")

	r, err := New(Config{Enabled: true, LogPath: logPath})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			r.Record("tool", "agent", time.Duration(idx)*time.Millisecond, false, "")
		}(i)
	}
	wg.Wait()

	data, _ := os.ReadFile(logPath)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 100 {
		t.Errorf("expected 100 lines, got %d", len(lines))
	}

	for i, line := range lines {
		var event ToolEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Errorf("line %d: invalid JSON: %v", i, err)
		}
	}
}

func TestEventSchema(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "schema.ndjson")

	r, err := New(Config{Enabled: true, LogPath: logPath})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	r.Record("test_tool", "test-agent", 500*time.Millisecond, false, "")

	data, _ := os.ReadFile(logPath)
	var event map[string]interface{}
	json.Unmarshal(data, &event)

	requiredFields := []string{"ts", "tool", "agent_id", "duration_ms", "success"}
	for _, field := range requiredFields {
		if _, ok := event[field]; !ok {
			t.Errorf("missing required field %q", field)
		}
	}

	if _, ok := event["error"]; ok {
		t.Error("error field should be omitted on success")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Enabled {
		t.Error("default should be enabled")
	}
	if cfg.LogPath == "" {
		t.Error("default log path should not be empty")
	}
	if cfg.IncludeArgs {
		t.Error("default should not include args")
	}
}

func TestEnabledMethod(t *testing.T) {
	r1, _ := New(Config{Enabled: true, LogPath: filepath.Join(t.TempDir(), "x.ndjson")})
	defer r1.Close()
	if !r1.Enabled() {
		t.Error("should report enabled")
	}

	r2, _ := New(Config{Enabled: false})
	defer r2.Close()
	if r2.Enabled() {
		t.Error("should report disabled")
	}
}

func TestEnvBool(t *testing.T) {
	tests := []struct {
		val  string
		want bool
	}{
		{"true", true},
		{"1", true},
		{"yes", true},
		{"false", false},
		{"0", false},
		{"no", false},
		{"", true}, // default=true for this test
	}
	for _, tc := range tests {
		t.Setenv("TEST_BOOL", tc.val)
		got := envBool("TEST_BOOL", true)
		if tc.val == "" {
			os.Unsetenv("TEST_BOOL")
			got = envBool("TEST_BOOL", true)
		}
		if got != tc.want {
			t.Errorf("envBool(%q) = %v, want %v", tc.val, got, tc.want)
		}
	}
}
