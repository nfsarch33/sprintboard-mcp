package temporal_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
	spbtemporal "github.com/nfsarch33/sprintboard-mcp/internal/temporal"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
)

func newTestStore(t *testing.T) *sprintboard.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := sprintboard.NewStore(dbPath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func seedSprint(t *testing.T, s *sprintboard.Store, sprintID string, ticketIDs []string) {
	t.Helper()
	if err := s.CreateSprint(sprintboard.Sprint{ID: sprintID, Name: "Test Sprint"}); err != nil {
		t.Fatalf("create sprint: %v", err)
	}
	for _, id := range ticketIDs {
		if err := s.CreateTicket(sprintboard.Ticket{ID: id, Title: "Ticket " + id, SprintID: sprintID}); err != nil {
			t.Fatalf("create ticket %s: %v", id, err)
		}
	}
}

func registerActivities(env *testsuite.TestWorkflowEnvironment, acts *spbtemporal.Activities) {
	env.RegisterActivityWithOptions(acts.ActivateSprint, activity.RegisterOptions{Name: spbtemporal.ActivateSprint})
	env.RegisterActivityWithOptions(acts.CloseSprint, activity.RegisterOptions{Name: spbtemporal.CloseSprint})
	env.RegisterActivityWithOptions(acts.ClaimTicketActivity, activity.RegisterOptions{Name: spbtemporal.ClaimTicketActivity})
	env.RegisterActivityWithOptions(acts.CompleteTicketActivity, activity.RegisterOptions{Name: spbtemporal.CompleteTicketActivity})
	env.RegisterActivityWithOptions(acts.HeartbeatActivity, activity.RegisterOptions{Name: spbtemporal.HeartbeatActivity})
	env.RegisterActivityWithOptions(acts.HandoffActivity, activity.RegisterOptions{Name: spbtemporal.HandoffActivity})
}

func TestSprintWorkflow_AllTicketsComplete(t *testing.T) {
	store := newTestStore(t)
	tickets := []string{"T-1", "T-2"}
	seedSprint(t, store, "sp-1", tickets)

	if err := store.RegisterAgent(sprintboard.Agent{ID: "agent-A", Surface: "test"}); err != nil {
		t.Fatalf("register agent: %v", err)
	}

	acts := &spbtemporal.Activities{Store: store}

	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(spbtemporal.SprintWorkflow)
	env.RegisterWorkflow(spbtemporal.TicketWorkflow)
	registerActivities(env, acts)

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflowByID("T-1", "ticket-complete", "tests pass")
	}, spbtemporal.TicketHeartbeatPeriod+1)

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflowByID("T-2", "ticket-complete", "build green")
	}, spbtemporal.TicketHeartbeatPeriod+2)

	input := spbtemporal.SprintInput{
		SprintID: "sp-1",
		Name:     "Test Sprint",
		Tickets:  tickets,
		Agents:   []string{"agent-A"},
	}
	env.ExecuteWorkflow(spbtemporal.SprintWorkflow, input)

	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}

	var result spbtemporal.SprintResult
	if err := env.GetWorkflowResult(&result); err != nil {
		t.Fatalf("get result: %v", err)
	}
	if result.TicketsDone != 2 {
		t.Errorf("tickets_done = %d, want 2", result.TicketsDone)
	}
	if result.TicketsTotal != 2 {
		t.Errorf("tickets_total = %d, want 2", result.TicketsTotal)
	}
}

func TestTicketWorkflow_Timeout(t *testing.T) {
	store := newTestStore(t)
	seedSprint(t, store, "sp-timeout", []string{"T-TIMEOUT"})

	if err := store.RegisterAgent(sprintboard.Agent{ID: "agent-slow", Surface: "test"}); err != nil {
		t.Fatalf("register agent: %v", err)
	}

	acts := &spbtemporal.Activities{Store: store}

	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(spbtemporal.TicketWorkflow)
	registerActivities(env, acts)

	input := spbtemporal.TicketInput{
		TicketID: "T-TIMEOUT",
		SprintID: "sp-timeout",
		AgentID:  "agent-slow",
	}
	env.ExecuteWorkflow(spbtemporal.TicketWorkflow, input)

	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}

	var result spbtemporal.TicketResult
	if err := env.GetWorkflowResult(&result); err != nil {
		t.Fatalf("get result: %v", err)
	}
	if result.Status != "timed_out" {
		t.Errorf("status = %q, want timed_out", result.Status)
	}
}

func TestTicketWorkflow_SignalComplete(t *testing.T) {
	store := newTestStore(t)
	seedSprint(t, store, "sp-signal", []string{"T-SIG"})

	if err := store.RegisterAgent(sprintboard.Agent{ID: "agent-fast", Surface: "test"}); err != nil {
		t.Fatalf("register agent: %v", err)
	}

	acts := &spbtemporal.Activities{Store: store}

	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(spbtemporal.TicketWorkflow)
	registerActivities(env, acts)

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow("ticket-complete", "go test -race ./... GREEN")
	}, spbtemporal.TicketHeartbeatPeriod/2)

	input := spbtemporal.TicketInput{
		TicketID: "T-SIG",
		SprintID: "sp-signal",
		AgentID:  "agent-fast",
	}
	env.ExecuteWorkflow(spbtemporal.TicketWorkflow, input)

	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}

	var result spbtemporal.TicketResult
	if err := env.GetWorkflowResult(&result); err != nil {
		t.Fatalf("get result: %v", err)
	}
	if result.Status != "done" {
		t.Errorf("status = %q, want done", result.Status)
	}
	if result.Evidence != "go test -race ./... GREEN" {
		t.Errorf("evidence = %q, want 'go test -race ./... GREEN'", result.Evidence)
	}
}

func TestActivities_ClaimAndComplete(t *testing.T) {
	store := newTestStore(t)
	seedSprint(t, store, "sp-act", []string{"T-ACT"})

	if err := store.RegisterAgent(sprintboard.Agent{ID: "agent-act", Surface: "test"}); err != nil {
		t.Fatalf("register agent: %v", err)
	}

	acts := &spbtemporal.Activities{Store: store}

	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivityWithOptions(acts.ClaimTicketActivity, activity.RegisterOptions{Name: spbtemporal.ClaimTicketActivity})
	env.RegisterActivityWithOptions(acts.CompleteTicketActivity, activity.RegisterOptions{Name: spbtemporal.CompleteTicketActivity})

	val, err := env.ExecuteActivity(acts.ClaimTicketActivity, "T-ACT", "agent-act")
	if err != nil {
		t.Fatalf("claim activity: %v", err)
	}
	var claimed bool
	if err := val.Get(&claimed); err != nil {
		t.Fatalf("get claim result: %v", err)
	}
	if !claimed {
		t.Error("expected claim to succeed")
	}

	_, err = env.ExecuteActivity(acts.CompleteTicketActivity, "T-ACT", "agent-act", "evidence-data")
	if err != nil {
		t.Fatalf("complete activity: %v", err)
	}

	ticket, err := store.GetTicket("T-ACT")
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if ticket.Status != sprintboard.StatusDone {
		t.Errorf("ticket status = %q, want done", ticket.Status)
	}
}

func TestTicketWorkflow_Cancellation(t *testing.T) {
	store := newTestStore(t)
	seedSprint(t, store, "sp-cancel", []string{"T-CANCEL"})

	if err := store.RegisterAgent(sprintboard.Agent{ID: "agent-cancel", Surface: "test"}); err != nil {
		t.Fatalf("register agent: %v", err)
	}

	acts := &spbtemporal.Activities{Store: store}

	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(spbtemporal.TicketWorkflow)
	registerActivities(env, acts)

	env.RegisterDelayedCallback(func() {
		env.CancelWorkflow()
	}, spbtemporal.TicketHeartbeatPeriod/4)

	input := spbtemporal.TicketInput{
		TicketID: "T-CANCEL",
		SprintID: "sp-cancel",
		AgentID:  "agent-cancel",
	}
	env.ExecuteWorkflow(spbtemporal.TicketWorkflow, input)

	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete after cancellation")
	}
	err := env.GetWorkflowError()
	if err == nil {
		var result spbtemporal.TicketResult
		env.GetWorkflowResult(&result)
		if result.Status == "done" {
			t.Error("cancelled workflow should not have status done")
		}
	}
}

func TestActivities_ClaimRetryOnTransientFailure(t *testing.T) {
	store := newTestStore(t)
	seedSprint(t, store, "sp-retry", []string{"T-RETRY"})

	if err := store.RegisterAgent(sprintboard.Agent{ID: "agent-retry", Surface: "test"}); err != nil {
		t.Fatalf("register agent: %v", err)
	}

	acts := &spbtemporal.Activities{Store: store}

	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivityWithOptions(acts.ClaimTicketActivity, activity.RegisterOptions{Name: spbtemporal.ClaimTicketActivity})
	env.RegisterActivityWithOptions(acts.CompleteTicketActivity, activity.RegisterOptions{Name: spbtemporal.CompleteTicketActivity})

	val, err := env.ExecuteActivity(acts.ClaimTicketActivity, "T-RETRY", "agent-retry")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	var claimed bool
	val.Get(&claimed)
	if !claimed {
		t.Error("expected initial claim to succeed")
	}

	val2, err := env.ExecuteActivity(acts.ClaimTicketActivity, "T-RETRY", "agent-conflict")
	if err != nil {
		t.Fatalf("conflicting claim: %v", err)
	}
	var claimed2 bool
	val2.Get(&claimed2)
	if claimed2 {
		t.Error("conflicting claim should return false")
	}

	_, err = env.ExecuteActivity(acts.CompleteTicketActivity, "T-RETRY", "agent-retry", "retry-evidence")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	ticket, _ := store.GetTicket("T-RETRY")
	if ticket.Status != sprintboard.StatusDone {
		t.Errorf("ticket status = %q, want done", ticket.Status)
	}
}

func TestSprintWorkflow_MultipleTicketsStaggered(t *testing.T) {
	store := newTestStore(t)
	tickets := []string{"T-M1", "T-M2", "T-M3", "T-M4"}
	seedSprint(t, store, "sp-multi", tickets)

	agents := []string{"agent-X", "agent-Y"}
	for _, a := range agents {
		if err := store.RegisterAgent(sprintboard.Agent{ID: a, Surface: "test"}); err != nil {
			t.Fatalf("register agent %s: %v", a, err)
		}
	}

	acts := &spbtemporal.Activities{Store: store}

	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(spbtemporal.SprintWorkflow)
	env.RegisterWorkflow(spbtemporal.TicketWorkflow)
	registerActivities(env, acts)

	for i, tid := range tickets {
		ticketID := tid
		delay := spbtemporal.TicketHeartbeatPeriod + time.Duration(i+1)
		env.RegisterDelayedCallback(func() {
			env.SignalWorkflowByID(ticketID, "ticket-complete", "staggered-evidence-"+ticketID)
		}, delay)
	}

	input := spbtemporal.SprintInput{
		SprintID: "sp-multi",
		Name:     "Multi-Ticket Sprint",
		Tickets:  tickets,
		Agents:   agents,
	}
	env.ExecuteWorkflow(spbtemporal.SprintWorkflow, input)

	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}

	var result spbtemporal.SprintResult
	if err := env.GetWorkflowResult(&result); err != nil {
		t.Fatalf("get result: %v", err)
	}
	if result.TicketsDone != 4 {
		t.Errorf("tickets_done = %d, want 4", result.TicketsDone)
	}
	if result.TicketsTotal != 4 {
		t.Errorf("tickets_total = %d, want 4", result.TicketsTotal)
	}
	if result.TicketsFailed != 0 {
		t.Errorf("tickets_failed = %d, want 0", result.TicketsFailed)
	}
}
