package sprintboard

import "testing"

func TestRewriteSQL_SQLite(t *testing.T) {
	q := `SELECT id FROM tickets WHERE sprint_id = ? AND status = ?`
	got := rewriteSQL(q, DialectSQLite)
	if got != q {
		t.Fatalf("sqlite should be no-op: got %q", got)
	}
}

func TestRewriteSQL_Postgres(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "two placeholders",
			in:   `SELECT id FROM tickets WHERE sprint_id = ? AND status = ?`,
			want: `SELECT id FROM tickets WHERE sprint_id = $1 AND status = $2`,
		},
		{
			name: "no placeholders",
			in:   `SELECT COUNT(*) FROM tickets`,
			want: `SELECT COUNT(*) FROM tickets`,
		},
		{
			name: "insert with many placeholders",
			in:   `INSERT INTO sprints (id, name, status) VALUES (?, ?, ?)`,
			want: `INSERT INTO sprints (id, name, status) VALUES ($1, $2, $3)`,
		},
		{
			name: "question mark in string literal preserved",
			in:   `SELECT * FROM tickets WHERE title LIKE '%?%' AND id = ?`,
			want: `SELECT * FROM tickets WHERE title LIKE '%?%' AND id = $1`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rewriteSQL(tt.in, DialectPostgres)
			if got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestDDLRewrite_SQLite(t *testing.T) {
	ddl := `CREATE TABLE IF NOT EXISTS t (id INTEGER PRIMARY KEY AUTOINCREMENT, data BLOB)`
	got := ddlRewrite(ddl, DialectSQLite)
	if got != ddl {
		t.Fatalf("sqlite should be no-op: got %q", got)
	}
}

func TestDDLRewrite_Postgres(t *testing.T) {
	ddl := `CREATE TABLE IF NOT EXISTS t (id INTEGER PRIMARY KEY AUTOINCREMENT, data BLOB NOT NULL)`
	want := `CREATE TABLE IF NOT EXISTS t (id BIGSERIAL PRIMARY KEY, data BYTEA NOT NULL)`
	got := ddlRewrite(ddl, DialectPostgres)
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}
