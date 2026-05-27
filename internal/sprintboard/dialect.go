package sprintboard

import (
	"database/sql"
	"fmt"
	"strings"
)

const (
	DialectSQLite   = "sqlite"
	DialectPostgres = "postgres"
)

// rewriteSQL converts SQLite-style ? placeholders to PostgreSQL-style $N.
// For SQLite dialect the query passes through unchanged.
func rewriteSQL(query, dialect string) string {
	if dialect != DialectPostgres {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 32)
	n := 1
	inQuote := false
	for i := 0; i < len(query); i++ {
		ch := query[i]
		if ch == '\'' {
			inQuote = !inQuote
		}
		if ch == '?' && !inQuote {
			fmt.Fprintf(&b, "$%d", n)
			n++
		} else {
			b.WriteByte(ch)
		}
	}
	return b.String()
}

// ddlRewrite translates SQLite-specific DDL keywords to PostgreSQL equivalents.
// Only affects CREATE TABLE statements with AUTOINCREMENT or BLOB types.
func ddlRewrite(schema, dialect string) string {
	if dialect != DialectPostgres {
		return schema
	}
	r := strings.NewReplacer(
		"INTEGER PRIMARY KEY AUTOINCREMENT", "BIGSERIAL PRIMARY KEY",
		"BLOB", "BYTEA",
	)
	return r.Replace(schema)
}

// dialectDB wraps *sql.DB with transparent SQL dialect rewriting so all
// Exec/Query/QueryRow calls automatically convert ? placeholders to $N
// when the target is PostgreSQL.
type dialectDB struct {
	raw     *sql.DB
	dialect string
}

func (d *dialectDB) Exec(query string, args ...any) (sql.Result, error) {
	return d.raw.Exec(rewriteSQL(query, d.dialect), args...)
}

func (d *dialectDB) ExecDDL(query string) (sql.Result, error) {
	return d.raw.Exec(ddlRewrite(query, d.dialect))
}

func (d *dialectDB) Query(query string, args ...any) (*sql.Rows, error) {
	return d.raw.Query(rewriteSQL(query, d.dialect), args...)
}

func (d *dialectDB) QueryRow(query string, args ...any) *sql.Row {
	return d.raw.QueryRow(rewriteSQL(query, d.dialect), args...)
}

func (d *dialectDB) Begin() (*dialectTx, error) {
	tx, err := d.raw.Begin()
	if err != nil {
		return nil, err
	}
	return &dialectTx{raw: tx, dialect: d.dialect}, nil
}

func (d *dialectDB) Close() error { return d.raw.Close() }

// dialectTx wraps *sql.Tx with automatic placeholder rewriting.
type dialectTx struct {
	raw     *sql.Tx
	dialect string
}

func (t *dialectTx) Exec(query string, args ...any) (sql.Result, error) {
	return t.raw.Exec(rewriteSQL(query, t.dialect), args...)
}

func (t *dialectTx) QueryRow(query string, args ...any) *sql.Row {
	return t.raw.QueryRow(rewriteSQL(query, t.dialect), args...)
}

func (t *dialectTx) Query(query string, args ...any) (*sql.Rows, error) {
	return t.raw.Query(rewriteSQL(query, t.dialect), args...)
}

func (t *dialectTx) Commit() error   { return t.raw.Commit() }
func (t *dialectTx) Rollback() error { return t.raw.Rollback() }
