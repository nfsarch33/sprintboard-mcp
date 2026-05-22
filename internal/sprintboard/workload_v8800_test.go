// T-8800-B14: agent workload analytics. Returns active load per agent across
// open tickets so an orchestrator can pick the least-loaded worker.
package sprintboard

import (
	"path/filepath"
	"testing"
)

func newWorkloadStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "workload.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func seedWorkload(t *testing.T, st *Store) {
	t.Helper()
	if err := st.CreateSprint(Sprint{ID: "s1", Name: "wl"}); err != nil {
		t.Fatalf("sprint: %v", err)
	}
	specs := []struct {
		id, owner, claim string
		status           TicketStatus
	}{
		{"t1", "alice", "alice", StatusInProgress},
		{"t2", "alice", "alice", StatusReview},
		{"t3", "alice", "alice", StatusDone},
		{"t4", "bob", "bob", StatusInProgress},
		{"t5", "bob", "", StatusReady},
		{"t6", "carol", "", StatusBacklog},
	}
	for _, sp := range specs {
		tk := Ticket{ID: sp.id, SprintID: "s1", Title: sp.id, OwnerAgent: sp.owner, Status: StatusReady, Priority: 10}
		if err := st.CreateTicket(tk); err != nil {
			t.Fatalf("ticket %s: %v", sp.id, err)
		}
		if sp.status != StatusReady {
			if err := st.UpdateTicket(sp.id, sp.status, sp.owner, ""); err != nil {
				t.Fatalf("update %s: %v", sp.id, err)
			}
		}
	}
}

func TestStore_AgentWorkload_CountsActive(t *testing.T) {
	t.Parallel()
	st := newWorkloadStore(t)
	seedWorkload(t, st)
	wl, err := st.AgentWorkload("s1")
	if err != nil {
		t.Fatalf("AgentWorkload: %v", err)
	}
	got := map[string]AgentWorkloadEntry{}
	for _, w := range wl {
		got[w.AgentID] = w
	}
	if got["alice"].ActiveTickets != 2 || got["alice"].DoneTickets != 1 {
		t.Errorf("alice = %+v, want active=2 done=1", got["alice"])
	}
	if got["bob"].ActiveTickets != 1 || got["bob"].DoneTickets != 0 {
		t.Errorf("bob = %+v, want active=1 done=0", got["bob"])
	}
	if got["carol"].ActiveTickets != 0 {
		t.Errorf("carol = %+v, want active=0", got["carol"])
	}
}

func TestStore_AgentWorkload_OrdersByActiveDesc(t *testing.T) {
	t.Parallel()
	st := newWorkloadStore(t)
	seedWorkload(t, st)
	wl, err := st.AgentWorkload("s1")
	if err != nil {
		t.Fatalf("AgentWorkload: %v", err)
	}
	if len(wl) < 2 {
		t.Fatalf("len = %d", len(wl))
	}
	for i := 1; i < len(wl); i++ {
		if wl[i-1].ActiveTickets < wl[i].ActiveTickets {
			t.Fatalf("not sorted desc: %+v", wl)
		}
	}
}

func TestStore_AgentWorkload_AllSprints(t *testing.T) {
	t.Parallel()
	st := newWorkloadStore(t)
	seedWorkload(t, st)
	if err := st.CreateSprint(Sprint{ID: "s2", Name: "two"}); err != nil {
		t.Fatalf("sprint: %v", err)
	}
	if err := st.CreateTicket(Ticket{ID: "t10", SprintID: "s2", Title: "t10", OwnerAgent: "alice", Status: StatusReady}); err != nil {
		t.Fatalf("t10: %v", err)
	}
	if err := st.UpdateTicket("t10", StatusInProgress, "alice", ""); err != nil {
		t.Fatalf("update t10: %v", err)
	}
	wl, err := st.AgentWorkload("")
	if err != nil {
		t.Fatalf("AgentWorkload(\"\"): %v", err)
	}
	got := map[string]AgentWorkloadEntry{}
	for _, w := range wl {
		got[w.AgentID] = w
	}
	if got["alice"].ActiveTickets != 3 {
		t.Errorf("alice across sprints = %d, want 3", got["alice"].ActiveTickets)
	}
}
