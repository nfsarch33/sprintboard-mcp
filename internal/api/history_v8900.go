package api

import (
	"net/http"
	"strings"

	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

// handleSprintHistory implements GET /api/v1/sprints (v8900-B17). It returns
// every sprint sorted newest-first; optional `?status=closed` narrows the
// result. The shape mirrors handleAgentList for consistency.
//
// v18685-2: optional `?tenant_id=...` query parameter narrows results to a
// single tenant. When absent (or empty), all sprints are returned (backward
// compatibility with v8900-B17).
func (s *Server) handleSprintHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	wantStatus := strings.TrimSpace(q.Get("status"))
	wantTenant := strings.TrimSpace(q.Get("tenant_id"))

	var sprints []sprintboard.Sprint
	var err error
	if wantTenant != "" {
		// Use tenant-scoped query when caller asks for a specific tenant.
		// Empty/no-filter callers still get all sprints for backward-compat.
		sprints, err = s.store.ListSprintsByTenant(wantTenant)
	} else {
		sprints, err = s.store.ListSprints()
	}
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
