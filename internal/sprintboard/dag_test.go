package sprintboard

import "testing"

func TestAddDependency(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "first"})
	s.CreateTicket(Ticket{ID: "T2", SprintID: "S1", Title: "second"})

	err := s.AddDependency("T2", "T1")
	if err != nil {
		t.Fatalf("AddDependency: %v", err)
	}

	blockers, err := s.BlockedBy("T2")
	if err != nil {
		t.Fatalf("BlockedBy: %v", err)
	}
	if len(blockers) != 1 || blockers[0] != "T1" {
		t.Errorf("blockers = %v, want [T1]", blockers)
	}
}

func TestAddDependency_SelfLoop(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "task"})

	err := s.AddDependency("T1", "T1")
	if err == nil {
		t.Fatal("expected error for self-loop")
	}
}

func TestAddDependency_CycleDetection(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "first"})
	s.CreateTicket(Ticket{ID: "T2", SprintID: "S1", Title: "second"})
	s.CreateTicket(Ticket{ID: "T3", SprintID: "S1", Title: "third"})

	s.AddDependency("T2", "T1")
	s.AddDependency("T3", "T2")

	err := s.AddDependency("T1", "T3")
	if err == nil {
		t.Fatal("expected error for cycle T1->T3->T2->T1")
	}
}

func TestBlockedBy_NoneWhenDepDone(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "first", Status: StatusDone})
	s.CreateTicket(Ticket{ID: "T2", SprintID: "S1", Title: "second"})

	s.AddDependency("T2", "T1")

	blockers, _ := s.BlockedBy("T2")
	if len(blockers) != 0 {
		t.Errorf("expected 0 blockers (T1 is done), got %v", blockers)
	}
}

func TestReadyTickets(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "first", Status: StatusBacklog})
	s.CreateTicket(Ticket{ID: "T2", SprintID: "S1", Title: "second", Status: StatusBacklog})
	s.CreateTicket(Ticket{ID: "T3", SprintID: "S1", Title: "blocked", Status: StatusBacklog})

	s.AddDependency("T3", "T1")

	ready, err := s.ReadyTickets("S1")
	if err != nil {
		t.Fatalf("ReadyTickets: %v", err)
	}

	readyIDs := make(map[string]bool)
	for _, ticket := range ready {
		readyIDs[ticket.ID] = true
	}

	if !readyIDs["T1"] {
		t.Error("T1 should be ready (no deps)")
	}
	if !readyIDs["T2"] {
		t.Error("T2 should be ready (no deps)")
	}
	if readyIDs["T3"] {
		t.Error("T3 should NOT be ready (blocked by T1)")
	}
}

func TestTopologicalSort(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "first"})
	s.CreateTicket(Ticket{ID: "T2", SprintID: "S1", Title: "second"})
	s.CreateTicket(Ticket{ID: "T3", SprintID: "S1", Title: "third"})

	s.AddDependency("T2", "T1")
	s.AddDependency("T3", "T2")

	sorted, err := s.TopologicalSort("S1")
	if err != nil {
		t.Fatalf("TopologicalSort: %v", err)
	}

	if len(sorted) != 3 {
		t.Fatalf("sorted = %d items, want 3", len(sorted))
	}

	pos := make(map[string]int)
	for i, id := range sorted {
		pos[id] = i
	}

	if pos["T1"] > pos["T2"] {
		t.Error("T1 must come before T2")
	}
	if pos["T2"] > pos["T3"] {
		t.Error("T2 must come before T3")
	}
}

func TestTopologicalSort_NoDeps(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "a"})
	s.CreateTicket(Ticket{ID: "T2", SprintID: "S1", Title: "b"})

	sorted, err := s.TopologicalSort("S1")
	if err != nil {
		t.Fatalf("TopologicalSort: %v", err)
	}
	if len(sorted) != 2 {
		t.Errorf("sorted = %d, want 2", len(sorted))
	}
}
