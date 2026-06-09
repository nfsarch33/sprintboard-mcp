package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

func TestFleetReportHistoryTool(t *testing.T) {
	dir := t.TempDir()
	store, err := sprintboard.NewStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	_, err = store.InsertFleetPROutcome(sprintboard.FleetPROutcome{
		Host:     "wsl2",
		Repo:     "nfsarch33/sprintboard-mcp",
		PRNumber: 9,
		Outcome:  "reviewed",
		Payload:  map[string]any{"verdict": "pass"},
	})
	if err != nil {
		t.Fatalf("InsertFleetPROutcome: %v", err)
	}

	srv := &Server{store: store}
	args, _ := json.Marshal(map[string]any{"kind": "pr_outcome", "host": "wsl2", "days": 7})
	out, isErr := srv.fleetReportHistory(args)
	if isErr {
		t.Fatalf("fleetReportHistory error: %s", out)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if parsed["count"].(float64) < 1 {
		t.Fatalf("count = %v", parsed["count"])
	}
}
