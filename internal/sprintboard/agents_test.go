package sprintboard

import (
	"testing"
	"time"
)

func TestRegisterAgent_NewAgent(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	err := s.RegisterAgent(Agent{
		ID:      "cursor-parent",
		Surface: "cursor",
	})
	if err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}

	a, err := s.GetAgent("cursor-parent")
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if a.Surface != "cursor" {
		t.Errorf("surface = %q, want cursor", a.Surface)
	}
	if a.LastSeen.IsZero() {
		t.Error("last_seen should be set")
	}
}

func TestRegisterAgent_RequiresID(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	err := s.RegisterAgent(Agent{Surface: "cursor"})
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestRegisterAgent_Upsert(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	s.RegisterAgent(Agent{ID: "a1", Surface: "cursor"})
	s.RegisterAgent(Agent{ID: "a1", Surface: "claude-code", Capabilities: "go,rust"})

	a, _ := s.GetAgent("a1")
	if a.Surface != "claude-code" {
		t.Errorf("surface = %q after upsert, want claude-code", a.Surface)
	}
	if a.Capabilities != "go,rust" {
		t.Errorf("capabilities = %q, want go,rust", a.Capabilities)
	}
}

func TestAgentHeartbeat_UpdatesLastSeen(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	s.RegisterAgent(Agent{ID: "a1", Surface: "cursor"})

	time.Sleep(1100 * time.Millisecond)
	err := s.AgentHeartbeat("a1", "T-001")
	if err != nil {
		t.Fatalf("AgentHeartbeat: %v", err)
	}

	after, _ := s.GetAgent("a1")
	if after.CurrentTicketID != "T-001" {
		t.Errorf("current_ticket = %q, want T-001", after.CurrentTicketID)
	}
}

func TestAgentHeartbeat_UnregisteredFails(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	err := s.AgentHeartbeat("ghost", "")
	if err == nil {
		t.Fatal("expected error for unregistered agent")
	}
}

func TestListActiveAgents(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	s.RegisterAgent(Agent{ID: "a1", Surface: "cursor"})
	s.RegisterAgent(Agent{ID: "a2", Surface: "codex"})

	agents, err := s.ListActiveAgents()
	if err != nil {
		t.Fatalf("ListActiveAgents: %v", err)
	}
	if len(agents) != 2 {
		t.Errorf("got %d agents, want 2", len(agents))
	}
}

func TestUpdatePreferences(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	s.RegisterAgent(Agent{ID: "a1", Surface: "cursor"})

	prefs := map[string]string{"indent": "tabs", "line_length": "100"}
	err := s.UpdatePreferences("a1", prefs)
	if err != nil {
		t.Fatalf("UpdatePreferences: %v", err)
	}

	got, err := s.GetPreferences("a1")
	if err != nil {
		t.Fatalf("GetPreferences: %v", err)
	}
	if got["indent"] != "tabs" {
		t.Errorf("indent = %q, want tabs", got["indent"])
	}
	if got["line_length"] != "100" {
		t.Errorf("line_length = %q, want 100", got["line_length"])
	}
}

func TestUpdatePreferences_UnregisteredFails(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	err := s.UpdatePreferences("ghost", map[string]string{"k": "v"})
	if err == nil {
		t.Fatal("expected error for unregistered agent")
	}
}

func TestGetPreferences_Empty(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	s.RegisterAgent(Agent{ID: "a1", Surface: "cursor"})
	prefs, err := s.GetPreferences("a1")
	if err != nil {
		t.Fatalf("GetPreferences: %v", err)
	}
	if len(prefs) != 0 {
		t.Errorf("expected empty prefs, got %v", prefs)
	}
}

func TestAutoRegisterThenList(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	err := s.RegisterAgent(Agent{ID: "cursor-parent", Surface: "cursor"})
	if err != nil {
		t.Fatalf("auto-register: %v", err)
	}

	agents, err := s.ListActiveAgents()
	if err != nil {
		t.Fatalf("ListActiveAgents: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent after auto-register, got %d", len(agents))
	}
	if agents[0].ID != "cursor-parent" {
		t.Errorf("agent id = %q, want cursor-parent", agents[0].ID)
	}
	if agents[0].Surface != "cursor" {
		t.Errorf("surface = %q, want cursor", agents[0].Surface)
	}
}

func TestAutoRegisterIdempotent(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	for i := 0; i < 5; i++ {
		err := s.RegisterAgent(Agent{ID: "cursor-parent", Surface: "cursor"})
		if err != nil {
			t.Fatalf("register attempt %d: %v", i, err)
		}
	}

	agents, err := s.ListActiveAgents()
	if err != nil {
		t.Fatalf("ListActiveAgents: %v", err)
	}
	if len(agents) != 1 {
		t.Errorf("expected 1 agent after 5 registers, got %d", len(agents))
	}
}

func TestExpireStaleAgents_30MinWindow(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	s.RegisterAgent(Agent{ID: "a1", Surface: "cursor"})

	staleTime := time.Now().Add(-31 * time.Minute)
	s.db.Exec(`UPDATE agents SET last_seen = ? WHERE id = ?`,
		formatTime(staleTime), "a1")

	expired, err := s.ExpireStaleAgents()
	if err != nil {
		t.Fatalf("ExpireStaleAgents: %v", err)
	}
	if expired != 1 {
		t.Errorf("expired = %d, want 1", expired)
	}

	agents, _ := s.ListActiveAgents()
	if len(agents) != 0 {
		t.Errorf("should have 0 active agents after expiry, got %d", len(agents))
	}
}
