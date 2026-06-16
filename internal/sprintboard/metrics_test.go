package sprintboard

import (
	"bytes"
	"strings"
	"testing"
)

func TestMetricsWritePrometheus(t *testing.T) {
	m := NewMetrics()
	m.IncTicketsCreated()
	m.IncTicketsCreated()
	m.IncTicketsClaimed()
	m.IncTicketsCompleted()
	m.IncAgentsRegistered()
	m.IncHandoffsPublished()
	m.IncCommentsAdded()

	var buf bytes.Buffer
	if err := m.WritePrometheus(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	out := buf.String()

	wants := []string{
		"sprintboard_tickets_created_total 2",
		"sprintboard_tickets_claimed_total 1",
		"sprintboard_tickets_completed_total 1",
		"sprintboard_agents_registered_total 1",
		"sprintboard_handoffs_published_total 1",
		"sprintboard_comments_added_total 1",
		"# TYPE sprintboard_tickets_created_total counter",
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("output missing %q\noutput:\n%s", w, out)
		}
	}
}

func TestMetricsEmpty(t *testing.T) {
	m := NewMetrics()
	var buf bytes.Buffer
	if err := m.WritePrometheus(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	out := buf.String()
	// Even with all counters at 0, we should emit at least one
	// metric line for each kind.
	if !strings.Contains(out, "sprintboard_tickets_created_total 0") {
		t.Errorf("empty metrics missing zero counter: %q", out)
	}
}

func TestMetricsDeterministicOrder(t *testing.T) {
	// Write twice and ensure byte-for-byte equality (counters are
	// the same; ordering is stable).
	m := NewMetrics()
	m.IncTicketsCreated()
	var b1, b2 bytes.Buffer
	_ = m.WritePrometheus(&b1)
	_ = m.WritePrometheus(&b2)
	if b1.String() != b2.String() {
		t.Errorf("non-deterministic output:\nb1=%q\nb2=%q", b1.String(), b2.String())
	}
}
