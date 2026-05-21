package temporal

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	TaskQueue             = "sprintboard"
	TicketHeartbeatPeriod = 5 * time.Minute
	TicketStaleTimeout    = 30 * time.Minute
)

type SprintInput struct {
	SprintID string   `json:"sprint_id"`
	Name     string   `json:"name"`
	Theme    string   `json:"theme,omitempty"`
	Tickets  []string `json:"tickets"`
	Agents   []string `json:"agents"`
}

type SprintResult struct {
	SprintID       string `json:"sprint_id"`
	TicketsDone    int    `json:"tickets_done"`
	TicketsFailed  int    `json:"tickets_failed"`
	TicketsTotal   int    `json:"tickets_total"`
	DurationMillis int64  `json:"duration_ms"`
}

type TicketInput struct {
	TicketID string `json:"ticket_id"`
	SprintID string `json:"sprint_id"`
	AgentID  string `json:"agent_id,omitempty"`
}

type TicketResult struct {
	TicketID string `json:"ticket_id"`
	Status   string `json:"status"`
	AgentID  string `json:"agent_id"`
	Evidence string `json:"evidence,omitempty"`
}

// SprintWorkflow orchestrates the lifecycle of a sprint:
// create tickets, distribute to agents, monitor until all done or failed.
func SprintWorkflow(ctx workflow.Context, input SprintInput) (SprintResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("SprintWorkflow started", "sprint_id", input.SprintID, "tickets", len(input.Tickets))

	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	if err := workflow.ExecuteActivity(ctx, ActivateSprint, input.SprintID).Get(ctx, nil); err != nil {
		return SprintResult{}, fmt.Errorf("activate sprint: %w", err)
	}

	var futures []workflow.ChildWorkflowFuture
	agentIdx := 0
	for _, ticketID := range input.Tickets {
		childCtx := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
			WorkflowID: ticketID,
			TaskQueue:  TaskQueue,
		})
		agent := ""
		if len(input.Agents) > 0 {
			agent = input.Agents[agentIdx%len(input.Agents)]
			agentIdx++
		}
		ti := TicketInput{
			TicketID: ticketID,
			SprintID: input.SprintID,
			AgentID:  agent,
		}
		f := workflow.ExecuteChildWorkflow(childCtx, TicketWorkflow, ti)
		futures = append(futures, f)
	}

	var done, failed int
	for _, f := range futures {
		var result TicketResult
		if err := f.Get(ctx, &result); err != nil {
			logger.Warn("ticket workflow failed", "error", err)
			failed++
		} else if result.Status == "done" {
			done++
		} else {
			failed++
		}
	}

	if err := workflow.ExecuteActivity(ctx, CloseSprint, input.SprintID).Get(ctx, nil); err != nil {
		logger.Warn("close sprint failed (non-fatal)", "error", err)
	}

	return SprintResult{
		SprintID:      input.SprintID,
		TicketsDone:   done,
		TicketsFailed: failed,
		TicketsTotal:  len(input.Tickets),
	}, nil
}

// TicketWorkflow manages a single ticket's lifecycle:
// claim -> heartbeat monitor -> complete or timeout-and-reassign.
func TicketWorkflow(ctx workflow.Context, input TicketInput) (TicketResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("TicketWorkflow started", "ticket_id", input.TicketID)

	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	if input.AgentID != "" {
		var claimed bool
		if err := workflow.ExecuteActivity(ctx, ClaimTicketActivity, input.TicketID, input.AgentID).Get(ctx, &claimed); err != nil {
			return TicketResult{TicketID: input.TicketID, Status: "failed"}, fmt.Errorf("claim: %w", err)
		}
		if !claimed {
			return TicketResult{TicketID: input.TicketID, Status: "conflict", AgentID: input.AgentID}, nil
		}
	}

	completeCh := workflow.GetSignalChannel(ctx, "ticket-complete")

	timerCtx, timerCancel := workflow.WithCancel(ctx)
	defer timerCancel()

	timerFuture := workflow.NewTimer(timerCtx, TicketStaleTimeout)

	heartbeatCtx, heartbeatCancel := workflow.WithCancel(ctx)
	defer heartbeatCancel()

	workflow.Go(heartbeatCtx, func(gCtx workflow.Context) {
		for {
			if gCtx.Err() != nil {
				return
			}
			_ = workflow.Sleep(gCtx, TicketHeartbeatPeriod)
			if gCtx.Err() != nil {
				return
			}
			_ = workflow.ExecuteActivity(gCtx, HeartbeatActivity, input.TicketID, input.AgentID).Get(gCtx, nil)
		}
	})

	s := workflow.NewSelector(ctx)

	var result TicketResult
	result.TicketID = input.TicketID
	result.AgentID = input.AgentID

	s.AddReceive(completeCh, func(c workflow.ReceiveChannel, more bool) {
		var evidence string
		c.Receive(ctx, &evidence)
		result.Status = "done"
		result.Evidence = evidence
	})

	s.AddFuture(timerFuture, func(f workflow.Future) {
		result.Status = "timed_out"
	})

	s.Select(ctx)

	heartbeatCancel()
	timerCancel()

	if result.Status == "done" {
		_ = workflow.ExecuteActivity(ctx, CompleteTicketActivity, input.TicketID, input.AgentID, result.Evidence).Get(ctx, nil)

		if input.SprintID != "" {
			h := HandoffInput{
				TicketID:  input.TicketID,
				FromAgent: input.AgentID,
				ToAgent:   "cursor-parent",
				Summary:   fmt.Sprintf("ticket %s completed with evidence: %s", input.TicketID, result.Evidence),
			}
			_ = workflow.ExecuteActivity(ctx, HandoffActivity, h).Get(ctx, nil)
		}
	}

	return result, nil
}
