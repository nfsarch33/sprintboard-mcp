package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// v18850 API layer: the terminal guard, the requeue route, and acceptance
// criteria on create, as observed over HTTP.

func postJSONBody(t *testing.T, url, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(url, "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// createBoardTicket provisions a sprint plus one ticket in the given status.
func createBoardTicket(t *testing.T, ts *httptest.Server, sprintID, ticketID, status string) {
	t.Helper()
	postJSONBody(t, ts.URL+"/api/v1/sprints", `{"id":"`+sprintID+`","name":"S"}`)
	body := `{"id":"` + ticketID + `","sprint_id":"` + sprintID + `","title":"t","status":"` + status + `"}`
	if resp := postJSONBody(t, ts.URL+"/api/v1/tickets", body); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create ticket %s = %d, want 201", ticketID, resp.StatusCode)
	}
}

func closeTicketDone(t *testing.T, ts *httptest.Server, ticketID string) {
	t.Helper()
	postJSONBody(t, ts.URL+"/api/v1/tickets/"+ticketID+"/claim", `{"agent_id":"agent-a"}`)
	resp := postJSONBody(t, ts.URL+"/api/v1/tickets/"+ticketID+"/complete", `{"agent_id":"agent-a","evidence":"done"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("complete %s = %d, want 200", ticketID, resp.StatusCode)
	}
}

// A claim against a done ticket must answer 409 (conflict with the ticket's
// closed state), not 500, and must not mutate the ticket.
func TestTicketClaim_TerminalTicketReturns409(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()
	createBoardTicket(t, ts, "S1", "T1", "ready")
	closeTicketDone(t, ts, "T1")

	resp := postJSONBody(t, ts.URL+"/api/v1/tickets/T1/claim", `{"agent_id":"agent-b"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("claim of done ticket = %d, want 409", resp.StatusCode)
	}

	get, err := http.Get(ts.URL + "/api/v1/tickets/T1")
	if err != nil {
		t.Fatalf("GET ticket: %v", err)
	}
	defer get.Body.Close()
	var ticket struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(get.Body).Decode(&ticket); err != nil {
		t.Fatalf("decode ticket: %v", err)
	}
	if ticket.Status != "done" {
		t.Fatalf("status after refused claim = %q, want done", ticket.Status)
	}
}

func TestTicketRequeue_RouteLifecycle(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()
	createBoardTicket(t, ts, "S1", "T1", "ready")
	closeTicketDone(t, ts, "T1")

	resp := postJSONBody(t, ts.URL+"/api/v1/tickets/T1/requeue", `{"actor":"op","reason":"wrong fix, run again"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("requeue of done ticket = %d, want 200", resp.StatusCode)
	}

	// After requeue the ticket is claimable again, by a different agent.
	resp = postJSONBody(t, ts.URL+"/api/v1/tickets/T1/claim", `{"agent_id":"agent-b"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("claim after requeue = %d, want 200", resp.StatusCode)
	}

	// Non-terminal requeue refused with 409.
	createBoardTicket(t, ts, "S1", "T2", "ready")
	resp = postJSONBody(t, ts.URL+"/api/v1/tickets/T2/requeue", `{"actor":"op","reason":"x"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("requeue of ready ticket = %d, want 409", resp.StatusCode)
	}

	// Missing actor refused with 400.
	createBoardTicket(t, ts, "S1", "T3", "ready")
	closeTicketDone(t, ts, "T3")
	resp = postJSONBody(t, ts.URL+"/api/v1/tickets/T3/requeue", `{"actor":"","reason":"why"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("requeue without actor = %d, want 400", resp.StatusCode)
	}

	// Unknown ticket is 404.
	resp = postJSONBody(t, ts.URL+"/api/v1/tickets/T-none/requeue", `{"actor":"op","reason":"x"}`)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("requeue of missing ticket = %d, want 404", resp.StatusCode)
	}
}

// Acceptance criteria on create: the board's whole contract is "done means the
// acceptance criteria passed", so the create route must accept and persist
// them (the store column already existed; the API dropped the field).
func TestTicketCreate_AcceptanceCriteriaPersisted(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()
	createBoardTicket(t, ts, "S1", "T1", "ready")

	resp := postJSONBody(t, ts.URL+"/api/v1/tickets",
		`{"id":"T-ac","sprint_id":"S1","title":"with criteria","acceptance_criteria":"all tests green; PR merged"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create with acceptance_criteria = %d, want 201", resp.StatusCode)
	}

	get, err := http.Get(ts.URL + "/api/v1/tickets/T-ac")
	if err != nil {
		t.Fatalf("GET ticket: %v", err)
	}
	defer get.Body.Close()
	var ticket struct {
		AcceptanceCriteria string `json:"acceptance_criteria"`
	}
	if err := json.NewDecoder(get.Body).Decode(&ticket); err != nil {
		t.Fatalf("decode ticket: %v", err)
	}
	if ticket.AcceptanceCriteria != "all tests green; PR merged" {
		t.Fatalf("acceptance_criteria = %q, want it persisted verbatim", ticket.AcceptanceCriteria)
	}
}
