package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func ticketStatus(t *testing.T, url string) (status, claimedBy string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	var tk struct {
		Status    string `json:"status"`
		ClaimedBy string `json:"claimed_by"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tk); err != nil {
		t.Fatalf("decode ticket: %v", err)
	}
	return tk.Status, tk.ClaimedBy
}

func TestTicketReviewAndRework_RouteContract(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()
	createBoardTicket(t, ts, "S1", "T1", "ready")
	postJSONBody(t, ts.URL+"/api/v1/tickets/T1/claim", `{"agent_id":"agent-a"}`)
	review := ts.URL + "/api/v1/tickets/T1/review"
	rework := ts.URL + "/api/v1/tickets/T1/rework"

	if resp := postJSONBody(t, review, `{}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("review without agent_id = %d, want 400", resp.StatusCode)
	}
	if resp := postJSONBody(t, review, `{"agent_id":"agent-b"}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("review by non-holder = %d, want 409", resp.StatusCode)
	}
	if resp := postJSONBody(t, ts.URL+"/api/v1/tickets/nope/review", `{"agent_id":"agent-a"}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("review of unknown ticket = %d, want 409", resp.StatusCode)
	}
	if resp := postJSONBody(t, review, `{"agent_id":"agent-a","note":"PR is up"}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("review by holder = %d, want 200", resp.StatusCode)
	}
	if resp := postJSONBody(t, review, `{"agent_id":"agent-a","note":"new head"}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("re-submit by holder = %d, want 200", resp.StatusCode)
	}
	if st, by := ticketStatus(t, ts.URL+"/api/v1/tickets/T1"); st != "review" || by != "agent-a" {
		t.Fatalf("after review: status %q claimed_by %q, want review / agent-a", st, by)
	}

	if resp := postJSONBody(t, rework, `{"actor":"reviewer"}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("rework without reason = %d, want 400", resp.StatusCode)
	}
	if resp := postJSONBody(t, rework, `{"actor":"reviewer","reason":"tests fail"}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("rework = %d, want 200", resp.StatusCode)
	}
	if st, by := ticketStatus(t, ts.URL+"/api/v1/tickets/T1"); st != "in_progress" || by != "agent-a" {
		t.Fatalf("after rework: status %q claimed_by %q, want in_progress / agent-a", st, by)
	}
	if resp := postJSONBody(t, rework, `{"actor":"reviewer","reason":"again"}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("rework of a ticket not in review = %d, want 409", resp.StatusCode)
	}
}
