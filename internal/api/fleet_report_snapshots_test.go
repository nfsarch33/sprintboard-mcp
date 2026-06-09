package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestFleetReportSnapshotCreate(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	body := `{
		"host": "DESKTOP-078M990",
		"report_kind": "status",
		"window_start": "2026-06-09T00:00:00Z",
		"window_end": "2026-06-09T12:00:00Z",
		"payload": {"agentrace_total_tokens": 0, "minimax_tokens": 0}
	}`
	resp := postJSON(t, ts.URL+"/api/v1/fleet-reports/snapshots", body)
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
	if _, ok := out["id"]; !ok {
		t.Fatal("missing id in response")
	}
}

func TestFleetReportSnapshotCreate_Validation(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/api/v1/fleet-reports/snapshots", `{"report_kind":"daily"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}
