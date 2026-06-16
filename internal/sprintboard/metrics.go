// Package metrics — minimal Prometheus text-format counters for
// sprintboard-api (v13910 S5.1).
//
// We deliberately do NOT depend on github.com/prometheus/client_golang
// to keep the binary small and the build hermetic. The counters are
// simple atomic uint64s protected by atomic.AddUint64. The /metrics
// handler emits them in Prometheus text format.
package sprintboard

import (
	"fmt"
	"io"
	"sort"
	"sync"
	"sync/atomic"
)

// Metrics holds the per-process counters. Create one per Store/Server.
type Metrics struct {
	TicketsCreated     atomic.Uint64
	TicketsClaimed     atomic.Uint64
	TicketsCompleted   atomic.Uint64
	AgentsRegistered   atomic.Uint64
	HandoffsPublished  atomic.Uint64
	CommentsAdded      atomic.Uint64
}

// NewMetrics returns a zero-initialised Metrics.
func NewMetrics() *Metrics { return &Metrics{} }

// IncTicketsCreated is a convenience wrapper for the most common call.
func (m *Metrics) IncTicketsCreated() { m.TicketsCreated.Add(1) }
func (m *Metrics) IncTicketsClaimed() { m.TicketsClaimed.Add(1) }
func (m *Metrics) IncTicketsCompleted() { m.TicketsCompleted.Add(1) }
func (m *Metrics) IncAgentsRegistered() { m.AgentsRegistered.Add(1) }
func (m *Metrics) IncHandoffsPublished() { m.HandoffsPublished.Add(1) }
func (m *Metrics) IncCommentsAdded() { m.CommentsAdded.Add(1) }

// WritePrometheus emits the metrics in Prometheus text format. Counter
// names use the "sprintboard_" prefix per the Prometheus naming
// convention. The HELP and TYPE lines are emitted once per metric.
func (m *Metrics) WritePrometheus(w io.Writer) error {
	type spec struct {
		name  string
		help  string
		value uint64
	}
	specs := []spec{
		{"sprintboard_tickets_created_total", "Tickets created since process start.", m.TicketsCreated.Load()},
		{"sprintboard_tickets_claimed_total", "Tickets claimed since process start.", m.TicketsClaimed.Load()},
		{"sprintboard_tickets_completed_total", "Tickets completed since process start.", m.TicketsCompleted.Load()},
		{"sprintboard_agents_registered_total", "Agents registered since process start.", m.AgentsRegistered.Load()},
		{"sprintboard_handoffs_published_total", "Handoffs published since process start.", m.HandoffsPublished.Load()},
		{"sprintboard_comments_added_total", "Ticket comments added since process start.", m.CommentsAdded.Load()},
	}
	// Stable order: sort by name for deterministic output (matters for
	// git diffs of the /metrics endpoint).
	sort.Slice(specs, func(i, j int) bool { return specs[i].name < specs[j].name })

	for _, s := range specs {
		if _, err := fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n%s %d\n",
			s.name, s.help, s.name, s.name, s.value); err != nil {
			return err
		}
	}
	return nil
}

// guard against unused import warnings.
var _ = sync.Mutex{}
