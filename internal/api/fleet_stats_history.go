package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

func (s *Server) handleFleetStatsHistory(w http.ResponseWriter, r *http.Request) {
	kind := sprintboard.FleetHistoryKind(strings.TrimSpace(r.URL.Query().Get("kind")))
	if kind == "" {
		kind = sprintboard.FleetHistoryAll
	}
	host := strings.TrimSpace(r.URL.Query().Get("host"))
	days := 7
	if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid days: %q", raw))
			return
		}
		days = n
	}

	items, err := s.store.ListFleetStatsHistory(kind, host, days)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"kind":  kind,
		"host":  host,
		"days":  days,
		"count": len(items),
		"items": items,
	})
}

func (s *Server) handleFleetPROutcomeCreate(w http.ResponseWriter, r *http.Request) {
	defer drainAndClose(r)

	var req struct {
		Host          string          `json:"host"`
		Repo          string          `json:"repo"`
		PRNumber      int             `json:"pr_number"`
		Outcome       string          `json:"outcome"`
		Verdict       string          `json:"verdict"`
		ReviewerAgent string          `json:"reviewer_agent"`
		MergeSHA      string          `json:"merge_sha"`
		RecordedAt    string          `json:"recorded_at"`
		Payload       json.RawMessage `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.Host == "" || req.Repo == "" || req.Outcome == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("host, repo, and outcome are required"))
		return
	}
	if req.PRNumber <= 0 {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("pr_number must be positive"))
		return
	}
	if len(req.Payload) == 0 {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("payload is required"))
		return
	}

	recordedAt, err := parseOptionalRFC3339(req.RecordedAt)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid recorded_at: %w", err))
		return
	}
	if recordedAt.IsZero() {
		recordedAt = time.Now().UTC()
	}

	id, err := s.store.InsertFleetPROutcome(sprintboard.FleetPROutcome{
		Host:          req.Host,
		Repo:          req.Repo,
		PRNumber:      req.PRNumber,
		Outcome:       req.Outcome,
		Verdict:       req.Verdict,
		ReviewerAgent: req.ReviewerAgent,
		MergeSHA:      req.MergeSHA,
		RecordedAt:    recordedAt,
		PayloadJSON:   req.Payload,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "created"})
}
