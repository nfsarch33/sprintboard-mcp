package sprintboard

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// FleetReportSnapshot stores a point-in-time fleet daily/status report payload
// for Grafana/MCP history queries (ADR-073 Tier 1 PG persistence).
type FleetReportSnapshot struct {
	ID          int64     `json:"id"`
	Host        string    `json:"host"`
	ReportKind  string    `json:"report_kind"`
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
	Payload     any       `json:"payload"`
	PayloadJSON []byte    `json:"-"`
	CreatedAt   time.Time `json:"created_at"`
}

func (s *Store) migrateFleetReportSnapshots() error {
	schema := `
	CREATE TABLE IF NOT EXISTS fleet_report_snapshots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		host TEXT NOT NULL,
		report_kind TEXT NOT NULL,
		window_start TEXT NOT NULL,
		window_end TEXT NOT NULL,
		payload TEXT NOT NULL,
		created_at TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_fleet_report_snapshots_host_kind
		ON fleet_report_snapshots(host, report_kind);
	CREATE INDEX IF NOT EXISTS idx_fleet_report_snapshots_window_end
		ON fleet_report_snapshots(window_end);
	`
	if _, err := s.db.ExecDDL(schema); err != nil {
		return fmt.Errorf("migrate fleet_report_snapshots: %w", err)
	}
	return nil
}

// InsertFleetReportSnapshot persists a report snapshot. Payload is stored as
// JSON (TEXT on SQLite; JSON-compatible TEXT on PostgreSQL).
func (s *Store) InsertFleetReportSnapshot(snap FleetReportSnapshot) (int64, error) {
	if snap.Host == "" {
		return 0, fmt.Errorf("host is required")
	}
	if snap.ReportKind == "" {
		return 0, fmt.Errorf("report_kind is required")
	}
	if snap.WindowStart.IsZero() || snap.WindowEnd.IsZero() {
		return 0, fmt.Errorf("window_start and window_end are required")
	}
	if snap.Payload == nil && len(snap.PayloadJSON) == 0 {
		return 0, fmt.Errorf("payload is required")
	}

	payloadJSON := snap.PayloadJSON
	if len(payloadJSON) == 0 {
		var err error
		payloadJSON, err = json.Marshal(snap.Payload)
		if err != nil {
			return 0, fmt.Errorf("marshal payload: %w", err)
		}
	}

	now := time.Now().UTC()
	if snap.CreatedAt.IsZero() {
		snap.CreatedAt = now
	}

	res, err := s.db.Exec(
		`INSERT INTO fleet_report_snapshots (host, report_kind, window_start, window_end, payload, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		snap.Host, snap.ReportKind,
		formatTime(snap.WindowStart), formatTime(snap.WindowEnd),
		string(payloadJSON), formatTime(snap.CreatedAt),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetFleetReportSnapshot returns a snapshot by id.
func (s *Store) GetFleetReportSnapshot(id int64) (FleetReportSnapshot, error) {
	var snap FleetReportSnapshot
	var windowStart, windowEnd, createdAt, payload sql.NullString
	err := s.db.QueryRow(
		`SELECT id, host, report_kind, window_start, window_end, payload, created_at
		 FROM fleet_report_snapshots WHERE id = ?`, id,
	).Scan(&snap.ID, &snap.Host, &snap.ReportKind, &windowStart, &windowEnd, &payload, &createdAt)
	if err != nil {
		return FleetReportSnapshot{}, fmt.Errorf("fleet report snapshot %d: %w", id, err)
	}
	snap.WindowStart = parseTime(nullString(windowStart))
	snap.WindowEnd = parseTime(nullString(windowEnd))
	snap.CreatedAt = parseTime(nullString(createdAt))
	snap.PayloadJSON = []byte(nullString(payload))
	return snap, nil
}

// ListFleetReportSnapshots returns snapshots with window_end >= since, newest first.
func (s *Store) ListFleetReportSnapshots(host string, since time.Time) ([]FleetReportSnapshot, error) {
	query := `SELECT id, host, report_kind, window_start, window_end, payload, created_at
		FROM fleet_report_snapshots WHERE window_end >= ?`
	args := []any{formatTime(since)}
	if host != "" {
		query += ` AND host = ?`
		args = append(args, host)
	}
	query += ` ORDER BY window_end DESC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []FleetReportSnapshot
	for rows.Next() {
		var snap FleetReportSnapshot
		var windowStart, windowEnd, createdAt, payload sql.NullString
		if err := rows.Scan(&snap.ID, &snap.Host, &snap.ReportKind, &windowStart, &windowEnd, &payload, &createdAt); err != nil {
			return nil, err
		}
		snap.WindowStart = parseTime(nullString(windowStart))
		snap.WindowEnd = parseTime(nullString(windowEnd))
		snap.CreatedAt = parseTime(nullString(createdAt))
		snap.PayloadJSON = []byte(nullString(payload))
		out = append(out, snap)
	}
	return out, rows.Err()
}
