package sprintboard_test

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

func TestInsertTerminalSessionEvent_SQLite(t *testing.T) {
	dir := t.TempDir()
	store, err := sprintboard.NewStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	code := 0
	id, err := store.InsertTerminalSessionEvent(sprintboard.TerminalSessionEvent{
		Host:         "wsl1",
		SessionID:    "sess-1",
		CommandClass: "git",
		ExitCode:     &code,
		DurationMs:   1200,
		Status:       "completed",
		Payload:      map[string]any{"command_hash": "abc123"},
	})
	if err != nil {
		t.Fatalf("InsertTerminalSessionEvent: %v", err)
	}
	got, err := store.GetTerminalSessionEvent(id)
	if err != nil {
		t.Fatalf("GetTerminalSessionEvent: %v", err)
	}
	if got.Host != "wsl1" || got.CommandClass != "git" {
		t.Fatalf("got %+v", got)
	}
}

func TestInsertEvalRunSnapshot_SQLite(t *testing.T) {
	dir := t.TempDir()
	store, err := sprintboard.NewStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	id, err := store.InsertEvalRunSnapshot(sprintboard.EvalRunSnapshot{
		Host:       "wsl1",
		EvalRunID:  "eval-2026-06-09",
		Suite:      "fleet-run",
		Model:      "router-judge",
		Score:      108,
		PassCount:  108,
		FailCount:  42,
		DurationMs: 900000,
		RecordedAt: time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC),
		Payload:    map[string]any{"judge": true},
	})
	if err != nil {
		t.Fatalf("InsertEvalRunSnapshot: %v", err)
	}
	got, err := store.GetEvalRunSnapshot(id)
	if err != nil {
		t.Fatalf("GetEvalRunSnapshot: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(got.PayloadJSON, &payload); err != nil {
		t.Fatalf("payload decode: %v", err)
	}
	if payload["judge"] != true {
		t.Fatalf("payload = %#v", payload)
	}
}
