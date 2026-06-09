package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

func parseOptionalRFC3339(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, raw)
}

func (s *Server) handleTerminalSessionEventCreate(w http.ResponseWriter, r *http.Request) {
	defer drainAndClose(r)

	var req struct {
		Host         string          `json:"host"`
		SessionID    string          `json:"session_id"`
		CommandClass string          `json:"command_class"`
		ExitCode     *int            `json:"exit_code"`
		DurationMs   int64           `json:"duration_ms"`
		Status       string          `json:"status"`
		Payload      json.RawMessage `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.Host == "" || req.SessionID == "" || req.CommandClass == "" || req.Status == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("host, session_id, command_class, and status are required"))
		return
	}
	if len(req.Payload) == 0 {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("payload is required"))
		return
	}

	id, err := s.store.InsertTerminalSessionEvent(sprintboard.TerminalSessionEvent{
		Host:         req.Host,
		SessionID:    req.SessionID,
		CommandClass: req.CommandClass,
		ExitCode:     req.ExitCode,
		DurationMs:   req.DurationMs,
		Status:       req.Status,
		PayloadJSON:  req.Payload,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "created"})
}

func (s *Server) handleEvalRunSnapshotCreate(w http.ResponseWriter, r *http.Request) {
	defer drainAndClose(r)

	var req struct {
		Host       string          `json:"host"`
		EvalRunID  string          `json:"eval_run_id"`
		Suite      string          `json:"suite"`
		Model      string          `json:"model"`
		Score      float64         `json:"score"`
		PassCount  int             `json:"pass_count"`
		FailCount  int             `json:"fail_count"`
		DurationMs int64           `json:"duration_ms"`
		RecordedAt string          `json:"recorded_at"`
		Payload    json.RawMessage `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.Host == "" || req.EvalRunID == "" || req.Suite == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("host, eval_run_id, and suite are required"))
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

	id, err := s.store.InsertEvalRunSnapshot(sprintboard.EvalRunSnapshot{
		Host:        req.Host,
		EvalRunID:   req.EvalRunID,
		Suite:       req.Suite,
		Model:       req.Model,
		Score:       req.Score,
		PassCount:   req.PassCount,
		FailCount:   req.FailCount,
		DurationMs:  req.DurationMs,
		RecordedAt:  recordedAt,
		PayloadJSON: req.Payload,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "created"})
}
