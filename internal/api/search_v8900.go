package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

// handleTicketSearch implements GET /api/v1/tickets/search (v8900-B16).
//
// Query parameters (all optional):
//
//	q             free-text fragment matched against title, description, acceptance
//	status        exact-match TicketStatus
//	owner         owner_agent
//	sprint_id     restrict to a single sprint
//	priority_min  integer floor for priority (>=)
//	label         repeatable; tickets matching ANY label are returned
//	limit         positive integer cap
//
// The handler returns 200 with `{"tickets":[...], "count":N}`.
func (s *Server) handleTicketSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := sprintboard.TicketFilter{
		Query:    strings.TrimSpace(q.Get("q")),
		Status:   sprintboard.TicketStatus(strings.TrimSpace(q.Get("status"))),
		Owner:    strings.TrimSpace(q.Get("owner")),
		SprintID: strings.TrimSpace(q.Get("sprint_id")),
		Labels:   q["label"],
	}
	if v := strings.TrimSpace(q.Get("priority_min")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.PriorityMin = n
		}
	}
	if v := strings.TrimSpace(q.Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			filter.Limit = n
		}
	}

	tickets, err := s.store.SearchTickets(filter)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if tickets == nil {
		tickets = []sprintboard.Ticket{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tickets": tickets,
		"count":   len(tickets),
	})
}
