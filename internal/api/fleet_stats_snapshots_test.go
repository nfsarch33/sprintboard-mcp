package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestTerminalSessionEventCreate(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	body := `{
		"host": "wsl1",
		"session_id": "sess-1",
		"command_class": "git",
		"exit_code": 0,
		"duration_ms": 1200,
		"status": "completed",
		"payload": {"command_hash":"abc","output_bytes":64}
	}`
	resp := postJSON(t, ts.URL+"/api/v1/terminal-sessions/events", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["status"] != "created" {
		t.Fatalf("status field = %v", out["status"])
	}
}

func TestEvalRunSnapshotCreate(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	body := `{
		"host": "wsl1",
		"eval_run_id": "eval-2026-06-09",
		"suite": "fleet-run",
		"score": 108,
		"pass_count": 108,
		"fail_count": 42,
		"duration_ms": 900000,
		"recorded_at": "2026-06-09T12:00:00Z",
		"payload": {"judge": true}
	}`
	resp := postJSON(t, ts.URL+"/api/v1/eval-runs/snapshots", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
}
