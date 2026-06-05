package sprintboard

import "testing"

func TestPriorityReadyList_SortedByPriorityDescThenDAG(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	s.CreateSprint(Sprint{ID: "S-PQ", Name: "Priority Queue"})

	s.CreateTicket(Ticket{ID: "T-LOW", SprintID: "S-PQ", Title: "Low prio", Priority: 1, Status: StatusBacklog})
	s.CreateTicket(Ticket{ID: "T-MED", SprintID: "S-PQ", Title: "Med prio", Priority: 5, Status: StatusBacklog})
	s.CreateTicket(Ticket{ID: "T-HIGH", SprintID: "S-PQ", Title: "High prio", Priority: 10, Status: StatusBacklog})
	s.CreateTicket(Ticket{ID: "T-MED2", SprintID: "S-PQ", Title: "Med prio 2", Priority: 5, Status: StatusBacklog})

	s.AddDependency("T-MED2", "T-MED")

	ready, err := s.PriorityReadyList("S-PQ")
	if err != nil {
		t.Fatalf("PriorityReadyList: %v", err)
	}

	if len(ready) < 3 {
		t.Fatalf("expected at least 3 ready tickets, got %d", len(ready))
	}

	if ready[0].ID != "T-HIGH" {
		t.Errorf("first ticket = %s, want T-HIGH (priority 10)", ready[0].ID)
	}

	if ready[0].Priority < ready[1].Priority {
		t.Errorf("tickets not sorted by priority DESC: %d < %d", ready[0].Priority, ready[1].Priority)
	}
}

func TestPriorityReadyList_ExcludesBlockedTickets(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	s.CreateSprint(Sprint{ID: "S-BLK", Name: "Blocked"})
	s.CreateTicket(Ticket{ID: "T-DEP", SprintID: "S-BLK", Title: "Dep", Priority: 10, Status: StatusBacklog})
	s.CreateTicket(Ticket{ID: "T-BLOCKED", SprintID: "S-BLK", Title: "Blocked", Priority: 20, Status: StatusBacklog})

	s.AddDependency("T-BLOCKED", "T-DEP")

	ready, err := s.PriorityReadyList("S-BLK")
	if err != nil {
		t.Fatalf("PriorityReadyList: %v", err)
	}

	for _, ticket := range ready {
		if ticket.ID == "T-BLOCKED" {
			t.Error("T-BLOCKED should not appear in ready list (blocked by T-DEP)")
		}
	}
	if len(ready) != 1 || ready[0].ID != "T-DEP" {
		t.Errorf("expected only [T-DEP], got %v", ready)
	}
}

func TestSprintVelocity(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	s.CreateSprint(Sprint{ID: "S-VEL", Name: "Velocity"})
	s.CreateTicket(Ticket{ID: "T-V1", SprintID: "S-VEL", Title: "v1"})
	s.CreateTicket(Ticket{ID: "T-V2", SprintID: "S-VEL", Title: "v2"})
	s.CreateTicket(Ticket{ID: "T-V3", SprintID: "S-VEL", Title: "v3"})

	s.RegisterAgent(Agent{ID: "fast-agent", Surface: "test"})
	s.RegisterAgent(Agent{ID: "slow-agent", Surface: "test"})

	s.ClaimTicket("T-V1", "fast-agent")
	s.ClaimTicket("T-V2", "fast-agent")
	s.ClaimTicket("T-V3", "slow-agent")

	s.CompleteTicket("T-V1", "fast-agent", "done", "", "")
	s.CompleteTicket("T-V2", "fast-agent", "done", "", "")
	s.CompleteTicket("T-V3", "slow-agent", "done", "", "")

	vel, err := s.SprintVelocity("S-VEL")
	if err != nil {
		t.Fatalf("SprintVelocity: %v", err)
	}

	if len(vel) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(vel))
	}

	found := map[string]AgentVelocity{}
	for _, v := range vel {
		found[v.AgentID] = v
	}

	if fast, ok := found["fast-agent"]; !ok {
		t.Error("missing fast-agent velocity")
	} else if fast.TicketsDone != 2 {
		t.Errorf("fast-agent tickets_done = %d, want 2", fast.TicketsDone)
	}

	if slow, ok := found["slow-agent"]; !ok {
		t.Error("missing slow-agent velocity")
	} else if slow.TicketsDone != 1 {
		t.Errorf("slow-agent tickets_done = %d, want 1", slow.TicketsDone)
	}
}

func TestSprintBurndown(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	s.CreateSprint(Sprint{ID: "S-BURN", Name: "Burndown"})
	s.CreateTicket(Ticket{ID: "T-B1", SprintID: "S-BURN", Title: "b1"})
	s.CreateTicket(Ticket{ID: "T-B2", SprintID: "S-BURN", Title: "b2"})
	s.CreateTicket(Ticket{ID: "T-B3", SprintID: "S-BURN", Title: "b3"})

	s.RegisterAgent(Agent{ID: "burn-agent", Surface: "test"})

	s.ClaimTicket("T-B1", "burn-agent")
	s.CompleteTicket("T-B1", "burn-agent", "done", "", "")

	s.ClaimTicket("T-B2", "burn-agent")
	s.CompleteTicket("T-B2", "burn-agent", "done", "", "")

	points, err := s.SprintBurndown("S-BURN")
	if err != nil {
		t.Fatalf("SprintBurndown: %v", err)
	}

	if len(points) < 3 {
		t.Fatalf("expected at least 3 points (start + 2 completions), got %d", len(points))
	}

	if points[0].TicketsDone != 0 {
		t.Errorf("initial point tickets_done = %d, want 0", points[0].TicketsDone)
	}
	if points[0].RemainingCount != 3 {
		t.Errorf("initial remaining = %d, want 3", points[0].RemainingCount)
	}

	last := points[len(points)-1]
	if last.TicketsDone != 2 {
		t.Errorf("final tickets_done = %d, want 2", last.TicketsDone)
	}
	if last.RemainingCount != 1 {
		t.Errorf("final remaining = %d, want 1", last.RemainingCount)
	}

	for i := 1; i < len(points); i++ {
		if points[i].TicketsDone < points[i-1].TicketsDone {
			t.Errorf("burndown not monotonically increasing: point %d = %d < point %d = %d",
				i, points[i].TicketsDone, i-1, points[i-1].TicketsDone)
		}
	}
}

func TestSprintVelocity_EmptySprint(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	s.CreateSprint(Sprint{ID: "S-EMPTY", Name: "Empty"})

	vel, err := s.SprintVelocity("S-EMPTY")
	if err != nil {
		t.Fatalf("SprintVelocity: %v", err)
	}
	if len(vel) != 0 {
		t.Errorf("expected 0 velocities, got %d", len(vel))
	}
}

func TestSprintBurndown_NoCompletions(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	s.CreateSprint(Sprint{ID: "S-NOCOMP", Name: "No Completions"})
	s.CreateTicket(Ticket{ID: "T-NC1", SprintID: "S-NOCOMP", Title: "nc1"})

	points, err := s.SprintBurndown("S-NOCOMP")
	if err != nil {
		t.Fatalf("SprintBurndown: %v", err)
	}
	if len(points) != 1 {
		t.Fatalf("expected 1 point (start only), got %d", len(points))
	}
	if points[0].RemainingCount != 1 {
		t.Errorf("remaining = %d, want 1", points[0].RemainingCount)
	}
}
