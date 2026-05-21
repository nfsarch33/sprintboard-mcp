package temporal

import (
	"context"
	"fmt"

	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

type Activities struct {
	Store *sprintboard.Store
}

type HandoffInput struct {
	TicketID  string `json:"ticket_id"`
	FromAgent string `json:"from_agent"`
	ToAgent   string `json:"to_agent"`
	Summary   string `json:"summary"`
}

func (a *Activities) ActivateSprint(_ context.Context, sprintID string) error {
	return a.Store.UpdateSprint(sprintID, sprintboard.SprintActive)
}

func (a *Activities) CloseSprint(_ context.Context, sprintID string) error {
	return a.Store.UpdateSprint(sprintID, sprintboard.SprintClosed)
}

func (a *Activities) ClaimTicketActivity(_ context.Context, ticketID, agentID string) (bool, error) {
	result, err := a.Store.ClaimTicket(ticketID, agentID)
	if err != nil {
		return false, fmt.Errorf("claim ticket %q: %w", ticketID, err)
	}
	return result.Success, nil
}

func (a *Activities) CompleteTicketActivity(_ context.Context, ticketID, agentID, evidence string) error {
	return a.Store.CompleteTicket(ticketID, agentID, evidence)
}

func (a *Activities) HeartbeatActivity(_ context.Context, ticketID, agentID string) error {
	return a.Store.AgentHeartbeat(agentID, ticketID)
}

func (a *Activities) HandoffActivity(_ context.Context, input HandoffInput) error {
	h := sprintboard.CoordinationHandoff{
		TicketID:  input.TicketID,
		FromAgent: input.FromAgent,
		ToAgent:   input.ToAgent,
		Summary:   input.Summary,
	}
	_, err := a.Store.PublishHandoff(h)
	return err
}

// Free functions used as activity references in workflow registration.
// They delegate to the Activities struct methods when executed by the worker.
var (
	ActivateSprint        = "ActivateSprint"
	CloseSprint           = "CloseSprint"
	ClaimTicketActivity   = "ClaimTicketActivity"
	CompleteTicketActivity = "CompleteTicketActivity"
	HeartbeatActivity     = "HeartbeatActivity"
	HandoffActivity       = "HandoffActivity"
)
