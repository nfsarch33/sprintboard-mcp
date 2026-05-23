package api

import (
	"fmt"
	"net/http"

	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

// SprintMetrics is the aggregate v8900-B18 response: a single REST call that
// returns summary, SLA samples, velocity rollup, and burndown for one sprint.
// Consumers (agentcore CLI, fleet-agent dashboards) avoid round-tripping four
// endpoints to assemble a sprint dashboard.
type SprintMetrics struct {
	Sprint    sprintboard.Sprint                     `json:"sprint"`
	ByStatus  map[sprintboard.TicketStatus]int       `json:"tickets_by_status"`
	Total     int                                    `json:"total_tickets"`
	SLAs      []sprintboard.SLA                      `json:"slas"`
	Velocity  []sprintboard.AgentVelocity            `json:"velocity"`
	Burndown  []sprintboard.BurndownPoint            `json:"burndown"`
}

func (s *Server) handleSprintMetrics(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("id is required"))
		return
	}

	summary, err := s.store.SprintSummary(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}

	slas, err := s.store.SprintSLAs(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if slas == nil {
		slas = []sprintboard.SLA{}
	}

	velocity, err := s.store.SprintVelocity(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if velocity == nil {
		velocity = []sprintboard.AgentVelocity{}
	}

	burndown, err := s.store.SprintBurndown(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if burndown == nil {
		burndown = []sprintboard.BurndownPoint{}
	}

	writeJSON(w, http.StatusOK, SprintMetrics{
		Sprint:   summary.Sprint,
		ByStatus: summary.TicketsByStatus,
		Total:    summary.TotalTickets,
		SLAs:     slas,
		Velocity: velocity,
		Burndown: burndown,
	})
}
