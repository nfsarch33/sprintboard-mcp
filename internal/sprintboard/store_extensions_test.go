package sprintboard

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"
)

// v7800-B3: Mini-jira extensions: due_date, labels, claimed_at, completed_at.

func TestCreateTicket_RoundTripsDueDateAndLabels(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "v7800", Name: "Track B"})

	due := time.Now().Add(48 * time.Hour).Truncate(time.Second)
	if err := s.CreateTicket(Ticket{
		ID:       "T-DUE-1",
		SprintID: "v7800",
		Title:    "Ticket with due date and labels",
		Status:   StatusReady,
		DueDate:  due,
		Labels:   []string{"P0", "infra", "agentrace"},
	}); err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}

	got, err := s.GetTicket("T-DUE-1")
	if err != nil {
		t.Fatalf("GetTicket: %v", err)
	}
	if !got.DueDate.Equal(due) {
		t.Errorf("DueDate = %v, want %v", got.DueDate, due)
	}
	if len(got.Labels) != 3 || got.Labels[0] != "P0" || got.Labels[2] != "agentrace" {
		t.Errorf("Labels = %v, want [P0 infra agentrace]", got.Labels)
	}
}

func TestCreateTicket_NullDueDateAndEmptyLabels(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "v7800", Name: "Track B"})
	if err := s.CreateTicket(Ticket{
		ID:       "T-NULL",
		SprintID: "v7800",
		Title:    "No due date",
		Status:   StatusBacklog,
	}); err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	got, err := s.GetTicket("T-NULL")
	if err != nil {
		t.Fatalf("GetTicket: %v", err)
	}
	if !got.DueDate.IsZero() {
		t.Errorf("expected zero DueDate, got %v", got.DueDate)
	}
	if len(got.Labels) != 0 {
		t.Errorf("expected empty Labels, got %v", got.Labels)
	}
}

func TestListTickets_PreservesDueDateAndLabels(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "v7800", Name: "Track B"})
	due := time.Now().Add(24 * time.Hour).Truncate(time.Second)
	s.CreateTicket(Ticket{
		ID: "T-LIST", SprintID: "v7800", Title: "L",
		Status: StatusReady, DueDate: due, Labels: []string{"x", "y"},
	})

	tickets, err := s.ListTickets("v7800")
	if err != nil {
		t.Fatalf("ListTickets: %v", err)
	}
	if len(tickets) != 1 {
		t.Fatalf("got %d tickets, want 1", len(tickets))
	}
	if !tickets[0].DueDate.Equal(due) {
		t.Errorf("DueDate = %v, want %v", tickets[0].DueDate, due)
	}
	if len(tickets[0].Labels) != 2 {
		t.Errorf("Labels = %v, want [x y]", tickets[0].Labels)
	}
}

func TestClaimTicket_SetsClaimedAtTimestamp(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "v7800", Name: "x"})
	s.CreateTicket(Ticket{ID: "T-CL", SprintID: "v7800", Title: "claim me", Status: StatusReady})

	before := time.Now().Add(-2 * time.Second)
	if _, err := s.ClaimTicket("T-CL", "claude-code"); err != nil {
		t.Fatalf("ClaimTicket: %v", err)
	}
	got, _ := s.GetTicket("T-CL")
	if got.ClaimedBy != "claude-code" {
		t.Errorf("ClaimedBy = %q, want claude-code", got.ClaimedBy)
	}
	if got.ClaimedAt.Before(before) || got.ClaimedAt.After(time.Now().Add(2*time.Second)) {
		t.Errorf("ClaimedAt = %v, expected near now", got.ClaimedAt)
	}
}

func TestCompleteTicket_SetsCompletedAt(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "v7800", Name: "x"})
	s.CreateTicket(Ticket{ID: "T-COMP", SprintID: "v7800", Title: "complete me", Status: StatusReady})
	s.ClaimTicket("T-COMP", "claude-code")

	before := time.Now().Add(-2 * time.Second)
	if err := s.CompleteTicket("T-COMP", "claude-code", "evidence-sha:abc", "", ""); err != nil {
		t.Fatalf("CompleteTicket: %v", err)
	}
	got, _ := s.GetTicket("T-COMP")
	if got.Status != StatusDone {
		t.Fatalf("status = %q, want done", got.Status)
	}
	if got.CompletedAt.Before(before) || got.CompletedAt.After(time.Now().Add(2*time.Second)) {
		t.Errorf("CompletedAt = %v, expected near now", got.CompletedAt)
	}
}

func TestUpdateTicket_ToDoneSetsCompletedAt(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "v7800", Name: "x"})
	s.CreateTicket(Ticket{ID: "T-UPD", SprintID: "v7800", Title: "u", Status: StatusInProgress})

	before := time.Now().Add(-2 * time.Second)
	if err := s.UpdateTicket("T-UPD", StatusDone, "claude-code", "manual transition"); err != nil {
		t.Fatalf("UpdateTicket: %v", err)
	}
	got, _ := s.GetTicket("T-UPD")
	if got.CompletedAt.Before(before) || got.CompletedAt.After(time.Now().Add(2*time.Second)) {
		t.Errorf("CompletedAt = %v, expected near now", got.CompletedAt)
	}
}

func TestUpdateTicket_NonDoneDoesNotSetCompletedAt(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "v7800", Name: "x"})
	s.CreateTicket(Ticket{ID: "T-UPD2", SprintID: "v7800", Title: "u", Status: StatusBacklog})

	if err := s.UpdateTicket("T-UPD2", StatusInProgress, "claude-code", "starting"); err != nil {
		t.Fatalf("UpdateTicket: %v", err)
	}
	got, _ := s.GetTicket("T-UPD2")
	if !got.CompletedAt.IsZero() {
		t.Errorf("CompletedAt = %v, expected zero for non-done transition", got.CompletedAt)
	}
}

func TestTicketJSON_MarshalsLabelsAsArrayAndOmitsEmptyTimestamps(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "v7800", Name: "x"})
	s.CreateTicket(Ticket{ID: "T-JSON", SprintID: "v7800", Title: "j", Status: StatusReady, Labels: []string{"alpha", "beta"}})

	got, _ := s.GetTicket("T-JSON")
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	out := string(raw)
	if !contains(out, `"labels":["alpha","beta"]`) {
		t.Errorf("expected labels array in JSON, got %s", out)
	}
	// completed_at should be omitted (zero value with omitempty)
	if contains(out, `"completed_at":"0001-01-01`) {
		t.Errorf("expected zero completed_at to be omitted, got %s", out)
	}
}

func TestSprintSLAs_ReportsTimeToClaimAndComplete(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "v7800", Name: "SLA"})

	created := time.Now().Add(-3 * time.Hour).Truncate(time.Second)
	if err := s.CreateTicket(Ticket{
		ID: "T-SLA", SprintID: "v7800", Title: "SLA me",
		Status: StatusReady, CreatedAt: created,
	}); err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}

	// Force claim & complete timestamps deterministically.
	claimedAt := created.Add(1 * time.Hour)
	completedAt := created.Add(2 * time.Hour)
	if _, err := s.db.Exec(
		`UPDATE tickets SET claimed_by = ?, claimed_at = ?, completed_at = ?, status = ?, updated_at = ? WHERE id = ?`,
		"claude-code", formatTime(claimedAt), formatTime(completedAt), StatusDone, formatTime(completedAt), "T-SLA",
	); err != nil {
		t.Fatalf("seed timestamps: %v", err)
	}

	slas, err := s.SprintSLAs("v7800")
	if err != nil {
		t.Fatalf("SprintSLAs: %v", err)
	}
	if len(slas) != 1 {
		t.Fatalf("got %d SLA rows, want 1", len(slas))
	}
	got := slas[0]
	if got.TicketID != "T-SLA" {
		t.Errorf("TicketID = %q", got.TicketID)
	}
	if got.TimeToClaim < 59*time.Minute || got.TimeToClaim > 61*time.Minute {
		t.Errorf("TimeToClaim = %v, want ~1h", got.TimeToClaim)
	}
	if got.TimeToComplete < 59*time.Minute || got.TimeToComplete > 61*time.Minute {
		t.Errorf("TimeToComplete = %v, want ~1h", got.TimeToComplete)
	}
}

func TestSprintSLAs_SkipsUnclaimed(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "v7800", Name: "x"})
	s.CreateTicket(Ticket{ID: "T-NOCLAIM", SprintID: "v7800", Title: "nope", Status: StatusReady})

	slas, err := s.SprintSLAs("v7800")
	if err != nil {
		t.Fatalf("SprintSLAs: %v", err)
	}
	if len(slas) != 0 {
		t.Fatalf("expected zero SLAs for unclaimed ticket, got %v", slas)
	}
}

func TestMigrate_IsIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := dir + "/sprint.db"
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open #1: %v", err)
	}
	s.Close()

	s2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open #2: %v", err)
	}
	defer s2.Close()
	// A reopen should not blow up; migrations must be ALTER-tolerant.
}

// v8000-B18: persist SLA durations as integer milliseconds on transition so
// downstream consumers (sprint-eval, mc dashboards, fleet-daily-report) can
// read them directly without recomputing from RFC3339 strings on every query.

// TestClaimTicket_PersistsTimeToClaimMS confirms ClaimTicket writes
// time_to_claim_ms = round(claimed_at - created_at) into the tickets row so a
// later SprintSLAs query returns it without recomputation.
func TestClaimTicket_PersistsTimeToClaimMS(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "v8000", Name: "B18"})

	created := time.Now().Add(-90 * time.Second).Truncate(time.Second)
	if err := s.CreateTicket(Ticket{
		ID: "T-8000-CLAIM-MS", SprintID: "v8000", Title: "claim ms",
		Status: StatusReady, CreatedAt: created,
	}); err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}

	if _, err := s.ClaimTicket("T-8000-CLAIM-MS", "claude-code"); err != nil {
		t.Fatalf("ClaimTicket: %v", err)
	}

	var ms sql.NullInt64
	if err := s.db.QueryRow(
		`SELECT time_to_claim_ms FROM tickets WHERE id = ?`, "T-8000-CLAIM-MS",
	).Scan(&ms); err != nil {
		t.Fatalf("query time_to_claim_ms: %v", err)
	}
	if !ms.Valid {
		t.Fatal("time_to_claim_ms is NULL after ClaimTicket; expected non-null")
	}
	// Created 90s ago, so claim duration must be at least 60s and well under 5min.
	if ms.Int64 < 60_000 || ms.Int64 > 300_000 {
		t.Errorf("time_to_claim_ms = %d, want 60_000..300_000", ms.Int64)
	}
}

// TestCompleteTicket_PersistsTimeToCompleteMS confirms CompleteTicket writes
// time_to_complete_ms = round(completed_at - claimed_at).
func TestCompleteTicket_PersistsTimeToCompleteMS(t *testing.T) {
	s := testStore(t)
	s.CreateSprint(Sprint{ID: "v8000", Name: "B18"})
	if err := s.CreateTicket(Ticket{
		ID: "T-8000-COMPLETE-MS", SprintID: "v8000", Title: "complete ms",
		Status: StatusReady,
	}); err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	if _, err := s.ClaimTicket("T-8000-COMPLETE-MS", "claude-code"); err != nil {
		t.Fatalf("ClaimTicket: %v", err)
	}

	// Force an older claimed_at so completion shows a measurable delta.
	claimedAt := time.Now().Add(-30 * time.Second).Truncate(time.Second)
	if _, err := s.db.Exec(
		`UPDATE tickets SET claimed_at = ? WHERE id = ?`,
		formatTime(claimedAt), "T-8000-COMPLETE-MS",
	); err != nil {
		t.Fatalf("rewrite claimed_at: %v", err)
	}

	if err := s.CompleteTicket("T-8000-COMPLETE-MS", "claude-code", "tests green", "", ""); err != nil {
		t.Fatalf("CompleteTicket: %v", err)
	}

	var ms sql.NullInt64
	if err := s.db.QueryRow(
		`SELECT time_to_complete_ms FROM tickets WHERE id = ?`, "T-8000-COMPLETE-MS",
	).Scan(&ms); err != nil {
		t.Fatalf("query time_to_complete_ms: %v", err)
	}
	if !ms.Valid {
		t.Fatal("time_to_complete_ms is NULL after CompleteTicket; expected non-null")
	}
	if ms.Int64 < 20_000 || ms.Int64 > 120_000 {
		t.Errorf("time_to_complete_ms = %d, want 20_000..120_000", ms.Int64)
	}
}

// TestSprintSLAs_ReturnsPersistedMSColumns seeds tickets directly so the
// reader path is exercised without wall-clock jitter:
//   - "T-PERSISTED" has both the legacy timestamps AND the new *_ms columns
//     populated, so SprintSLAs must surface the persisted ms values verbatim.
//   - "T-LEGACY" has only the legacy timestamps (NULL ms columns), modelling a
//     row written by a pre-B18 binary; the reader must derive the ms fields
//     from the duration so dashboards keep working during rolling restart.
func TestSprintSLAs_ReturnsPersistedMSColumns(t *testing.T) {
	s := testStore(t)
	if err := s.CreateSprint(Sprint{ID: "v8000", Name: "SLA ms"}); err != nil {
		t.Fatalf("CreateSprint: %v", err)
	}

	created := time.Now().Add(-3 * time.Hour).Truncate(time.Second)
	claimed := created.Add(1 * time.Hour)
	completed := created.Add(2 * time.Hour)

	insertTicket := func(id string, withMS bool) {
		t.Helper()
		if err := s.CreateTicket(Ticket{
			ID: id, SprintID: "v8000", Title: id,
			Status: StatusReady, CreatedAt: created,
		}); err != nil {
			t.Fatalf("CreateTicket %s: %v", id, err)
		}
		var (
			claimMS    interface{} = nil
			completeMS interface{} = nil
		)
		if withMS {
			claimMS = int64(7777)
			completeMS = int64(8888)
		}
		if _, err := s.db.Exec(
			`UPDATE tickets SET claimed_by = ?, claimed_at = ?, completed_at = ?,
			    status = ?, updated_at = ?, time_to_claim_ms = ?, time_to_complete_ms = ?
			 WHERE id = ?`,
			"claude-code", formatTime(claimed), formatTime(completed),
			StatusDone, formatTime(completed), claimMS, completeMS, id,
		); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	insertTicket("T-PERSISTED", true)
	insertTicket("T-LEGACY", false)

	slas, err := s.SprintSLAs("v8000")
	if err != nil {
		t.Fatalf("SprintSLAs: %v", err)
	}
	if len(slas) != 2 {
		t.Fatalf("got %d SLAs, want 2", len(slas))
	}
	by := map[string]SLA{}
	for _, sla := range slas {
		by[sla.TicketID] = sla
	}

	persisted := by["T-PERSISTED"]
	if persisted.TimeToClaimMS != 7777 {
		t.Errorf("persisted TimeToClaimMS = %d, want 7777 (must read column verbatim)", persisted.TimeToClaimMS)
	}
	if persisted.TimeToCompleteMS != 8888 {
		t.Errorf("persisted TimeToCompleteMS = %d, want 8888 (must read column verbatim)", persisted.TimeToCompleteMS)
	}

	legacy := by["T-LEGACY"]
	wantClaimMS := int64((1 * time.Hour) / time.Millisecond)
	wantCompleteMS := int64((1 * time.Hour) / time.Millisecond)
	if legacy.TimeToClaimMS != wantClaimMS {
		t.Errorf("legacy TimeToClaimMS = %d, want %d (must derive from duration)", legacy.TimeToClaimMS, wantClaimMS)
	}
	if legacy.TimeToCompleteMS != wantCompleteMS {
		t.Errorf("legacy TimeToCompleteMS = %d, want %d (must derive from duration)", legacy.TimeToCompleteMS, wantCompleteMS)
	}
}
