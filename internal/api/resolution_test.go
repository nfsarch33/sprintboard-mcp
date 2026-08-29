package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// escalatedTicket reproduces the state the fleet agent leaves behind when it
// escalates: the ticket is claimed by the agent and still in_progress, with
// the failure evidence in a comment and no completion. That is the state a
// human previously had no route to close.
func escalatedTicket(t *testing.T, baseURL, ticketID, agentID string) {
	t.Helper()
	postJSON(t, baseURL+"/api/v1/sprints", `{"id":"sp1","name":"Sprint 1"}`).Body.Close()

	r := postJSON(t, baseURL+"/api/v1/tickets",
		`{"id":"`+ticketID+`","sprint_id":"sp1","title":"poison","status":"ready"}`)
	r.Body.Close()
	if r.StatusCode != http.StatusCreated {
		t.Fatalf("create ticket = %d", r.StatusCode)
	}

	r = postJSON(t, baseURL+"/api/v1/tickets/"+ticketID+"/claim", `{"agent_id":"`+agentID+`"}`)
	r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("claim = %d", r.StatusCode)
	}

	r = postJSON(t, baseURL+"/api/v1/tickets/"+ticketID+"/comments",
		`{"author":"`+agentID+`","body":"verifier failed; escalating to a human"}`)
	r.Body.Close()
	if r.StatusCode != http.StatusCreated {
		t.Fatalf("comment = %d", r.StatusCode)
	}
}

func getTicket(t *testing.T, baseURL, ticketID string) map[string]any {
	t.Helper()
	r, err := http.Get(baseURL + "/api/v1/tickets/" + ticketID)
	if err != nil {
		t.Fatalf("GET ticket: %v", err)
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("GET ticket = %d", r.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(r.Body).Decode(&out); err != nil {
		t.Fatalf("decode ticket: %v", err)
	}
	return out
}

// TestTicketResolve_HumanClosesEscalation is the route this whole change
// exists for: someone who is NOT the claimer closes an escalated ticket.
func TestTicketResolve_HumanClosesEscalation(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()
	escalatedTicket(t, ts.URL, "copilot-02", "fleet-agent-01")

	r := postJSON(t, ts.URL+"/api/v1/tickets/copilot-02/resolve",
		`{"actor":"operator","reason":"deliberate poison ticket; no human action needed"}`)
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(r.Body)
		t.Fatalf("resolve = %d: %s", r.StatusCode, body)
	}

	got := getTicket(t, ts.URL, "copilot-02")
	if got["status"] != "resolved_by_human" {
		t.Errorf("status = %v, want resolved_by_human", got["status"])
	}
	if got["resolved_by"] != "operator" {
		t.Errorf("resolved_by = %v, want operator", got["resolved_by"])
	}
	if got["resolution_reason"] != "deliberate poison ticket; no human action needed" {
		t.Errorf("resolution_reason = %v", got["resolution_reason"])
	}
	if got["resolved_at"] == nil {
		t.Error("resolved_at absent from the ticket")
	}
	// The escalating agent stays on the record.
	if got["claimed_by"] != "fleet-agent-01" {
		t.Errorf("claimed_by = %v, want the escalating agent preserved", got["claimed_by"])
	}
}

// TestTicketResolve_DoesNotRelaxCompleteGuard is the control that must never
// go green for the wrong reason. Only-the-claimer-may-complete is a correct
// guard on the AGENT path; the resolve route exists so that guard does not
// have to be weakened. If a later change "simplifies" the two paths into one
// permissive close, this test fails.
func TestTicketResolve_DoesNotRelaxCompleteGuard(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()
	escalatedTicket(t, ts.URL, "copilot-03", "fleet-agent-01")

	// A human calling /complete is still refused, both as themselves and as
	// the anonymous caller that produced the original 500.
	for _, body := range []string{
		`{"agent_id":"operator","evidence":"closing by hand"}`,
		`{"evidence":"closing by hand"}`,
	} {
		r := postJSON(t, ts.URL+"/api/v1/tickets/copilot-03/complete", body)
		r.Body.Close()
		if r.StatusCode < 400 {
			t.Fatalf("complete as non-claimer with %s = %d, want a refusal", body, r.StatusCode)
		}
	}
	if got := getTicket(t, ts.URL, "copilot-03"); got["status"] != "in_progress" {
		t.Fatalf("status = %v after refused completions, want in_progress", got["status"])
	}

	// And the claimer itself can still complete: the guard rejects the wrong
	// caller, not every caller.
	r := postJSON(t, ts.URL+"/api/v1/tickets/copilot-03/complete",
		`{"agent_id":"fleet-agent-01","evidence":"finished after all"}`)
	r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("complete as claimer = %d, want 200", r.StatusCode)
	}
	if got := getTicket(t, ts.URL, "copilot-03"); got["status"] != "done" {
		t.Errorf("status = %v, want done", got["status"])
	}
}

func TestTicketResolve_RejectsMissingActorOrReason(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()
	escalatedTicket(t, ts.URL, "copilot-04", "fleet-agent-01")

	for _, body := range []string{
		`{"reason":"closing"}`,
		`{"actor":"operator"}`,
		`{}`,
	} {
		r := postJSON(t, ts.URL+"/api/v1/tickets/copilot-04/resolve", body)
		r.Body.Close()
		if r.StatusCode != http.StatusBadRequest {
			t.Errorf("resolve %s = %d, want 400", body, r.StatusCode)
		}
	}
	if got := getTicket(t, ts.URL, "copilot-04"); got["status"] != "in_progress" {
		t.Errorf("status = %v after rejected resolutions, want in_progress", got["status"])
	}
}

func TestTicketResolve_StatusCodes(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()
	escalatedTicket(t, ts.URL, "tk1", "agent-a")

	r := postJSON(t, ts.URL+"/api/v1/tickets/does-not-exist/resolve",
		`{"actor":"operator","reason":"x"}`)
	r.Body.Close()
	if r.StatusCode != http.StatusNotFound {
		t.Errorf("resolve unknown ticket = %d, want 404", r.StatusCode)
	}

	r = postJSON(t, ts.URL+"/api/v1/tickets/tk1/resolve", `{"actor":"operator","reason":"first"}`)
	r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("first resolve = %d", r.StatusCode)
	}

	r = postJSON(t, ts.URL+"/api/v1/tickets/tk1/resolve", `{"actor":"someone","reason":"second"}`)
	r.Body.Close()
	if r.StatusCode != http.StatusConflict {
		t.Errorf("second resolve = %d, want 409", r.StatusCode)
	}
	if got := getTicket(t, ts.URL, "tk1"); got["resolved_by"] != "operator" {
		t.Errorf("resolved_by = %v, want the first resolver to stand", got["resolved_by"])
	}
}

// TestStaleTicketsView covers the board view that makes an abandoned
// escalation visible. AgentStalled cannot: it treats an escalation as a
// terminal outcome for the agent, so the handed-over ticket disappears from
// its view at exactly the moment a human becomes responsible for it.
func TestStaleTicketsView(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()
	escalatedTicket(t, ts.URL, "tk1", "fleet-agent-01")

	type staleResp struct {
		ThresholdHours float64 `json:"threshold_hours"`
		Count          int     `json:"count"`
		Tickets        []struct {
			ID         string `json:"id"`
			ClaimedBy  string `json:"claimed_by"`
			AgeSeconds int64  `json:"age_seconds"`
		} `json:"tickets"`
	}
	fetch := func(query string) staleResp {
		t.Helper()
		r, err := http.Get(ts.URL + "/api/v1/tickets/stale" + query)
		if err != nil {
			t.Fatalf("GET stale: %v", err)
		}
		defer r.Body.Close()
		if r.StatusCode != http.StatusOK {
			t.Fatalf("GET stale%s = %d", query, r.StatusCode)
		}
		var out staleResp
		if err := json.NewDecoder(r.Body).Decode(&out); err != nil {
			t.Fatalf("decode stale: %v", err)
		}
		return out
	}

	// Positive control: with a zero window the freshly claimed ticket is
	// visible. Without it, "the list is empty" below would prove nothing.
	got := fetch("?hours=0")
	if got.Count != 1 || got.Tickets[0].ID != "tk1" {
		t.Fatalf("hours=0 = %+v, want tk1", got)
	}
	if got.Tickets[0].ClaimedBy != "fleet-agent-01" {
		t.Errorf("claimed_by = %q", got.Tickets[0].ClaimedBy)
	}

	// A ticket claimed seconds ago is not stale on the default window.
	if got := fetch(""); got.Count != 0 || got.ThresholdHours != 24 {
		t.Errorf("default window = %+v, want 0 tickets at 24h", got)
	}

	// Resolving takes it off the view for good.
	postJSON(t, ts.URL+"/api/v1/tickets/tk1/resolve",
		`{"actor":"operator","reason":"closing"}`).Body.Close()
	if got := fetch("?hours=0"); got.Count != 0 {
		t.Errorf("stale after resolution = %+v, want empty", got)
	}
}

func TestStaleTicketsView_RejectsBadHours(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	for _, q := range []string{"?hours=soon", "?hours=-1"} {
		r, err := http.Get(ts.URL + "/api/v1/tickets/stale" + q)
		if err != nil {
			t.Fatalf("GET stale%s: %v", q, err)
		}
		r.Body.Close()
		if r.StatusCode != http.StatusBadRequest {
			t.Errorf("GET stale%s = %d, want 400", q, r.StatusCode)
		}
	}
}

// TestMetricsExportsInProgressAge covers the series an alert on abandoned
// escalations has to key off. The counters on this endpoint are
// process-lifetime totals that reset on restart; "stuck for two days" is a
// database property and has to be read at scrape time.
func TestMetricsExportsInProgressAge(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	scrape := func() string {
		t.Helper()
		r, err := http.Get(ts.URL + "/metrics")
		if err != nil {
			t.Fatalf("GET /metrics: %v", err)
		}
		defer r.Body.Close()
		body, _ := io.ReadAll(r.Body)
		return string(body)
	}

	// With nothing in flight the gauge is present and zero -- emitted, not
	// omitted, so an alert can distinguish "nothing stuck" from "no data".
	if body := scrape(); !strings.Contains(body, "sprintboard_tickets_in_progress_oldest_age_seconds 0") {
		t.Errorf("empty board did not export a zero age gauge:\n%s", body)
	}

	escalatedTicket(t, ts.URL, "tk1", "agent-a")
	body := scrape()
	for _, want := range []string{
		"# TYPE sprintboard_tickets_in_progress gauge",
		"sprintboard_tickets_in_progress 1",
		"# TYPE sprintboard_tickets_in_progress_oldest_age_seconds gauge",
		"# TYPE sprintboard_tickets_resolved_total counter",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in:\n%s", want, body)
		}
	}

	postJSON(t, ts.URL+"/api/v1/tickets/tk1/resolve",
		`{"actor":"operator","reason":"closing"}`).Body.Close()

	body = scrape()
	if !strings.Contains(body, "sprintboard_tickets_resolved_total 1") {
		t.Errorf("resolve did not move the counter:\n%s", body)
	}
	if !strings.Contains(body, "sprintboard_tickets_in_progress 0") {
		t.Errorf("resolved ticket still counted as in progress:\n%s", body)
	}
}
