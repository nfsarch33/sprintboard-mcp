package sprintboard

import (
	"testing"
	"time"
)

func TestSetTicketEstimate(t *testing.T) {
	store := testStore(t)
	sp := Sprint{ID: "est-sprint", Name: "Estimate Sprint", Status: SprintPlanned}
	if err := store.CreateSprint(sp); err != nil {
		t.Fatalf("CreateSprint: %v", err)
	}

	ticket := Ticket{
		ID:       "est-1",
		SprintID: sp.ID,
		Title:    "estimate test",
		Status:   StatusBacklog,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := store.CreateTicket(ticket); err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}

	if err := store.SetTicketEstimate("est-1", 2.5); err != nil {
		t.Fatalf("SetTicketEstimate: %v", err)
	}

	if err := store.SetTicketEstimate("nonexistent", 1.0); err == nil {
		t.Fatal("expected error for nonexistent ticket")
	}
}

func TestGetSprintBurndown(t *testing.T) {
	store := testStore(t)
	sp := Sprint{ID: "bd-sprint", Name: "Burndown Sprint", Status: SprintPlanned}
	if err := store.CreateSprint(sp); err != nil {
		t.Fatalf("CreateSprint: %v", err)
	}

	for _, id := range []string{"bd-1", "bd-2", "bd-3"} {
		tk := Ticket{ID: id, SprintID: sp.ID, Title: id, Status: StatusBacklog, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		if err := store.CreateTicket(tk); err != nil {
			t.Fatalf("CreateTicket %s: %v", id, err)
		}
	}

	_ = store.SetTicketEstimate("bd-1", 3.0)
	_ = store.SetTicketEstimate("bd-2", 2.0)
	_ = store.SetTicketEstimate("bd-3", 1.5)

	if err := store.UpdateTicket("bd-1", StatusDone, "", ""); err != nil {
		t.Fatalf("UpdateTicket: %v", err)
	}

	entry, err := store.GetSprintBurndown(sp.ID)
	if err != nil {
		t.Fatalf("GetSprintBurndown: %v", err)
	}
	if entry.TotalEstimate != 6.5 {
		t.Errorf("total = %f, want 6.5", entry.TotalEstimate)
	}
	if entry.DoneEstimate != 3.0 {
		t.Errorf("done = %f, want 3.0", entry.DoneEstimate)
	}
	if entry.RemainingEstimate != 3.5 {
		t.Errorf("remaining = %f, want 3.5", entry.RemainingEstimate)
	}
}

func TestStealTicket(t *testing.T) {
	store := testStore(t)
	sp := Sprint{ID: "steal-sprint", Name: "Steal Sprint", Status: SprintPlanned}
	if err := store.CreateSprint(sp); err != nil {
		t.Fatalf("CreateSprint: %v", err)
	}

	ticket := Ticket{ID: "steal-1", SprintID: sp.ID, Title: "steal target", Status: StatusBacklog, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := store.CreateTicket(ticket); err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}

	if _, err := store.ClaimTicket("steal-1", "agent-1"); err != nil {
		t.Fatalf("ClaimTicket: %v", err)
	}

	if err := store.StealTicket("steal-1", "agent-2", "priority override"); err != nil {
		t.Fatalf("StealTicket: %v", err)
	}

	if err := store.StealTicket("nonexistent", "agent-2", "fail"); err == nil {
		t.Fatal("expected error for nonexistent ticket")
	}
}
