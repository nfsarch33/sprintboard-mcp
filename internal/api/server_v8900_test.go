package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
)

// TestTicketSearch_v8900 (B16): GET /api/v1/tickets/search hits SearchTickets
// and returns the encoded ticket list with the requested filters applied.
func TestTicketSearch_v8900(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/api/v1/sprints", `{"id":"sp1","name":"Sprint 1"}`)
	resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("create sprint = %d", resp.StatusCode)
	}

	tickets := []string{
		`{"id":"tk1","sprint_id":"sp1","title":"Wire LLM provider","priority":9,"status":"ready","owner_agent":"alice"}`,
		`{"id":"tk2","sprint_id":"sp1","title":"SSE streaming","priority":7,"status":"in_progress","owner_agent":"bob"}`,
		`{"id":"tk3","sprint_id":"sp1","title":"Docs polish","priority":3,"status":"backlog","owner_agent":"alice"}`,
	}
	for _, body := range tickets {
		r := postJSON(t, ts.URL+"/api/v1/tickets", body)
		r.Body.Close()
		if r.StatusCode != 201 {
			t.Fatalf("create ticket = %d", r.StatusCode)
		}
	}

	q := url.Values{}
	q.Set("priority_min", "7")
	q.Set("owner", "alice")
	r, err := http.Get(ts.URL + "/api/v1/tickets/search?" + q.Encode())
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		t.Fatalf("status = %d", r.StatusCode)
	}
	var got struct {
		Tickets []struct {
			ID string `json:"id"`
		} `json:"tickets"`
		Count int `json:"count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Count != 1 || got.Tickets[0].ID != "tk1" {
		t.Fatalf("unexpected hits: %+v", got)
	}
}

// TestSprintHistory_v8900 (B17): GET /api/v1/sprints lists sprints; status
// filter narrows to a single status bucket.
func TestSprintHistory_v8900(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	bodies := []string{
		`{"id":"h1","name":"First","status":"active"}`,
		`{"id":"h2","name":"Second","status":"planned"}`,
		`{"id":"h3","name":"Third","status":"active"}`,
	}
	for i, b := range bodies {
		r := postJSON(t, ts.URL+"/api/v1/sprints", b)
		r.Body.Close()
		if r.StatusCode != 201 {
			t.Fatalf("create sprint %d = %d", i, r.StatusCode)
		}
	}

	r, err := http.Get(ts.URL + "/api/v1/sprints?status=active")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	defer r.Body.Close()
	var got struct {
		Sprints []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"sprints"`
		Count int `json:"count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Count != 2 {
		t.Fatalf("count = %d, want 2", got.Count)
	}
	for _, sp := range got.Sprints {
		if sp.Status != "active" {
			t.Fatalf("status = %q, want active", sp.Status)
		}
	}
}

// TestSprintMetrics_v8900 (B18): GET /api/v1/sprints/{id}/metrics returns the
// rollup payload (summary + slas + velocity + burndown).
func TestSprintMetrics_v8900(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	r := postJSON(t, ts.URL+"/api/v1/sprints", `{"id":"metrics-sp","name":"Metrics Sprint"}`)
	r.Body.Close()
	if r.StatusCode != 201 {
		t.Fatalf("create sprint = %d", r.StatusCode)
	}
	r = postJSON(t, ts.URL+"/api/v1/tickets",
		`{"id":"mtk1","sprint_id":"metrics-sp","title":"T1","priority":5}`)
	r.Body.Close()
	if r.StatusCode != 201 {
		t.Fatalf("create ticket = %d", r.StatusCode)
	}

	resp, err := http.Get(ts.URL + "/api/v1/sprints/metrics-sp/metrics")
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var got map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, key := range []string{"sprint", "tickets_by_status", "total_tickets", "slas", "velocity", "burndown"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("missing key %q in metrics rollup: %v", key, fmt.Sprintf("%v", got))
		}
	}

	resp, err = http.Get(ts.URL + "/api/v1/sprints/no-such-sprint/metrics")
	if err != nil {
		t.Fatalf("metrics 404: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}
