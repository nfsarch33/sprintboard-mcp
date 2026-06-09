package sprintboard_test

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

func TestInsertFleetPROutcome_SQLite(t *testing.T) {
	dir := t.TempDir()
	store, err := sprintboard.NewStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	id, err := store.InsertFleetPROutcome(sprintboard.FleetPROutcome{
		Host:          "wsl2",
		Repo:          "nfsarch33/helixon-ec",
		PRNumber:      159,
		Outcome:       "merged",
		Verdict:       "pass",
		ReviewerAgent: "fleet-pr-reviewer",
		MergeSHA:      "abc123",
		RecordedAt:    time.Date(2026, 6, 9, 14, 0, 0, 0, time.UTC),
		Payload:       map[string]any{"checks": "green"},
	})
	if err != nil {
		t.Fatalf("InsertFleetPROutcome: %v", err)
	}
	got, err := store.GetFleetPROutcome(id)
	if err != nil {
		t.Fatalf("GetFleetPROutcome: %v", err)
	}
	if got.Outcome != "merged" || got.PRNumber != 159 {
		t.Fatalf("got %+v", got)
	}
}

func TestListFleetStatsHistory_SQLite(t *testing.T) {
	dir := t.TempDir()
	store, err := sprintboard.NewStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	now := time.Now().UTC()
	_, err = store.InsertFleetReportSnapshot(sprintboard.FleetReportSnapshot{
		Host:        "wsl1",
		ReportKind:  "status",
		WindowStart: now.Add(-time.Hour),
		WindowEnd:   now,
		Payload:     map[string]any{"ok": true},
	})
	if err != nil {
		t.Fatalf("InsertFleetReportSnapshot: %v", err)
	}
	_, err = store.InsertFleetPROutcome(sprintboard.FleetPROutcome{
		Host:     "wsl2",
		Repo:     "nfsarch33/sprintboard-mcp",
		PRNumber: 8,
		Outcome:  "merged",
		Payload:  map[string]any{"verdict": "pass"},
	})
	if err != nil {
		t.Fatalf("InsertFleetPROutcome: %v", err)
	}

	all, err := store.ListFleetStatsHistory(sprintboard.FleetHistoryAll, "", 7)
	if err != nil {
		t.Fatalf("ListFleetStatsHistory all: %v", err)
	}
	if len(all) < 2 {
		t.Fatalf("expected >=2 items, got %d", len(all))
	}

	prOnly, err := store.ListFleetStatsHistory(sprintboard.FleetHistoryPROutcome, "wsl2", 7)
	if err != nil {
		t.Fatalf("ListFleetStatsHistory pr_outcome: %v", err)
	}
	if len(prOnly) != 1 {
		t.Fatalf("expected 1 pr outcome, got %d", len(prOnly))
	}
	if prOnly[0].Kind != sprintboard.FleetHistoryPROutcome {
		t.Fatalf("kind = %s", prOnly[0].Kind)
	}
	var summary map[string]any
	if err := json.Unmarshal(prOnly[0].Payload, &summary); err == nil {
		if summary["verdict"] != "pass" {
			t.Fatalf("payload = %#v", summary)
		}
	}
}

func TestListFleetStatsHistory_UnknownKind(t *testing.T) {
	dir := t.TempDir()
	store, err := sprintboard.NewStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	_, err = store.ListFleetStatsHistory("bogus", "", 7)
	if err == nil {
		t.Fatal("expected error for unknown kind")
	}
}
