package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

var tables = []string{
	"sprints",
	"tickets",
	"ticket_transitions",
	"handoffs",
	"agents",
	"ticket_dependencies",
	"embeddings",
	"ticket_comments",
	"sprint_templates",
	"roadmaps",
	"programmes",
	"epics",
	"session_handoffs",
}

func main() {
	sqlitePath := flag.String("sqlite", "", "Source SQLite database path")
	pgDSN := flag.String("pg", "", "Target PostgreSQL DSN")
	dryRun := flag.Bool("dry-run", false, "Count rows without copying")
	flag.Parse()

	if *sqlitePath == "" || *pgDSN == "" {
		fmt.Fprintln(os.Stderr, "usage: sprintboard-migrate -sqlite <path> -pg <dsn>")
		os.Exit(1)
	}

	srcDB, err := sql.Open("sqlite", *sqlitePath)
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = srcDB.Close() }()

	dstDB, err := sql.Open("pgx", *pgDSN)
	if err != nil {
		log.Fatalf("open postgres: %v", err)
	}
	defer func() { _ = dstDB.Close() }()

	if err := dstDB.Ping(); err != nil {
		log.Fatalf("ping postgres: %v", err)
	}

	for _, table := range tables {
		srcCount := countRows(srcDB, table)
		if srcCount < 0 {
			// countRows could not read the table. Distinct from "0 rows": we do
			// not know what is there, so skipping would silently drop data.
			log.Printf("%-25s: SKIPPED, source count unreadable", table)
			continue
		}
		if srcCount == 0 {
			log.Printf("%-25s: 0 rows (skip)", table)
			continue
		}

		if *dryRun {
			dstCount := countRows(dstDB, table)
			log.Printf("%-25s: %d src → %d dst", table, srcCount, dstCount)
			continue
		}

		copied, err := copyTable(srcDB, dstDB, table)
		if err != nil {
			log.Printf("%-25s: ERROR: %v", table, err)
			continue
		}

		dstCount := countRows(dstDB, table)
		log.Printf("%-25s: %d copied (%d in dst)", table, copied, dstCount)
	}

	if !*dryRun {
		resetSequences(dstDB)
	}
	log.Println("migration complete")
}

func resetSequences(db *sql.DB) {
	seqs := []struct {
		table, col string
	}{
		{"ticket_transitions", "id"},
		{"handoffs", "id"},
		{"ticket_comments", "id"},
	}
	for _, s := range seqs {
		var maxID sql.NullInt64
		// #nosec G201 -- table/column come from the hardcoded seqs list above,
		// never from user input.
		if err := db.QueryRow(fmt.Sprintf("SELECT MAX(%s) FROM %s", s.col, s.table)).Scan(&maxID); err != nil {
			log.Printf("read max %s.%s: %v", s.table, s.col, err)
			continue
		}
		if maxID.Valid && maxID.Int64 > 0 {
			seqName := fmt.Sprintf("%s_%s_seq", s.table, s.col)
			_, err := db.Exec(fmt.Sprintf("SELECT setval('%s', $1)", seqName), maxID.Int64)
			if err != nil {
				log.Printf("reset sequence %s: %v", seqName, err)
			} else {
				log.Printf("sequence %-30s: reset to %d", seqName, maxID.Int64)
			}
		}
	}
}

// countRows returns -1 when the count cannot be read. Returning 0 there
// made a failed query indistinguishable from an empty table, which is exactly
// the reading a migration verification must not get wrong.
func countRows(db *sql.DB, table string) int {
	var count int
	// #nosec G201 -- table names come from the migrator's own hardcoded list.
	if err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count); err != nil {
		log.Printf("count rows in %s: %v", table, err)
		return -1
	}
	return count
}

func copyTable(src, dst *sql.DB, table string) (int, error) {
	cols, err := getColumns(src, table)
	if err != nil {
		return 0, fmt.Errorf("get columns: %w", err)
	}

	// #nosec G201 -- `table` comes from this migrator's hardcoded table list
	// and `cols` from the driver's own column metadata; neither is user input.
	selectQ := fmt.Sprintf("SELECT %s FROM %s", cols, table)
	rows, err := src.Query(selectQ)
	if err != nil {
		return 0, fmt.Errorf("query source: %w", err)
	}
	defer func() { _ = rows.Close() }()

	colNames, err := rows.Columns()
	if err != nil {
		return 0, err
	}

	placeholders := makePGPlaceholders(len(colNames))
	// #nosec G201 -- same provenance as selectQ above; values are still bound
	// as parameters via the generated placeholders.
	insertQ := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON CONFLICT DO NOTHING",
		table, cols, placeholders)

	tx, err := dst.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	copied := 0
	for rows.Next() {
		vals := make([]any, len(colNames))
		ptrs := make([]any, len(colNames))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return copied, fmt.Errorf("scan row: %w", err)
		}

		if _, err := tx.Exec(insertQ, vals...); err != nil {
			return copied, fmt.Errorf("insert row: %w", err)
		}
		copied++
	}

	if err := tx.Commit(); err != nil {
		return copied, fmt.Errorf("commit: %w", err)
	}
	return copied, rows.Err()
}

func getColumns(db *sql.DB, table string) (string, error) {
	rows, err := db.Query(fmt.Sprintf("SELECT * FROM %s LIMIT 0", table))
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		return "", err
	}

	quoted := make([]string, len(cols))
	copy(quoted, cols)
	result := ""
	for i, c := range quoted {
		if i > 0 {
			result += ", "
		}
		result += c
	}
	return result, nil
}

func makePGPlaceholders(n int) string {
	s := ""
	for i := 1; i <= n; i++ {
		if i > 1 {
			s += ", "
		}
		s += fmt.Sprintf("$%d", i)
	}
	return s
}
