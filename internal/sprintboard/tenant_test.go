package sprintboard

import (
	"path/filepath"
	"testing"
)

// helper: open an in-memory-like store backed by a temp file
func newTestStoreForTenants(t *testing.T) *Store {
	t.Helper()
	tmp := filepath.Join(t.TempDir(), "sb.db")
	st, err := NewStore(tmp)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestCreateSprint_StoresTenantID(t *testing.T) {
	st := newTestStoreForTenants(t)
	if err := st.CreateSprint(Sprint{ID: "s1", Name: "n", TenantID: "tenant-a"}); err != nil {
		t.Fatalf("CreateSprint: %v", err)
	}
	got, err := st.GetSprint("s1")
	if err != nil {
		t.Fatalf("GetSprint: %v", err)
	}
	if got.TenantID != "tenant-a" {
		t.Errorf("TenantID = %q, want tenant-a (got: %+v)", got.TenantID, got)
	}
}

func TestListSprintsByTenant_OnlyReturnsMatchingTenant(t *testing.T) {
	st := newTestStoreForTenants(t)
	must := func(id, name, tenant string) {
		t.Helper()
		if err := st.CreateSprint(Sprint{ID: id, Name: name, TenantID: tenant}); err != nil {
			t.Fatalf("CreateSprint(%s): %v", id, err)
		}
	}
	must("s-a-1", "alpha", "tenant-a")
	must("s-b-1", "beta", "tenant-b")
	must("s-legacy", "no-tenant", "") // pre-M-T migration (backward-compat)

	got, err := st.ListSprintsByTenant("tenant-a")
	if err != nil {
		t.Fatalf("ListSprintsByTenant: %v", err)
	}
	if len(got) != 2 {
		// tenant-a + legacy (NULL/empty tenant) are visible to tenant-a
		ids := sprintIDs(got)
		t.Logf("tenant-a sees: %v", ids)
		if !containsString(ids, "s-a-1") || !containsString(ids, "s-legacy") {
			t.Errorf("tenant-a missing s-a-1 or s-legacy in %v", ids)
		}
		if containsString(ids, "s-b-1") {
			t.Errorf("tenant-a should not see s-b-1 in %v", ids)
		}
	}
}

func TestListSprintsByTenant_EmptyTenantReturnsAll(t *testing.T) {
	st := newTestStoreForTenants(t)
	must := func(id, name, tenant string) {
		t.Helper()
		if err := st.CreateSprint(Sprint{ID: id, Name: name, TenantID: tenant}); err != nil {
			t.Fatalf("CreateSprint(%s): %v", id, err)
		}
	}
	must("s-a", "alpha", "tenant-a")
	must("s-b", "beta", "tenant-b")
	must("s-c", "gamma", "tenant-c")

	got, err := st.ListSprintsByTenant("")
	if err != nil {
		t.Fatalf("ListSprintsByTenant: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("empty tenant should return all 3 sprints, got %d", len(got))
	}
}

func TestGetSprintForTenant_IsolatesAcrossTenants(t *testing.T) {
	st := newTestStoreForTenants(t)
	must := func(id, name, tenant string) {
		t.Helper()
		if err := st.CreateSprint(Sprint{ID: id, Name: name, TenantID: tenant}); err != nil {
			t.Fatalf("CreateSprint(%s): %v", id, err)
		}
	}
	must("s1", "alpha", "tenant-a")
	must("s2", "beta", "tenant-b")

	// Same tenant: succeeds
	if _, err := st.GetSprintForTenant("s1", "tenant-a"); err != nil {
		t.Errorf("tenant-a should see s1: %v", err)
	}
	// Cross-tenant: returns not-found error
	if _, err := st.GetSprintForTenant("s1", "tenant-b"); err == nil {
		t.Errorf("tenant-b should NOT see s1 (cross-tenant leak)")
	}
	// Empty tenant: backward-compat (allows any)
	if _, err := st.GetSprintForTenant("s1", ""); err != nil {
		t.Errorf("empty tenant should see s1 (backward-compat): %v", err)
	}
}

func TestCreateTicket_StoresTenantID(t *testing.T) {
	st := newTestStoreForTenants(t)
	if err := st.CreateSprint(Sprint{ID: "s1", Name: "n", TenantID: "tenant-x"}); err != nil {
		t.Fatalf("CreateSprint: %v", err)
	}
	if err := st.CreateTicket(Ticket{ID: "t1", SprintID: "s1", Title: "demo", TenantID: "tenant-x"}); err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	got, err := st.GetTicket("t1")
	if err != nil {
		t.Fatalf("GetTicket: %v", err)
	}
	if got.TenantID != "tenant-x" {
		t.Errorf("Ticket.TenantID = %q, want tenant-x", got.TenantID)
	}
}

func TestListTicketsByTenant_IsolatesAcrossTenants(t *testing.T) {
	st := newTestStoreForTenants(t)
	mustS := func(id, name, tenant string) {
		t.Helper()
		if err := st.CreateSprint(Sprint{ID: id, Name: name, TenantID: tenant}); err != nil {
			t.Fatalf("CreateSprint(%s): %v", id, err)
		}
	}
	mustT := func(id, sid, title, tenant string) {
		t.Helper()
		if err := st.CreateTicket(Ticket{ID: id, SprintID: sid, Title: title, TenantID: tenant}); err != nil {
			t.Fatalf("CreateTicket(%s): %v", id, err)
		}
	}
	mustS("s-a", "alpha", "tenant-a")
	mustS("s-b", "beta", "tenant-b")
	mustT("t-a-1", "s-a", "alpha-ticket-1", "tenant-a")
	mustT("t-b-1", "s-b", "beta-ticket-1", "tenant-b")
	mustT("t-a-2", "s-a", "alpha-ticket-2", "tenant-a")

	// tenant-a sees its own + legacy, not tenant-b
	got, err := st.ListTicketsByTenant("s-a", "tenant-a")
	if err != nil {
		t.Fatalf("ListTicketsByTenant: %v", err)
	}
	ids := ticketIDs(got)
	if !containsString(ids, "t-a-1") || !containsString(ids, "t-a-2") {
		t.Errorf("tenant-a missing own tickets in %v", ids)
	}
	if containsString(ids, "t-b-1") {
		t.Errorf("tenant-a should not see t-b-1 in %v", ids)
	}
}

func TestGetTicketForTenant_CrossTenantRejected(t *testing.T) {
	st := newTestStoreForTenants(t)
	must := func(id, name, tenant string) {
		t.Helper()
		if err := st.CreateSprint(Sprint{ID: id, Name: name, TenantID: tenant}); err != nil {
			t.Fatalf("CreateSprint(%s): %v", id, err)
		}
	}
	must("s1", "alpha", "tenant-a")
	if err := st.CreateTicket(Ticket{ID: "t1", SprintID: "s1", Title: "t", TenantID: "tenant-a"}); err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	if _, err := st.GetTicketForTenant("t1", "tenant-b"); err == nil {
		t.Errorf("tenant-b should NOT see tenant-a's ticket")
	}
}

func sprintIDs(sprints []Sprint) []string {
	out := make([]string, 0, len(sprints))
	for _, s := range sprints {
		out = append(out, s.ID)
	}
	return out
}

func ticketIDs(tickets []Ticket) []string {
	out := make([]string, 0, len(tickets))
	for _, t := range tickets {
		out = append(out, t.ID)
	}
	return out
}

func containsString(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
