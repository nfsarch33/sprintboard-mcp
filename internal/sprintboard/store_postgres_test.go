package sprintboard

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// pgTestDSN returns the DSN for the PG smoke tests, or empty if not set.
// The smoke tests are skipped when SPRINTBOARD_TEST_PG_URL is unset so the
// suite still runs cleanly on machines without a local PostgreSQL.
func pgTestDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("SPRINTBOARD_TEST_PG_URL")
	if dsn == "" {
		t.Skip("SPRINTBOARD_TEST_PG_URL not set; skipping PG smoke test")
	}
	return dsn
}

// pgIsolatedDSN creates a fresh schema in the target PG database, sets
// search_path to it via DSN options, and returns a cleanup function that
// drops the schema.
func pgIsolatedDSN(t *testing.T, baseDSN string) (string, func()) {
	t.Helper()
	rawDB, err := sql.Open("pgx", baseDSN)
	if err != nil {
		t.Fatalf("open base pg: %v", err)
	}
	defer rawDB.Close()
	if err := rawDB.Ping(); err != nil {
		t.Skipf("PostgreSQL unreachable at %s: %v", redactDSN(baseDSN), err)
	}

	schema := fmt.Sprintf("sb_test_%d", os.Getpid())
	// Append a per-test suffix.
	schema += "_" + strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	if len(schema) > 60 {
		schema = schema[:60]
	}
	schema = strings.ToLower(schema)

	if _, err := rawDB.Exec(fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schema)); err != nil {
		t.Fatalf("pre-drop schema: %v", err)
	}
	if _, err := rawDB.Exec(fmt.Sprintf("CREATE SCHEMA %s", schema)); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	sep := "?"
	if strings.Contains(baseDSN, "?") {
		sep = "&"
	}
	isolatedDSN := baseDSN + sep + "search_path=" + schema

	cleanup := func() {
		cleanupDB, err := sql.Open("pgx", baseDSN)
		if err == nil {
			_, _ = cleanupDB.Exec(fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schema))
			cleanupDB.Close()
		}
	}
	return isolatedDSN, cleanup
}

func redactDSN(dsn string) string {
	if i := strings.Index(dsn, "@"); i > 0 {
		if j := strings.Index(dsn, "://"); j >= 0 && j < i {
			return dsn[:j+3] + "***" + dsn[i:]
		}
	}
	return dsn
}

func TestNewStore_PostgresFromEnv(t *testing.T) {
	baseDSN := pgTestDSN(t)
	dsn, cleanup := pgIsolatedDSN(t, baseDSN)
	defer cleanup()

	t.Setenv("SPRINTBOARD_DB_URL", dsn)
	store, err := NewStore("")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	if got := store.Dialect(); got != DialectPostgres {
		t.Fatalf("Dialect = %q, want %q", got, DialectPostgres)
	}
	if store.RawDB() == nil {
		t.Fatal("RawDB returned nil")
	}
	if err := store.Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestOpenPostgres_LifecycleSmoke(t *testing.T) {
	baseDSN := pgTestDSN(t)
	dsn, cleanup := pgIsolatedDSN(t, baseDSN)
	defer cleanup()

	store, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("OpenPostgres: %v", err)
	}
	defer store.Close()

	if store.Dialect() != DialectPostgres {
		t.Fatalf("Dialect = %q, want postgres", store.Dialect())
	}
	if err := store.Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	sprintID := "test-pg-" + t.Name()
	if err := store.CreateSprint(Sprint{ID: sprintID, Name: "PG Smoke", Status: SprintActive}); err != nil {
		t.Fatalf("CreateSprint: %v", err)
	}

	got, err := store.GetSprint(sprintID)
	if err != nil {
		t.Fatalf("GetSprint: %v", err)
	}
	if got.ID != sprintID || got.Name != "PG Smoke" {
		t.Fatalf("GetSprint round-trip: got %+v", got)
	}

	if err := store.CreateTicket(Ticket{
		ID:       "tkt-pg-1",
		SprintID: sprintID,
		Title:    "PG smoke ticket",
		Status:   StatusReady,
	}); err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}

	tickets, err := store.ListTickets(sprintID)
	if err != nil {
		t.Fatalf("ListTickets: %v", err)
	}
	if len(tickets) != 1 {
		t.Fatalf("ListTickets len = %d, want 1", len(tickets))
	}

	sprints, err := store.ListSprints()
	if err != nil {
		t.Fatalf("ListSprints: %v", err)
	}
	found := false
	for _, sp := range sprints {
		if sp.ID == sprintID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("ListSprints did not return %q", sprintID)
	}
}

func TestOpenPostgres_DialectRewrite(t *testing.T) {
	baseDSN := pgTestDSN(t)
	dsn, cleanup := pgIsolatedDSN(t, baseDSN)
	defer cleanup()

	store, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("OpenPostgres: %v", err)
	}
	defer store.Close()

	// Use ? placeholders; the dialectDB wrapper must rewrite to $1, $2... for PG.
	if _, err := store.db.Exec(
		"INSERT INTO sprints (id, name, status, created_at) VALUES (?, ?, ?, ?)",
		"rewrite-pg", "Rewrite", string(SprintActive), "2026-05-27T00:00:00Z",
	); err != nil {
		t.Fatalf("Exec via wrapper: %v", err)
	}

	row := store.db.QueryRow("SELECT name FROM sprints WHERE id = ?", "rewrite-pg")
	var name string
	if err := row.Scan(&name); err != nil {
		t.Fatalf("QueryRow: %v", err)
	}
	if name != "Rewrite" {
		t.Fatalf("name = %q, want Rewrite", name)
	}

	// Exercise Query (multi-row) wrapper as well.
	rows, err := store.db.Query("SELECT id FROM sprints WHERE id = ?", "rewrite-pg")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("rows.Scan: %v", err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	if count != 1 {
		t.Fatalf("Query returned %d rows, want 1", count)
	}

	// Exercise Begin/Commit/Rollback wrappers.
	tx, err := store.db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if _, err := tx.Exec(
		"INSERT INTO sprints (id, name, status, created_at) VALUES (?, ?, ?, ?)",
		"tx-pg", "Tx", string(SprintActive), "2026-05-27T00:00:00Z",
	); err != nil {
		_ = tx.Rollback()
		t.Fatalf("tx.Exec: %v", err)
	}
	var txName string
	if err := tx.QueryRow("SELECT name FROM sprints WHERE id = ?", "tx-pg").Scan(&txName); err != nil {
		_ = tx.Rollback()
		t.Fatalf("tx.QueryRow: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	tx2, err := store.db.Begin()
	if err != nil {
		t.Fatalf("Begin2: %v", err)
	}
	rrows, err := tx2.Query("SELECT id FROM sprints WHERE id = ?", "tx-pg")
	if err != nil {
		_ = tx2.Rollback()
		t.Fatalf("tx2.Query: %v", err)
	}
	rrows.Close()
	if err := tx2.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
}

func TestOpenPostgres_BadDSN(t *testing.T) {
	_, err := OpenPostgres("postgres://invalid:wrong@127.0.0.1:1/nope?sslmode=disable&connect_timeout=1")
	if err == nil {
		t.Fatal("expected error for unreachable PG, got nil")
	}
}

func TestInsertReturningID_CorePaths_Postgres(t *testing.T) {
	baseDSN := pgTestDSN(t)
	dsn, cleanup := pgIsolatedDSN(t, baseDSN)
	defer cleanup()

	store, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("OpenPostgres: %v", err)
	}
	defer store.Close()

	sprintID := "pg-insert-" + t.Name()
	if err := store.CreateSprint(Sprint{ID: sprintID, Name: "PG insert smoke", Status: SprintActive}); err != nil {
		t.Fatalf("CreateSprint: %v", err)
	}
	ticketID := "tkt-pg-insert"
	if err := store.CreateTicket(Ticket{
		ID:       ticketID,
		SprintID: sprintID,
		Title:    "PG insert ticket",
		Status:   StatusReady,
	}); err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}

	goalID, err := store.CreateSprintGoal(SprintGoal{
		SprintID: sprintID,
		GoalText: "ship PG inserts",
	})
	if err != nil {
		t.Fatalf("CreateSprintGoal: %v", err)
	}
	if goalID <= 0 {
		t.Fatalf("CreateSprintGoal id = %d, want > 0", goalID)
	}

	handoffID, err := store.PublishHandoff(CoordinationHandoff{
		TicketID:  ticketID,
		FromAgent: "agent-a",
		ToAgent:   "agent-b",
		Summary:   "pg insert smoke",
	})
	if err != nil {
		t.Fatalf("PublishHandoff: %v", err)
	}
	if handoffID <= 0 {
		t.Fatalf("PublishHandoff id = %d, want > 0", handoffID)
	}

	comment, err := store.AddTicketComment(ticketID, "agent-a", "pg comment smoke")
	if err != nil {
		t.Fatalf("AddTicketComment: %v", err)
	}
	if comment.ID <= 0 {
		t.Fatalf("AddTicketComment id = %d, want > 0", comment.ID)
	}
}

func TestInsertTerminalSessionEvent_Postgres(t *testing.T) {
	baseDSN := pgTestDSN(t)
	dsn, cleanup := pgIsolatedDSN(t, baseDSN)
	defer cleanup()

	store, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("OpenPostgres: %v", err)
	}
	defer store.Close()

	id, err := store.InsertTerminalSessionEvent(TerminalSessionEvent{
		Host:         "wsl1",
		SessionID:    "pg-probe",
		CommandClass: "curl",
		DurationMs:   1,
		Status:       "ok",
		Payload:      map[string]string{"rca": "insertReturningID"},
	})
	if err != nil {
		t.Fatalf("InsertTerminalSessionEvent: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive id, got %d", id)
	}
}
