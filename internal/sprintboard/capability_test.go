package sprintboard

import "testing"

func TestSuggestAgent_ExactMatch(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	s.RegisterAgent(Agent{ID: "a1", Surface: "cursor", Capabilities: "go,testing"})
	s.RegisterAgent(Agent{ID: "a2", Surface: "codex", Capabilities: "go,typescript"})
	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "task", AcceptanceCriteria: "go,testing"})

	matches, err := s.SuggestAgent("T1")
	if err != nil {
		t.Fatalf("SuggestAgent: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected at least one match")
	}
	if matches[0].AgentID != "a1" {
		t.Errorf("best match = %q, want a1 (has go+testing)", matches[0].AgentID)
	}
	if matches[0].MatchScore != 1.0 {
		t.Errorf("match score = %f, want 1.0", matches[0].MatchScore)
	}
}

func TestSuggestAgent_PartialMatch(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	s.RegisterAgent(Agent{ID: "a1", Surface: "cursor", Capabilities: "go"})
	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "task", AcceptanceCriteria: "go,flutter"})

	matches, _ := s.SuggestAgent("T1")
	if len(matches) == 0 {
		t.Fatal("expected matches")
	}
	if matches[0].MatchScore != 0.5 {
		t.Errorf("score = %f, want 0.5 (1/2 caps)", matches[0].MatchScore)
	}
	if len(matches[0].MissingCaps) != 1 || matches[0].MissingCaps[0] != "flutter" {
		t.Errorf("missing = %v, want [flutter]", matches[0].MissingCaps)
	}
}

func TestSuggestAgent_NoRequirements(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	s.RegisterAgent(Agent{ID: "a1", Surface: "cursor"})
	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "task"})

	matches, _ := s.SuggestAgent("T1")
	if len(matches) != 1 {
		t.Errorf("got %d matches, want 1 (all agents match when no reqs)", len(matches))
	}
}

func TestSuggestAgent_RankedByScore(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	s.RegisterAgent(Agent{ID: "a1", Surface: "cursor", Capabilities: "go"})
	s.RegisterAgent(Agent{ID: "a2", Surface: "codex", Capabilities: "go,testing,rust"})
	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "task", AcceptanceCriteria: "go,testing"})

	matches, _ := s.SuggestAgent("T1")
	if matches[0].AgentID != "a2" {
		t.Errorf("first = %q, want a2 (2/2 match vs 1/2)", matches[0].AgentID)
	}
}

func TestCapabilityGapReport(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	s.RegisterAgent(Agent{ID: "a1", Surface: "cursor", Capabilities: "go,testing"})
	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "go task", AcceptanceCriteria: "go"})
	s.CreateTicket(Ticket{ID: "T2", SprintID: "S1", Title: "flutter task", AcceptanceCriteria: "flutter"})

	gaps, err := s.CapabilityGapReport("S1")
	if err != nil {
		t.Fatalf("CapabilityGapReport: %v", err)
	}
	if len(gaps) != 1 {
		t.Fatalf("got %d gaps, want 1 (flutter missing)", len(gaps))
	}
	if gaps[0].TicketID != "T2" {
		t.Errorf("gap ticket = %q, want T2", gaps[0].TicketID)
	}
	if gaps[0].MissingCaps[0] != "flutter" {
		t.Errorf("missing = %v, want [flutter]", gaps[0].MissingCaps)
	}
}

func TestCapabilityGapReport_NoGaps(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	s.RegisterAgent(Agent{ID: "a1", Surface: "cursor", Capabilities: "go,testing"})
	s.CreateSprint(Sprint{ID: "S1", Name: "test"})
	s.CreateTicket(Ticket{ID: "T1", SprintID: "S1", Title: "go task", AcceptanceCriteria: "go"})

	gaps, _ := s.CapabilityGapReport("S1")
	if len(gaps) != 0 {
		t.Errorf("got %d gaps, want 0", len(gaps))
	}
}

func TestParseCaps(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"go,testing,rust", 3},
		{"go", 1},
		{"", 0},
		{" go , testing ", 2},
	}
	for _, tc := range tests {
		got := parseCaps(tc.input)
		if len(got) != tc.want {
			t.Errorf("parseCaps(%q) = %d caps, want %d", tc.input, len(got), tc.want)
		}
	}
}
