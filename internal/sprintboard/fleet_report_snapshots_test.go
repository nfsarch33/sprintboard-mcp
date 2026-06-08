package sprintboard_test

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

func TestInsertFleetReportSnapshot_SQLite(t *testing.T) {
	dir := t.TempDir()
	store, err := sprintboard.NewStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	payload := map[string]any{"tokens": 42, "report_kind": "status"}
	start := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)
	end := start.Add(12 * time.Hour)

	id, err := store.InsertFleetReportSnapshot(sprintboard.FleetReportSnapshot{
		Host:        "wsl1",
		ReportKind:  "status",
		WindowStart: start,
		WindowEnd:   end,
		Payload:     payload,
	})
	if err != nil {
		t.Fatalf("InsertFleetReportSnapshot: %v", err)
	}
	if id <= 0 {
		t.Fatalf("id = %d, want positive", id)
	}

	got, err := store.GetFleetReportSnapshot(id)
	if err != nil {
		t.Fatalf("GetFleetReportSnapshot: %v", err)
	}
	if got.Host != "wsl1" || got.ReportKind != "status" {
		t.Fatalf("got %+v", got)
	}
	if !got.WindowStart.Equal(start) || !got.WindowEnd.Equal(end) {
		t.Fatalf("window mismatch: %+v", got)
	}
	var decoded map[string]any
	if err := json.Unmarshal(got.PayloadJSON, &decoded); err != nil {
		t.Fatalf("payload decode: %v", err)
	}
	if decoded["tokens"] != float64(42) {
		t.Fatalf("payload tokens = %v", decoded["tokens"])
	}
}

func TestInsertFleetReportSnapshot_Validation(t *testing.T) {
	dir := t.TempDir()
	store, err := sprintboard.NewStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	_, err = store.InsertFleetReportSnapshot(sprintboard.FleetReportSnapshot{
		ReportKind: "daily",
	})
	if err == nil {
		t.Fatal("expected error for missing host")
	}
}
