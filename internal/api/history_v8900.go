package api

import (
	"net/http"
	"strings"

	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

// handleSprintHistory implements GET /api/v1/sprints (v8900-B17). It returns
// every sprint sorted newest-first; optional `?status=closed` narrows the
// result. The shape mirrors handleAgentList for consistency.
func (s *Server) handleSprintHistory(w http.ResponseWriter, r *http.Request) {
	wantStatus := strings.TrimSpace(r.URL.Query().Get("status"))

	sprints, err := s.store.ListSprints()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	if wantStatus != "" {
		filtered := sprints[:0]
		for _, sp := range sprints {
			if string(sp.Status) == wantStatus {
				filtered = append(filtered, sp)
			}
		}
		sprints = filtered
	}
	if sprints == nil {
		sprints = []sprintboard.Sprint{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sprints": sprints,
		"count":   len(sprints),
	})
}
