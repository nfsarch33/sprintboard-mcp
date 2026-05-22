// T-8800-B13/B14/B15 REST: sprint templates, agent workload, sprint burndown.
package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

func postJSON(t *testing.T, url, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(url, "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

func TestV8800_B13_TemplateLifecycle(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	body := `{
	  "id":"tmpl-overnight",
	  "name":"Overnight",
	  "theme":"overnight",
	  "tickets":[
	    {"title":"Foundations","priority":90,"labels":["foundation"]},
	    {"title":"Closeout","priority":50}
	  ]
	}`
	resp := postJSON(t, ts.URL+"/api/v1/templates", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}
	resp.Body.Close()

	resp, err := http.Get(ts.URL + "/api/v1/templates")
	if err != nil {
		t.Fatalf("GET templates: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d", resp.StatusCode)
	}
	var listed struct {
		Templates []sprintboard.SprintTemplate `json:"templates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listed.Templates) != 1 || listed.Templates[0].ID != "tmpl-overnight" {
		t.Fatalf("listed = %+v", listed.Templates)
	}

	resp = postJSON(t, ts.URL+"/api/v1/templates/tmpl-overnight/instantiate",
		`{"sprint":{"id":"v8800-real","name":"v8800 real","owner_agent":"claude-code","status":"active"}}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := readAllString(resp.Body)
		t.Fatalf("instantiate status = %d body=%s", resp.StatusCode, body)
	}
	var inst sprintboard.SprintInstantiation
	if err := json.NewDecoder(resp.Body).Decode(&inst); err != nil {
		t.Fatalf("decode inst: %v", err)
	}
	if inst.Sprint.ID != "v8800-real" || len(inst.Tickets) != 2 {
		t.Fatalf("inst = %+v", inst)
	}
}

func TestV8800_B13_TemplateInstantiate_NotFound(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()
	resp := postJSON(t, ts.URL+"/api/v1/templates/missing/instantiate",
		`{"sprint":{"id":"x","name":"x"}}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestV8800_B14_AgentWorkloadEndpoint(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/api/v1/sprints", `{"id":"s","name":"s"}`)
	resp.Body.Close()
	mkTicket := func(id, owner string) {
		resp := postJSON(t, ts.URL+"/api/v1/tickets",
			`{"id":"`+id+`","title":"`+id+`","sprint_id":"s","priority":1}`)
		resp.Body.Close()
		resp = postJSON(t, ts.URL+"/api/v1/tickets/"+id+"/claim",
			`{"agent_id":"`+owner+`"}`)
		resp.Body.Close()
	}
	mkTicket("a1", "alice")
	mkTicket("a2", "alice")
	mkTicket("b1", "bob")

	resp, err := http.Get(ts.URL + "/api/v1/agents/workload?sprint_id=s")
	if err != nil {
		t.Fatalf("GET workload: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out struct {
		Workload []sprintboard.AgentWorkloadEntry `json:"workload"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	by := map[string]sprintboard.AgentWorkloadEntry{}
	for _, e := range out.Workload {
		by[e.AgentID] = e
	}
	if by["alice"].ActiveTickets != 2 {
		t.Errorf("alice active = %d, want 2 (got %+v)", by["alice"].ActiveTickets, by["alice"])
	}
	if by["bob"].ActiveTickets != 1 {
		t.Errorf("bob active = %d, want 1", by["bob"].ActiveTickets)
	}
}

func TestV8800_B15_SprintBurndownEndpoint(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/api/v1/sprints", `{"id":"sb","name":"sb"}`)
	resp.Body.Close()
	for _, id := range []string{"t1", "t2", "t3"} {
		resp := postJSON(t, ts.URL+"/api/v1/tickets",
			`{"id":"`+id+`","title":"`+id+`","sprint_id":"sb"}`)
		resp.Body.Close()
	}
	for _, id := range []string{"t1", "t2"} {
		resp := postJSON(t, ts.URL+"/api/v1/tickets/"+id+"/claim",
			`{"agent_id":"alice"}`)
		resp.Body.Close()
		resp = postJSON(t, ts.URL+"/api/v1/tickets/"+id+"/complete",
			`{"agent_id":"alice","evidence":"done"}`)
		resp.Body.Close()
	}

	resp, err := http.Get(ts.URL + "/api/v1/sprints/sb/burndown")
	if err != nil {
		t.Fatalf("GET burndown: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out struct {
		SprintID string                      `json:"sprint_id"`
		Points   []sprintboard.BurndownPoint `json:"points"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.SprintID != "sb" {
		t.Errorf("sprint_id = %q", out.SprintID)
	}
	if len(out.Points) < 3 {
		t.Fatalf("points = %d, want >=3", len(out.Points))
	}
	last := out.Points[len(out.Points)-1]
	if last.TicketsDone != 2 || last.RemainingCount != 1 {
		t.Errorf("last point = %+v", last)
	}
	for _, p := range out.Points {
		if p.Timestamp.IsZero() {
			t.Errorf("zero timestamp at %+v", p)
		}
	}
	if time.Since(out.Points[0].Timestamp) > 30*time.Minute {
		t.Errorf("first timestamp suspicious: %v", out.Points[0].Timestamp)
	}
}

func readAllString(r interface {
	Read(p []byte) (int, error)
}) (string, error) {
	var sb strings.Builder
	buf := make([]byte, 1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			sb.Write(buf[:n])
		}
		if err != nil {
			if err.Error() == "EOF" {
				return sb.String(), nil
			}
			return sb.String(), err
		}
	}
}
