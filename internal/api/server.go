package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

// drainAndClose drains any unread bytes on the request body and closes it.
// net/http requires the body to be drained before the connection can be
// returned to the keep-alive pool; otherwise the underlying TCP socket
// stays half-closed in CLOSE_WAIT.
func drainAndClose(r *http.Request) {
	if r == nil || r.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, r.Body)
	_ = r.Body.Close()
}

type Server struct {
	store    *sprintboard.Store
	logger   *slog.Logger
	mux      *http.ServeMux
	shutting atomic.Bool
}

func NewServer(store *sprintboard.Store, logger *slog.Logger) *Server {
	s := &Server{store: store, logger: logger}
	s.mux = http.NewServeMux()
	s.routes()
	return s
}

// Handler returns the configured middleware chain. We deliberately do NOT
// wrap with http.TimeoutHandler here: the SQLite store uses SetMaxOpenConns(1)
// for single-writer correctness, and TimeoutHandler's detached goroutine
// model can leak DB connection ownership when the timer fires before the
// inner handler releases the conn -- which cascades into permanent queue
// starvation for every subsequent request. Per-request bounding is delegated
// to client-side timeouts (helixon-agent's http.Client.Timeout=10s) and the
// SQLite busy_timeout=5000 pragma. The CLOSE_WAIT regression that motivated
// timeouts is solved by drainAndClose on every body-decoding handler, which
// allows net/http to recycle the keep-alive connection cleanly.
func (s *Server) Handler() http.Handler {
	return s.withMiddleware(s.mux)
}

// SetShuttingDown marks the server as shutting down; /readyz returns 503.
func (s *Server) SetShuttingDown() { s.shutting.Store(true) }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("GET /readyz", s.handleReadyz)
	s.mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	s.mux.HandleFunc("POST /api/v1/sprints", s.handleSprintCreate)
	s.mux.HandleFunc("GET /api/v1/sprints/{id}", s.handleSprintStatus)
	s.mux.HandleFunc("POST /api/v1/sprints/{id}/close", s.handleSprintClose)
	s.mux.HandleFunc("POST /api/v1/tickets", s.handleTicketCreate)
	s.mux.HandleFunc("GET /api/v1/tickets/{id}", s.handleTicketGet)
	s.mux.HandleFunc("GET /api/v1/sprints/{id}/tickets", s.handleSprintTicketList)
	s.mux.HandleFunc("GET /api/v1/sprints/{id}/slas", s.handleSprintSLAs)
	s.mux.HandleFunc("POST /api/v1/tickets/{id}/claim", s.handleTicketClaim)
	s.mux.HandleFunc("POST /api/v1/tickets/{id}/complete", s.handleTicketComplete)
	s.mux.HandleFunc("GET /api/v1/agents", s.handleAgentList)
	s.mux.HandleFunc("POST /api/v1/agents", s.handleAgentRegister)
	s.mux.HandleFunc("POST /api/v1/handoffs", s.handleHandoffPublish)
	s.mux.HandleFunc("POST /api/v1/tickets/{id}/comments", s.handleTicketCommentAdd)
	s.mux.HandleFunc("GET /api/v1/tickets/{id}/comments", s.handleTicketCommentList)
	// T-8800-B13: sprint templates
	s.mux.HandleFunc("POST /api/v1/templates", s.handleTemplateCreate)
	s.mux.HandleFunc("GET /api/v1/templates", s.handleTemplateList)
	s.mux.HandleFunc("DELETE /api/v1/templates/{id}", s.handleTemplateDelete)
	s.mux.HandleFunc("POST /api/v1/templates/{id}/instantiate", s.handleTemplateInstantiate)
	// T-8800-B14: agent workload
	s.mux.HandleFunc("GET /api/v1/agents/workload", s.handleAgentWorkload)
	// T-8800-B15: sprint burndown
	s.mux.HandleFunc("GET /api/v1/sprints/{id}/burndown", s.handleSprintBurndown)
	// v8900-B16: ticket search
	s.mux.HandleFunc("GET /api/v1/tickets/search", s.handleTicketSearch)
	// v8900-B17: sprint history
	s.mux.HandleFunc("GET /api/v1/sprints", s.handleSprintHistory)
	// v8900-B18: sprint metrics rollup
	s.mux.HandleFunc("GET /api/v1/sprints/{id}/metrics", s.handleSprintMetrics)
	// fleet report persistence (ADR-073 / Melbourne Phase 2 C4)
	s.mux.HandleFunc("POST /api/v1/fleet-reports/snapshots", s.handleFleetReportSnapshotCreate)
	// fleet stats persistence (Melbourne Phase 3 Part B / fleet-stats-extend)
	s.mux.HandleFunc("POST /api/v1/terminal-sessions/events", s.handleTerminalSessionEventCreate)
	s.mux.HandleFunc("POST /api/v1/eval-runs/snapshots", s.handleEvalRunSnapshotCreate)
	// Phase 5: unified history + PR outcomes (pr-review poll wire target)
	s.mux.HandleFunc("GET /api/v1/fleet-stats/history", s.handleFleetStatsHistory)
	s.mux.HandleFunc("POST /api/v1/fleet-pr-outcomes", s.handleFleetPROutcomeCreate)
}

// T-8800-B13: sprint templates ---------------------------------------------

func (s *Server) handleTemplateCreate(w http.ResponseWriter, r *http.Request) {
	defer drainAndClose(r)
	var tmpl sprintboard.SprintTemplate
	if err := json.NewDecoder(r.Body).Decode(&tmpl); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if tmpl.ID == "" || tmpl.Name == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("id and name are required"))
		return
	}
	if err := s.store.CreateSprintTemplate(tmpl); err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": tmpl.ID, "status": "created"})
}

func (s *Server) handleTemplateList(w http.ResponseWriter, _ *http.Request) {
	tmpls, err := s.store.ListSprintTemplates()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if tmpls == nil {
		tmpls = []sprintboard.SprintTemplate{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"templates": tmpls})
}

func (s *Server) handleTemplateDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.DeleteSprintTemplate(id); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "deleted"})
}

func (s *Server) handleTemplateInstantiate(w http.ResponseWriter, r *http.Request) {
	defer drainAndClose(r)
	id := r.PathValue("id")
	var req struct {
		Sprint sprintboard.Sprint `json:"sprint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.Sprint.ID == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("sprint.id is required"))
		return
	}
	inst, err := s.store.InstantiateSprintFromTemplate(id, req.Sprint)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, inst)
}

// T-8800-B14: agent workload ------------------------------------------------

func (s *Server) handleAgentWorkload(w http.ResponseWriter, r *http.Request) {
	sprintID := r.URL.Query().Get("sprint_id")
	wl, err := s.store.AgentWorkload(sprintID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if wl == nil {
		wl = []sprintboard.AgentWorkloadEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"sprint_id": sprintID, "workload": wl})
}

// T-8800-B15: sprint burndown -----------------------------------------------

func (s *Server) handleSprintBurndown(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	pts, err := s.store.SprintBurndown(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	if pts == nil {
		pts = []sprintboard.BurndownPoint{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"sprint_id": id, "points": pts})
}

func (s *Server) handleTicketCommentAdd(w http.ResponseWriter, r *http.Request) {
	defer drainAndClose(r)
	id := r.PathValue("id")
	var req struct {
		Author string `json:"author"`
		Body   string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.Author == "" || req.Body == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("author and body are required"))
		return
	}
	c, err := s.store.AddTicketComment(id, req.Author, req.Body)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) handleTicketCommentList(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	comments, err := s.store.ListTicketComments(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if comments == nil {
		comments = []sprintboard.TicketComment{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ticket_id": id, "comments": comments})
}

func (s *Server) withMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		w.Header().Set("Content-Type", "application/json")
		h.ServeHTTP(w, r)
		s.logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	if err := s.store.Ping(); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	if s.shutting.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "shutting_down"})
		return
	}
	if err := s.store.Ping(); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "sprintboard-api",
		"version": "1.0.0",
	})
}

func (s *Server) handleSprintCreate(w http.ResponseWriter, r *http.Request) {
	defer drainAndClose(r)
	var req struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Theme  string `json:"theme,omitempty"`
		Status string `json:"status,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.ID == "" || req.Name == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("id and name are required"))
		return
	}
	sp := sprintboard.Sprint{
		ID:    req.ID,
		Name:  req.Name,
		Theme: req.Theme,
	}
	if req.Status != "" {
		sp.Status = sprintboard.SprintStatus(req.Status)
	}
	if err := s.store.CreateSprint(sp); err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": req.ID, "status": "created"})
}

func (s *Server) handleSprintStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	summary, err := s.store.SprintSummary(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) handleSprintClose(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.UpdateSprint(id, sprintboard.SprintClosed); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "closed"})
}

func (s *Server) handleTicketCreate(w http.ResponseWriter, r *http.Request) {
	defer drainAndClose(r)
	var req struct {
		ID          string   `json:"id"`
		Title       string   `json:"title"`
		SprintID    string   `json:"sprint_id"`
		Description string   `json:"description,omitempty"`
		Priority    int      `json:"priority,omitempty"`
		DueDate     string   `json:"due_date,omitempty"`
		Labels      []string `json:"labels,omitempty"`
		Status      string   `json:"status,omitempty"`
		OwnerAgent  string   `json:"owner_agent,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.ID == "" || req.Title == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("id and title are required"))
		return
	}
	t := sprintboard.Ticket{
		ID:          req.ID,
		Title:       req.Title,
		SprintID:    req.SprintID,
		Description: req.Description,
		Priority:    req.Priority,
		Labels:      req.Labels,
		OwnerAgent:  req.OwnerAgent,
	}
	if req.Status != "" {
		t.Status = sprintboard.TicketStatus(req.Status)
	}
	if req.DueDate != "" {
		due, err := time.Parse(time.RFC3339, req.DueDate)
		if err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("due_date must be RFC3339: %w", err))
			return
		}
		t.DueDate = due
	}
	if err := s.store.CreateTicket(t); err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": req.ID, "status": "created"})
}

func (s *Server) handleTicketGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, err := s.store.GetTicket(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleSprintTicketList(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tickets, err := s.store.ListTickets(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if tickets == nil {
		tickets = []sprintboard.Ticket{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"sprint_id": id, "tickets": tickets})
}

func (s *Server) handleSprintSLAs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	slas, err := s.store.SprintSLAs(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if slas == nil {
		slas = []sprintboard.SLA{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"sprint_id": id, "slas": slas})
}

func (s *Server) handleTicketClaim(w http.ResponseWriter, r *http.Request) {
	defer drainAndClose(r)
	id := r.PathValue("id")
	var req struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	agentID := req.AgentID
	if agentID == "" {
		agentID = "anonymous"
	}
	result, err := s.store.ClaimTicket(id, agentID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	status := http.StatusOK
	if !result.Success {
		status = http.StatusConflict
	}
	writeJSON(w, status, result)
}

func (s *Server) handleTicketComplete(w http.ResponseWriter, r *http.Request) {
	defer drainAndClose(r)
	id := r.PathValue("id")
	var req struct {
		AgentID  string `json:"agent_id"`
		Evidence string `json:"evidence"`
		Branch   string `json:"branch"`
		PRURL    string `json:"pr_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.CompleteTicket(id, req.AgentID, req.Evidence, req.Branch, req.PRURL); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "done"})
}

func (s *Server) handleAgentList(w http.ResponseWriter, _ *http.Request) {
	agents, err := s.store.ListActiveAgents()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"agents": agents})
}

func (s *Server) handleAgentRegister(w http.ResponseWriter, r *http.Request) {
	defer drainAndClose(r)
	var req struct {
		AgentID      string `json:"agent_id"`
		Surface      string `json:"surface"`
		Capabilities string `json:"capabilities"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.AgentID == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("agent_id is required"))
		return
	}
	a := sprintboard.Agent{
		ID:           req.AgentID,
		Surface:      req.Surface,
		Capabilities: req.Capabilities,
	}
	if a.Surface == "" {
		a.Surface = "api"
	}
	if err := s.store.RegisterAgent(a); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"agent_id": req.AgentID, "status": "registered"})
}

func (s *Server) handleHandoffPublish(w http.ResponseWriter, r *http.Request) {
	defer drainAndClose(r)
	var req struct {
		TicketID  string `json:"ticket_id"`
		FromAgent string `json:"from_agent"`
		ToAgent   string `json:"to_agent"`
		Summary   string `json:"summary"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.TicketID == "" || req.ToAgent == "" || req.Summary == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("ticket_id, to_agent, and summary are required"))
		return
	}
	h := sprintboard.CoordinationHandoff{
		TicketID:  req.TicketID,
		FromAgent: req.FromAgent,
		ToAgent:   req.ToAgent,
		Summary:   req.Summary,
	}
	id, err := s.store.PublishHandoff(h)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"handoff_id": id, "status": "published"})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	msg := err.Error()
	if i := strings.Index(msg, ":"); i > 0 && i < 40 {
		msg = strings.TrimSpace(msg[i+1:])
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
