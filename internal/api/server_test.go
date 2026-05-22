package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"log/slog"

	"github.com/nfsarch33/sprintboard-mcp/internal/api"
	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	store, err := sprintboard.NewStore(dbPath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	srv := api.NewServer(store, logger)
	return httptest.NewServer(srv.Handler())
}

func TestHealthEndpoint(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want ok", body["status"])
	}
}

func TestSprintLifecycle(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	payload := `{"id":"v7300-test","name":"Test Sprint","theme":"testing"}`
	resp, err := http.Post(ts.URL+"/api/v1/sprints", "application/json", bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("POST /sprints: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("create status = %d, want 201", resp.StatusCode)
	}

	resp2, err := http.Get(ts.URL + "/api/v1/sprints/v7300-test")
	if err != nil {
		t.Fatalf("GET /sprints/v7300-test: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp2.StatusCode)
	}
	var sprintBody map[string]interface{}
	if err := json.NewDecoder(resp2.Body).Decode(&sprintBody); err != nil {
		t.Fatalf("decode sprint GET: %v", err)
	}
	sprint, _ := sprintBody["sprint"].(map[string]interface{})
	if sprint == nil {
		t.Fatalf("sprint GET response missing 'sprint' key; got %v", sprintBody)
	}
	if sprint["name"] != "Test Sprint" {
		t.Errorf("sprint name = %v, want Test Sprint", sprint["name"])
	}
	if sprint["id"] != "v7300-test" {
		t.Errorf("sprint id = %v, want v7300-test", sprint["id"])
	}

	resp3, err := http.Post(ts.URL+"/api/v1/sprints/v7300-test/close", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /sprints/close: %v", err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Errorf("close status = %d, want 200", resp3.StatusCode)
	}
}

func TestTicketClaimRacePrevention(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	http.Post(ts.URL+"/api/v1/sprints", "application/json", bytes.NewBufferString(`{"id":"race-test","name":"Race Test"}`))

	http.Post(ts.URL+"/api/v1/tickets", "application/json", bytes.NewBufferString(`{"id":"T-RACE-1","title":"Race ticket","sprint_id":"race-test"}`))

	claim1 := `{"agent_id":"agent-A"}`
	resp1, _ := http.Post(ts.URL+"/api/v1/tickets/T-RACE-1/claim", "application/json", bytes.NewBufferString(claim1))
	defer resp1.Body.Close()

	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("first claim status = %d, want 200", resp1.StatusCode)
	}

	claim2 := `{"agent_id":"agent-B"}`
	resp2, _ := http.Post(ts.URL+"/api/v1/tickets/T-RACE-1/claim", "application/json", bytes.NewBufferString(claim2))
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusConflict {
		t.Errorf("double claim status = %d, want 409", resp2.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&result)
	if result["success"] != false {
		t.Errorf("double claim success = %v, want false", result["success"])
	}
}

func TestAgentRegistration(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	payload := `{"agent_id":"test-agent","surface":"cursor","capabilities":"code,test"}`
	resp, err := http.Post(ts.URL+"/api/v1/agents", "application/json", bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("POST /agents: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("register status = %d, want 201", resp.StatusCode)
	}

	resp2, err := http.Get(ts.URL + "/api/v1/agents")
	if err != nil {
		t.Fatalf("GET /agents: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("list status = %d, want 200", resp2.StatusCode)
	}
	var agentResp map[string]interface{}
	if err := json.NewDecoder(resp2.Body).Decode(&agentResp); err != nil {
		t.Fatalf("decode agent list: %v", err)
	}
	agentList, ok := agentResp["agents"].([]interface{})
	if !ok {
		t.Fatalf("agent response missing 'agents' array; got %v", agentResp)
	}
	found := false
	for _, raw := range agentList {
		a, _ := raw.(map[string]interface{})
		if a["id"] == "test-agent" || a["agent_id"] == "test-agent" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("agent list does not contain test-agent; got %v", agentList)
	}
}

func TestHandoffPublish(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	http.Post(ts.URL+"/api/v1/sprints", "application/json", bytes.NewBufferString(`{"id":"ho-test","name":"Handoff Test"}`))
	http.Post(ts.URL+"/api/v1/tickets", "application/json", bytes.NewBufferString(`{"id":"T-HO-1","title":"Handoff ticket","sprint_id":"ho-test"}`))

	payload := `{"ticket_id":"T-HO-1","from_agent":"agent-A","to_agent":"agent-B","summary":"Test handoff"}`
	resp, err := http.Post(ts.URL+"/api/v1/handoffs", "application/json", bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("POST /handoffs: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("handoff status = %d, want 201", resp.StatusCode)
	}
	var handoffBody map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&handoffBody); err != nil {
		t.Fatalf("decode handoff response: %v", err)
	}
	if handoffBody["handoff_id"] == nil || handoffBody["handoff_id"] == "" {
		t.Errorf("handoff response missing handoff_id; got %v", handoffBody)
	}
	if handoffBody["status"] != "published" {
		t.Errorf("handoff status = %v, want published", handoffBody["status"])
	}
}

// --- Error Path Tests (v7462-v7465) ---

func TestSprintCreate_InvalidJSON(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/sprints", "application/json", bytes.NewBufferString(`{bad json`))
	if err != nil {
		t.Fatalf("POST /sprints: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["error"] == "" {
		t.Error("expected error message in response body")
	}
}

func TestSprintCreate_MissingRequiredFields(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	cases := []struct {
		name    string
		payload string
	}{
		{"missing id", `{"name":"Test"}`},
		{"missing name", `{"id":"s1"}`},
		{"both empty", `{"id":"","name":""}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Post(ts.URL+"/api/v1/sprints", "application/json", bytes.NewBufferString(tc.payload))
			if err != nil {
				t.Fatalf("POST: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", resp.StatusCode)
			}
		})
	}
}

func TestSprintCreate_Duplicate(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	payload := `{"id":"dup-sprint","name":"First"}`
	resp, _ := http.Post(ts.URL+"/api/v1/sprints", "application/json", bytes.NewBufferString(payload))
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("first create status = %d, want 201", resp.StatusCode)
	}

	resp2, _ := http.Post(ts.URL+"/api/v1/sprints", "application/json", bytes.NewBufferString(payload))
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusConflict {
		t.Errorf("duplicate create status = %d, want 409", resp2.StatusCode)
	}
}

func TestSprintStatus_NotFound(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/sprints/nonexistent-sprint-xyz")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestTicketCreate_InvalidJSON(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, _ := http.Post(ts.URL+"/api/v1/tickets", "application/json", bytes.NewBufferString(`not json`))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestTicketCreate_MissingFields(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	cases := []struct {
		name    string
		payload string
	}{
		{"missing id", `{"title":"Task"}`},
		{"missing title", `{"id":"t1"}`},
		{"both empty", `{"id":"","title":""}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, _ := http.Post(ts.URL+"/api/v1/tickets", "application/json", bytes.NewBufferString(tc.payload))
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", resp.StatusCode)
			}
		})
	}
}

func TestTicketComplete_NonOwner(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	http.Post(ts.URL+"/api/v1/sprints", "application/json", bytes.NewBufferString(`{"id":"own-test","name":"Owner Test"}`))
	http.Post(ts.URL+"/api/v1/tickets", "application/json", bytes.NewBufferString(`{"id":"T-OWN-1","title":"Owner ticket","sprint_id":"own-test"}`))
	http.Post(ts.URL+"/api/v1/tickets/T-OWN-1/claim", "application/json", bytes.NewBufferString(`{"agent_id":"agent-owner"}`))

	resp, err := http.Post(ts.URL+"/api/v1/tickets/T-OWN-1/complete", "application/json",
		bytes.NewBufferString(`{"agent_id":"agent-intruder","evidence":"stolen work"}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Error("expected non-200 status for non-owner completing ticket")
	}
}

func TestTicketClaim_InvalidJSON(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	http.Post(ts.URL+"/api/v1/sprints", "application/json", bytes.NewBufferString(`{"id":"cj-test","name":"Claim JSON"}`))
	http.Post(ts.URL+"/api/v1/tickets", "application/json", bytes.NewBufferString(`{"id":"T-CJ-1","title":"Claim JSON ticket","sprint_id":"cj-test"}`))

	resp, _ := http.Post(ts.URL+"/api/v1/tickets/T-CJ-1/claim", "application/json", bytes.NewBufferString(`{broken`))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestAgentRegister_MissingAgentID(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, _ := http.Post(ts.URL+"/api/v1/agents", "application/json", bytes.NewBufferString(`{"surface":"test"}`))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestHandoff_MissingFields(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	cases := []struct {
		name    string
		payload string
	}{
		{"missing ticket_id", `{"to_agent":"b","summary":"s"}`},
		{"missing to_agent", `{"ticket_id":"t","summary":"s"}`},
		{"missing summary", `{"ticket_id":"t","to_agent":"b"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, _ := http.Post(ts.URL+"/api/v1/handoffs", "application/json", bytes.NewBufferString(tc.payload))
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", resp.StatusCode)
			}
		})
	}
}

// --- v7800-B3 mini-jira extension tests ---

func TestTicketCreate_DueDateAndLabels(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	http.Post(ts.URL+"/api/v1/sprints", "application/json", bytes.NewBufferString(`{"id":"v7800","name":"v7800"}`))

	payload := `{"id":"T-EXT-1","title":"With extras","sprint_id":"v7800","priority":2,"due_date":"2026-12-01T15:04:05Z","labels":["P0","infra"]}`
	resp, err := http.Post(ts.URL+"/api/v1/tickets", "application/json", bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("POST /tickets: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}

	got, err := http.Get(ts.URL + "/api/v1/tickets/T-EXT-1")
	if err != nil {
		t.Fatalf("GET /tickets: %v", err)
	}
	defer got.Body.Close()
	if got.StatusCode != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", got.StatusCode)
	}
	var ticket map[string]interface{}
	if err := json.NewDecoder(got.Body).Decode(&ticket); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ticket["due_date"] == nil {
		t.Errorf("due_date missing in response: %v", ticket)
	}
	labels, _ := ticket["labels"].([]interface{})
	if len(labels) != 2 || labels[0] != "P0" {
		t.Errorf("labels = %v, want [P0 infra]", labels)
	}
}

func TestTicketCreate_RejectsBadDueDate(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	payload := `{"id":"T-BAD","title":"bad","due_date":"yesterday"}`
	resp, _ := http.Post(ts.URL+"/api/v1/tickets", "application/json", bytes.NewBufferString(payload))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for bad due_date", resp.StatusCode)
	}
}

func TestSprintTickets_ListEndpoint(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	http.Post(ts.URL+"/api/v1/sprints", "application/json", bytes.NewBufferString(`{"id":"v7800-list","name":"List"}`))
	http.Post(ts.URL+"/api/v1/tickets", "application/json", bytes.NewBufferString(`{"id":"L1","title":"a","sprint_id":"v7800-list","priority":3}`))
	http.Post(ts.URL+"/api/v1/tickets", "application/json", bytes.NewBufferString(`{"id":"L2","title":"b","sprint_id":"v7800-list","priority":1}`))

	resp, err := http.Get(ts.URL + "/api/v1/sprints/v7800-list/tickets")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		SprintID string                   `json:"sprint_id"`
		Tickets  []map[string]interface{} `json:"tickets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.SprintID != "v7800-list" {
		t.Errorf("sprint_id = %q", body.SprintID)
	}
	if len(body.Tickets) != 2 {
		t.Errorf("tickets count = %d, want 2", len(body.Tickets))
	}
	if body.Tickets[0]["id"] != "L1" {
		t.Errorf("expected priority-DESC ordering, got %v", body.Tickets)
	}
}

func TestSprintSLAs_Endpoint(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	http.Post(ts.URL+"/api/v1/sprints", "application/json", bytes.NewBufferString(`{"id":"v7800-sla","name":"SLA"}`))
	http.Post(ts.URL+"/api/v1/tickets", "application/json", bytes.NewBufferString(`{"id":"S1","title":"sla","sprint_id":"v7800-sla"}`))
	http.Post(ts.URL+"/api/v1/tickets/S1/claim", "application/json", bytes.NewBufferString(`{"agent_id":"a"}`))
	http.Post(ts.URL+"/api/v1/tickets/S1/complete", "application/json", bytes.NewBufferString(`{"agent_id":"a","evidence":"done"}`))

	resp, err := http.Get(ts.URL + "/api/v1/sprints/v7800-sla/slas")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		SprintID string                   `json:"sprint_id"`
		SLAs     []map[string]interface{} `json:"slas"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	if len(body.SLAs) != 1 {
		t.Fatalf("got %d SLAs, want 1", len(body.SLAs))
	}
	if body.SLAs[0]["ticket_id"] != "S1" {
		t.Errorf("SLA ticket_id = %v", body.SLAs[0]["ticket_id"])
	}
}

// --- Concurrent Claim Load Test (v7462-v7465) ---

func TestTicketClaim_Concurrent10(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	http.Post(ts.URL+"/api/v1/sprints", "application/json", bytes.NewBufferString(`{"id":"conc-test","name":"Concurrency Test"}`))
	http.Post(ts.URL+"/api/v1/tickets", "application/json", bytes.NewBufferString(`{"id":"T-CONC-1","title":"Concurrent ticket","sprint_id":"conc-test"}`))

	const goroutines = 10
	var wg sync.WaitGroup
	var successCount atomic.Int32
	var conflictCount atomic.Int32

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			payload := fmt.Sprintf(`{"agent_id":"agent-%d"}`, idx)
			resp, err := http.Post(ts.URL+"/api/v1/tickets/T-CONC-1/claim", "application/json", bytes.NewBufferString(payload))
			if err != nil {
				t.Errorf("goroutine %d: POST error: %v", idx, err)
				return
			}
			defer resp.Body.Close()
			switch resp.StatusCode {
			case http.StatusOK:
				successCount.Add(1)
			case http.StatusConflict:
				conflictCount.Add(1)
			default:
				t.Errorf("goroutine %d: unexpected status %d", idx, resp.StatusCode)
			}
		}(i)
	}
	wg.Wait()

	if s := successCount.Load(); s != 1 {
		t.Errorf("success count = %d, want exactly 1", s)
	}
	if c := conflictCount.Load(); c != goroutines-1 {
		t.Errorf("conflict count = %d, want %d", c, goroutines-1)
	}
}
