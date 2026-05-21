package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
}
