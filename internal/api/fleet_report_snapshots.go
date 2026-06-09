package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

func (s *Server) handleFleetReportSnapshotCreate(w http.ResponseWriter, r *http.Request) {
	defer drainAndClose(r)

	var req struct {
		Host        string          `json:"host"`
		ReportKind  string          `json:"report_kind"`
		WindowStart string          `json:"window_start"`
		WindowEnd   string          `json:"window_end"`
		Payload     json.RawMessage `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.Host == "" || req.ReportKind == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("host and report_kind are required"))
		return
	}
	if len(req.Payload) == 0 {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("payload is required"))
		return
	}

	windowStart, err := time.Parse(time.RFC3339, req.WindowStart)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid window_start: %w", err))
		return
	}
	windowEnd, err := time.Parse(time.RFC3339, req.WindowEnd)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid window_end: %w", err))
		return
	}

	id, err := s.store.InsertFleetReportSnapshot(sprintboard.FleetReportSnapshot{
		Host:        req.Host,
		ReportKind:  req.ReportKind,
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
		PayloadJSON: req.Payload,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "created"})
}
