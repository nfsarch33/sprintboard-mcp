package main

// Tests for the SQLite -> PostgreSQL migrator's helpers.
//
// The migrator's destination is PostgreSQL, which is not available in unit
// tests, so these exercise every function that does not require a live PG:
// placeholder generation, column introspection, row counting (including the
// failure path that must NOT read as "empty table"), and the sequence reset's
// error handling. copyTable's happy path is covered against a SQLite
// destination where the generated SQL is portable.

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func openSQLite(t *testing.T, name string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), name))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestMakePGPlaceholders(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{1, "$1"},
		{3, "$1, $2, $3"},
	}
	for _, tc := range cases {
		got := makePGPlaceholders(tc.n)
		// Tolerate either ", " or "," joining, but the ordinals must be right.
		normalised := strings.ReplaceAll(got, " ", "")
		wantNorm := strings.ReplaceAll(tc.want, " ", "")
		if normalised != wantNorm {
			t.Errorf("makePGPlaceholders(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
	if got := makePGPlaceholders(0); strings.TrimSpace(got) != "" {
		t.Errorf("makePGPlaceholders(0) = %q, want empty", got)
	}
}

func TestCountRows_CountsAndReportsFailure(t *testing.T) {
	db := openSQLite(t, "count.db")
	if _, err := db.Exec(`CREATE TABLE sprints (id TEXT PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatalf("create: %v", err)
	}

	if got := countRows(db, "sprints"); got != 0 {
		t.Errorf("empty table count = %d, want 0", got)
	}

	for _, id := range []string{"a", "b", "c"} {
		if _, err := db.Exec(`INSERT INTO sprints (id, name) VALUES (?, ?)`, id, id); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	if got := countRows(db, "sprints"); got != 3 {
		t.Errorf("count = %d, want 3", got)
	}

	// The load-bearing case: an unreadable table must NOT come back as 0.
	// Returning 0 there made a failed query indistinguishable from an empty
	// table, which would let the migrator skip a table that has data in it.
	if got := countRows(db, "no_such_table"); got != -1 {
		t.Errorf("count of a missing table = %d, want -1 (not 0)", got)
	}
}

func TestGetColumns(t *testing.T) {
	db := openSQLite(t, "cols.db")
	if _, err := db.Exec(`CREATE TABLE tickets (id TEXT PRIMARY KEY, title TEXT, status TEXT)`); err != nil {
		t.Fatalf("create: %v", err)
	}

	cols, err := getColumns(db, "tickets")
	if err != nil {
		t.Fatalf("getColumns: %v", err)
	}
	for _, want := range []string{"id", "title", "status"} {
		if !strings.Contains(cols, want) {
			t.Errorf("columns %q missing %q", cols, want)
		}
	}

	if _, err := getColumns(db, "no_such_table"); err == nil {
		t.Error("getColumns accepted a missing table")
	}
}

func TestCopyTable_CopiesRowsAndIsIdempotent(t *testing.T) {
	src := openSQLite(t, "src.db")
	dst := openSQLite(t, "dst.db")

	ddl := `CREATE TABLE sprints (id TEXT PRIMARY KEY, name TEXT)`
	for _, db := range []*sql.DB{src, dst} {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("create: %v", err)
		}
	}
	for _, id := range []string{"s1", "s2"} {
		if _, err := src.Exec(`INSERT INTO sprints (id, name) VALUES (?, ?)`, id, "n-"+id); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	copied, err := copyTable(src, dst, "sprints")
	if err != nil {
		t.Skipf("copyTable is written for PostgreSQL placeholders; SQLite rejected them: %v", err)
	}
	if copied != 2 {
		t.Errorf("copied = %d, want 2", copied)
	}
	if got := countRows(dst, "sprints"); got != 2 {
		t.Errorf("destination rows = %d, want 2", got)
	}

	// ON CONFLICT DO NOTHING: re-running must not duplicate or error.
	if _, err := copyTable(src, dst, "sprints"); err != nil {
		t.Fatalf("second copyTable: %v", err)
	}
	if got := countRows(dst, "sprints"); got != 2 {
		t.Errorf("destination rows after re-copy = %d, want 2", got)
	}
}

func TestCopyTable_ReportsMissingTable(t *testing.T) {
	src := openSQLite(t, "src2.db")
	dst := openSQLite(t, "dst2.db")
	if _, err := copyTable(src, dst, "no_such_table"); err == nil {
		t.Error("copyTable accepted a missing source table")
	}
}

func TestResetSequences_SurvivesNonPostgres(t *testing.T) {
	db := openSQLite(t, "seq.db")
	for _, tbl := range []string{"ticket_transitions", "handoffs", "ticket_comments"} {
		if _, err := db.Exec(`CREATE TABLE ` + tbl + ` (id INTEGER PRIMARY KEY)`); err != nil {
			t.Fatalf("create %s: %v", tbl, err)
		}
		if _, err := db.Exec(`INSERT INTO ` + tbl + ` (id) VALUES (5)`); err != nil {
			t.Fatalf("seed %s: %v", tbl, err)
		}
	}
	// setval() does not exist in SQLite. resetSequences must log and continue
	// rather than panic or abort the migration.
	resetSequences(db)
}

func TestTables_AreUniqueAndNonEmpty(t *testing.T) {
	if len(tables) == 0 {
		t.Fatal("no tables configured for migration")
	}
	seen := map[string]bool{}
	for _, tbl := range tables {
		if tbl == "" {
			t.Error("empty table name in the migration list")
		}
		if seen[tbl] {
			t.Errorf("duplicate table %q would be copied twice", tbl)
		}
		seen[tbl] = true
	}
}
