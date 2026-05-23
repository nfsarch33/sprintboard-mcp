package sprintboard

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSearchTickets_v8900 pins the v8900-B16 contract: SearchTickets must
// support free-text query (title/description/acceptance), status, owner,
// label, priority floor, and sprint filters. An empty filter returns every
// ticket. Results are stable-sorted by priority desc, created_at asc.
func TestSearchTickets_v8900(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "sb.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()

	if err := store.CreateSprint(Sprint{ID: "s1", Name: "Sprint 1", Status: SprintActive}); err != nil {
		t.Fatalf("create sprint s1: %v", err)
	}
	if err := store.CreateSprint(Sprint{ID: "s2", Name: "Sprint 2", Status: SprintPlanned}); err != nil {
		t.Fatalf("create sprint s2: %v", err)
	}

	now := time.Now()
	tickets := []Ticket{
		{ID: "t1", SprintID: "s1", Title: "Ship platform server", Description: "expose /healthz", Status: StatusReady, OwnerAgent: "alice", Priority: 9, Labels: []string{"backend", "go"}, CreatedAt: now.Add(-3 * time.Hour)},
		{ID: "t2", SprintID: "s1", Title: "Wire SSE streaming", Description: "tokens", Status: StatusInProgress, OwnerAgent: "bob", Priority: 7, Labels: []string{"backend"}, CreatedAt: now.Add(-2 * time.Hour)},
		{ID: "t3", SprintID: "s2", Title: "UI dashboard polish", Description: "responsive layout", Status: StatusBacklog, OwnerAgent: "alice", Priority: 3, Labels: []string{"frontend"}, CreatedAt: now.Add(-1 * time.Hour)},
		{ID: "t4", SprintID: "s1", Title: "Doc handoff template", Description: "", Status: StatusDone, OwnerAgent: "charlie", Priority: 5, Labels: []string{"docs"}, CreatedAt: now.Add(-30 * time.Minute), AcceptanceCriteria: "platform team approves"},
	}
	for _, tk := range tickets {
		if err := store.CreateTicket(tk); err != nil {
			t.Fatalf("create ticket %s: %v", tk.ID, err)
		}
	}

	type tc struct {
		name   string
		filter TicketFilter
		wantID []string
	}
	cases := []tc{
		{
			name:   "empty filter returns all tickets sorted",
			filter: TicketFilter{},
			wantID: []string{"t1", "t2", "t4", "t3"},
		},
		{
			name:   "query matches title",
			filter: TicketFilter{Query: "platform"},
			wantID: []string{"t1", "t4"}, // platform server + acceptance "platform team"
		},
		{
			name:   "query matches description",
			filter: TicketFilter{Query: "responsive"},
			wantID: []string{"t3"},
		},
		{
			name:   "status filter",
			filter: TicketFilter{Status: StatusReady},
			wantID: []string{"t1"},
		},
		{
			name:   "owner filter",
			filter: TicketFilter{Owner: "alice"},
			wantID: []string{"t1", "t3"},
		},
		{
			name:   "label filter (any-of)",
			filter: TicketFilter{Labels: []string{"backend"}},
			wantID: []string{"t1", "t2"},
		},
		{
			name:   "priority floor",
			filter: TicketFilter{PriorityMin: 7},
			wantID: []string{"t1", "t2"},
		},
		{
			name:   "sprint filter",
			filter: TicketFilter{SprintID: "s2"},
			wantID: []string{"t3"},
		},
		{
			name:   "limit",
			filter: TicketFilter{Limit: 2},
			wantID: []string{"t1", "t2"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := store.SearchTickets(c.filter)
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			gotIDs := make([]string, len(got))
			for i, tk := range got {
				gotIDs[i] = tk.ID
			}
			if strings.Join(gotIDs, ",") != strings.Join(c.wantID, ",") {
				t.Fatalf("ids = %v, want %v", gotIDs, c.wantID)
			}
		})
	}
}
