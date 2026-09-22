package api_test

import (
	"net/http"
	"testing"
)

// v18860-1 API layer: the claim-lease renewal route over HTTP.

func TestTicketRenew_RouteContract(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()
	createBoardTicket(t, ts, "S1", "T1", "ready")
	postJSONBody(t, ts.URL+"/api/v1/tickets/T1/claim", `{"agent_id":"agent-a"}`)

	// Holder renews: 200.
	if resp := postJSONBody(t, ts.URL+"/api/v1/tickets/T1/renew", `{"agent_id":"agent-a"}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("renew by holder = %d, want 200", resp.StatusCode)
	}
	// Non-holder: 409, and it must not have stolen the lease.
	if resp := postJSONBody(t, ts.URL+"/api/v1/tickets/T1/renew", `{"agent_id":"agent-b"}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("renew by non-holder = %d, want 409", resp.StatusCode)
	}
	// Missing agent_id: 400.
	if resp := postJSONBody(t, ts.URL+"/api/v1/tickets/T1/renew", `{}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("renew without agent_id = %d, want 400", resp.StatusCode)
	}
	// Unknown ticket: 409 from the claimant guard (an UPDATE that matched
	// nothing is indistinguishable from "not yours"), never a 500.
	if resp := postJSONBody(t, ts.URL+"/api/v1/tickets/nope/renew", `{"agent_id":"agent-a"}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("renew unknown ticket = %d, want 409", resp.StatusCode)
	}
	// The ticket is still claimed by agent-a afterwards.
	get, err := http.Get(ts.URL + "/api/v1/tickets/T1")
	if err != nil {
		t.Fatalf("GET ticket: %v", err)
	}
	defer get.Body.Close()
	if get.StatusCode != http.StatusOK {
		t.Fatalf("GET after renew = %d, want 200", get.StatusCode)
	}
}
