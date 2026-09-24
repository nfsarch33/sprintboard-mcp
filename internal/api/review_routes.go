package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

// handleTicketReview is the claimant's "my change is up for review" verb: the
// ticket moves in_progress -> review and keeps its claim, so a change that
// waits on a reviewer or a merge queue is not released by the stale-claim
// sweeper for being idle. Re-submitting a ticket already in review under the
// same claim is a 200 (a new revision of the same change).
func (s *Server) handleTicketReview(w http.ResponseWriter, r *http.Request) {
	defer drainAndClose(r)
	id := r.PathValue("id")
	var req struct {
		AgentID string `json:"agent_id"`
		Note    string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.AgentID == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("agent_id is required"))
		return
	}
	if err := s.store.SubmitForReview(id, req.AgentID, req.Note); err != nil {
		if errors.Is(err, sprintboard.ErrTicketNotClaimedBy) {
			writeErr(w, http.StatusConflict, err)
			return
		}
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	s.metrics.IncTicketsSubmittedForReview()
	writeJSON(w, http.StatusOK, map[string]any{
		"success":    true,
		"ticket_id":  id,
		"status":     sprintboard.StatusReview,
		"claimed_by": req.AgentID,
	})
}

// handleTicketRework is the reviewer's "changes requested" verb: review ->
// in_progress for the same claimant, with a fresh lease. It needs an actor and
// a reason, like resolve, because it is recorded against the ticket.
func (s *Server) handleTicketRework(w http.ResponseWriter, r *http.Request) {
	defer drainAndClose(r)
	id := r.PathValue("id")
	var req struct {
		Actor  string `json:"actor"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.Actor == "" || strings.TrimSpace(req.Reason) == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("actor and reason are required"))
		return
	}
	if err := s.store.ReturnToWork(id, req.Actor, req.Reason); err != nil {
		if errors.Is(err, sprintboard.ErrTicketNotInReview) {
			writeErr(w, http.StatusConflict, err)
			return
		}
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	s.metrics.IncTicketsReturnedToWork()
	writeJSON(w, http.StatusOK, map[string]any{
		"success":   true,
		"ticket_id": id,
		"status":    sprintboard.StatusInProgress,
	})
}
