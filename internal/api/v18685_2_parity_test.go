package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// v18685-2: SprintBoard feature parity.
// (1) GET /api/v1/sprints accepts `?tenant_id=...` query param; when present,
//     only sprints whose tenant_id matches are returned. When absent, all
//     sprints are returned (backward-compat).
// (2) POST /api/v1/sprints accepts `owner_agent` (created_by_agent audit) and
//     `tenant_id` fields in the request body; the response (and subsequent
//     GET) reflects the stored values.

func TestSprintTenantFilter_v18685_2(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	bodies := []string{
		`{"id":"tnt-a-1","name":"Tenant A sprint 1","tenant_id":"tenant-a"}`,
		`{"id":"tnt-b-1","name":"Tenant B sprint 1","tenant_id":"tenant-b"}`,
		`{"id":"tnt-a-2","name":"Tenant A sprint 2","tenant_id":"tenant-a"}`,
		`{"id":"tnt-none","name":"No-tenant sprint"}`,
	}
	for _, b := range bodies {
		r := postJSON(t, ts.URL+"/api/v1/sprints", b)
		_ = r.Body.Close()
		if r.StatusCode != http.StatusCreated {
			t.Fatalf("create sprint %q = %d", b, r.StatusCode)
		}
	}

	// tenant-a → 2 own + 1 legacy (no tenant) = 3 total (per established
	// backward-compat semantic: un-tenanted sprints are visible to all tenants).
	r, err := http.Get(ts.URL + "/api/v1/sprints?tenant_id=tenant-a")
	if err != nil {
		t.Fatalf("list tenant-a: %v", err)
	}
	defer func() { _ = r.Body.Close() }()
	if r.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", r.StatusCode)
	}
	var got struct {
		Sprints []struct {
			ID       string `json:"id"`
			TenantID string `json:"tenant_id"`
		} `json:"sprints"`
		Count int `json:"count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Count != 3 {
		t.Fatalf("tenant-a count = %d, want 3 (own+legacy), got=%+v", got.Count, got.Sprints)
	}
	ids := make([]string, 0, len(got.Sprints))
	for _, sp := range got.Sprints {
		ids = append(ids, sp.ID)
	}
	// tnt-b-1 must NOT leak
	for _, sp := range got.Sprints {
		if sp.ID == "tnt-b-1" {
			t.Fatalf("cross-tenant leak: tenant-a saw tnt-b-1 (%+v)", got.Sprints)
		}
	}

	// tenant-b → 1 own + 1 legacy = 2 total
	r, err = http.Get(ts.URL + "/api/v1/sprints?tenant_id=tenant-b")
	if err != nil {
		t.Fatalf("list tenant-b: %v", err)
	}
	defer func() { _ = r.Body.Close() }()
	var gotB struct {
		Count int `json:"count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&gotB); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if gotB.Count != 2 {
		t.Fatalf("tenant-b count = %d, want 2 (own+legacy)", gotB.Count)
	}

	// no filter → all 4 (backward-compat)
	r, err = http.Get(ts.URL + "/api/v1/sprints")
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	defer func() { _ = r.Body.Close() }()
	var gotAll struct {
		Count int `json:"count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&gotAll); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if gotAll.Count != 4 {
		t.Fatalf("all count = %d, want 4 (backward-compat)", gotAll.Count)
	}
	_ = ids
}

func TestSprintCreate_StampsOwnerAgentAndTenantID_v18685_2(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	body := `{"id":"audit-sp","name":"Audit sprint","owner_agent":"cursor-parent","tenant_id":"tenant-x"}`
	r := postJSON(t, ts.URL+"/api/v1/sprints", body)
	_ = r.Body.Close()
	if r.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", r.StatusCode)
	}

	r2, err := http.Get(ts.URL + "/api/v1/sprints/audit-sp")
	if err != nil {
		t.Fatalf("GET /sprints/audit-sp: %v", err)
	}
	defer func() { _ = r2.Body.Close() }()
	if r2.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", r2.StatusCode)
	}
	var got struct {
		Sprint struct {
			ID         string `json:"id"`
			OwnerAgent string `json:"owner_agent"`
			TenantID   string `json:"tenant_id"`
		} `json:"sprint"`
	}
	if err := json.NewDecoder(r2.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Sprint.OwnerAgent != "cursor-parent" {
		t.Fatalf("owner_agent = %q, want cursor-parent", got.Sprint.OwnerAgent)
	}
	if got.Sprint.TenantID != "tenant-x" {
		t.Fatalf("tenant_id = %q, want tenant-x", got.Sprint.TenantID)
	}
}
