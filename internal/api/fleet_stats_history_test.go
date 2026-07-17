package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestFleetStatsHistory_Empty(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/fleet-stats/history?kind=fleet_report&days=7")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["count"].(float64) != 0 {
		t.Fatalf("count = %v", out["count"])
	}
}

func TestFleetPROutcomeCreateAndHistory(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	// Use recorded_at = now so the record falls inside the 7-day
	// window queried by the history endpoint. Earlier versions
	// hard-coded a date that drifted out of the window as time
	// passed, causing a stale-data FAIL.
	body := fmt.Sprintf(`{
		"host": "wsl2",
		"repo": "nfsarch33/helixon-ec",
		"pr_number": 184,
		"outcome": "merged",
		"verdict": "pass",
		"reviewer_agent": "fleet-pr-reviewer",
		"merge_sha": "deadbeef",
		"recorded_at": %q,
		"payload": {"checks_url": "https://example/ci"}
	}`, time.Now().UTC().Format(time.RFC3339))
	resp := postJSON(t, ts.URL+"/api/v1/fleet-pr-outcomes", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST status = %d, want 201", resp.StatusCode)
	}

	hist, err := http.Get(ts.URL + "/api/v1/fleet-stats/history?kind=pr_outcome&host=wsl2&days=7")
	if err != nil {
		t.Fatalf("GET history: %v", err)
	}
	defer hist.Body.Close()
	if hist.StatusCode != http.StatusOK {
		t.Fatalf("history status = %d", hist.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(hist.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["count"].(float64) < 1 {
		t.Fatalf("expected history rows, got %v", out["count"])
	}
}

func TestFleetPROutcomeCreate_Validation(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/api/v1/fleet-pr-outcomes", `{"host":"wsl2"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestFleetStatsHistory_InvalidDays(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/fleet-stats/history?days=abc")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}
