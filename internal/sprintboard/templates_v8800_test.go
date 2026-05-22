// T-8800-B13: sprint templates. A reusable bundle of TicketTemplate rows that
// can be instantiated into a fresh Sprint+Tickets in one transactional call.
package sprintboard

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTemplateStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "templates.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func sampleTemplate() SprintTemplate {
	return SprintTemplate{
		ID:          "tmpl-overnight",
		Name:        "Overnight Programme",
		Description: "Standard overnight sprint with foundations, services, and closeout.",
		Theme:       "overnight",
		Tickets: []TicketTemplate{
			{Title: "Foundations", Description: "scaffold + tests", Priority: 90, AcceptanceCriteria: "tests pass", Labels: []string{"foundation"}},
			{Title: "Services", Description: "wire services", Priority: 70, OwnerAgent: "claude-code", Labels: []string{"service"}},
			{Title: "Closeout", Description: "ORHEP + handoff", Priority: 50, Labels: []string{"closeout"}},
		},
	}
}

func TestStore_CreateAndGetSprintTemplate(t *testing.T) {
	t.Parallel()
	st := newTemplateStore(t)
	tmpl := sampleTemplate()
	if err := st.CreateSprintTemplate(tmpl); err != nil {
		t.Fatalf("CreateSprintTemplate: %v", err)
	}
	got, err := st.GetSprintTemplate(tmpl.ID)
	if err != nil {
		t.Fatalf("GetSprintTemplate: %v", err)
	}
	if got.Name != tmpl.Name || got.Theme != tmpl.Theme {
		t.Fatalf("metadata mismatch: %+v", got)
	}
	if len(got.Tickets) != 3 {
		t.Fatalf("ticket count = %d, want 3", len(got.Tickets))
	}
	if got.Tickets[0].Title != "Foundations" || got.Tickets[0].Priority != 90 {
		t.Fatalf("ticket[0] = %+v", got.Tickets[0])
	}
	if len(got.Tickets[0].Labels) != 1 || got.Tickets[0].Labels[0] != "foundation" {
		t.Fatalf("labels lost: %+v", got.Tickets[0].Labels)
	}
	if got.CreatedAt.IsZero() {
		t.Fatalf("created_at not stamped")
	}
}

func TestStore_ListSprintTemplates(t *testing.T) {
	t.Parallel()
	st := newTemplateStore(t)
	for i, name := range []string{"alpha", "beta", "gamma"} {
		tmpl := SprintTemplate{
			ID:   "tmpl-" + name,
			Name: name,
			Tickets: []TicketTemplate{
				{Title: "x", Priority: i + 1},
			},
		}
		if err := st.CreateSprintTemplate(tmpl); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
	got, err := st.ListSprintTemplates()
	if err != nil {
		t.Fatalf("ListSprintTemplates: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
}

func TestStore_DeleteSprintTemplate(t *testing.T) {
	t.Parallel()
	st := newTemplateStore(t)
	tmpl := sampleTemplate()
	if err := st.CreateSprintTemplate(tmpl); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := st.DeleteSprintTemplate(tmpl.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := st.GetSprintTemplate(tmpl.ID); err == nil {
		t.Fatalf("expected not-found after delete")
	}
}

func TestStore_InstantiateSprintFromTemplate(t *testing.T) {
	t.Parallel()
	st := newTemplateStore(t)
	tmpl := sampleTemplate()
	if err := st.CreateSprintTemplate(tmpl); err != nil {
		t.Fatalf("create tmpl: %v", err)
	}
	sprintID := "sprint-v8800"
	created, err := st.InstantiateSprintFromTemplate(tmpl.ID, Sprint{
		ID:         sprintID,
		Name:       "v8800 Overnight",
		OwnerAgent: "claude-code",
		Status:     SprintActive,
		StartAt:    time.Now(),
	})
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	if created.Sprint.ID != sprintID || created.Sprint.Theme != tmpl.Theme {
		t.Fatalf("sprint mismatch: %+v", created.Sprint)
	}
	if len(created.Tickets) != len(tmpl.Tickets) {
		t.Fatalf("tickets len = %d, want %d", len(created.Tickets), len(tmpl.Tickets))
	}
	gotTickets, err := st.ListTickets(sprintID)
	if err != nil {
		t.Fatalf("ListTickets: %v", err)
	}
	if len(gotTickets) != len(tmpl.Tickets) {
		t.Fatalf("listed tickets = %d, want %d", len(gotTickets), len(tmpl.Tickets))
	}
	for _, tk := range gotTickets {
		if tk.SprintID != sprintID {
			t.Errorf("ticket %s sprint = %q, want %q", tk.ID, tk.SprintID, sprintID)
		}
		if !strings.HasPrefix(tk.ID, sprintID+"-") {
			t.Errorf("ticket id %q should be prefixed with sprint id", tk.ID)
		}
	}
}

func TestStore_InstantiateSprintFromTemplate_MissingTemplate(t *testing.T) {
	t.Parallel()
	st := newTemplateStore(t)
	if _, err := st.InstantiateSprintFromTemplate("does-not-exist", Sprint{ID: "s1", Name: "s1"}); err == nil {
		t.Fatalf("expected error for missing template")
	}
}
