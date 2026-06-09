package sprintboard

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// EvalRunSnapshot stores eval harness judge run aggregates for Grafana 7d trends.
type EvalRunSnapshot struct {
	ID          int64     `json:"id"`
	Host        string    `json:"host"`
	EvalRunID   string    `json:"eval_run_id"`
	Suite       string    `json:"suite"`
	Model       string    `json:"model,omitempty"`
	Score       float64   `json:"score"`
	PassCount   int       `json:"pass_count"`
	FailCount   int       `json:"fail_count"`
	DurationMs  int64     `json:"duration_ms"`
	Payload     any       `json:"payload"`
	PayloadJSON []byte    `json:"-"`
	RecordedAt  time.Time `json:"recorded_at"`
	CreatedAt   time.Time `json:"created_at"`
}

func (s *Store) migrateEvalRunSnapshots() error {
	schema := `
	CREATE TABLE IF NOT EXISTS eval_run_snapshots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		host TEXT NOT NULL,
		eval_run_id TEXT NOT NULL,
		suite TEXT NOT NULL,
		model TEXT,
		score REAL NOT NULL DEFAULT 0,
		pass_count INTEGER NOT NULL DEFAULT 0,
		fail_count INTEGER NOT NULL DEFAULT 0,
		duration_ms INTEGER NOT NULL DEFAULT 0,
		payload TEXT NOT NULL,
		recorded_at TEXT NOT NULL,
		created_at TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_eval_run_snapshots_host_suite
		ON eval_run_snapshots(host, suite);
	CREATE INDEX IF NOT EXISTS idx_eval_run_snapshots_recorded_at
		ON eval_run_snapshots(recorded_at);
	`
	if _, err := s.db.ExecDDL(schema); err != nil {
		return fmt.Errorf("migrate eval_run_snapshots: %w", err)
	}
	return nil
}

// InsertEvalRunSnapshot persists one eval run snapshot row.
func (s *Store) InsertEvalRunSnapshot(snap EvalRunSnapshot) (int64, error) {
	if snap.Host == "" {
		return 0, fmt.Errorf("host is required")
	}
	if snap.EvalRunID == "" {
		return 0, fmt.Errorf("eval_run_id is required")
	}
	if snap.Suite == "" {
		return 0, fmt.Errorf("suite is required")
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
	if snap.RecordedAt.IsZero() {
		snap.RecordedAt = now
	}
	if snap.CreatedAt.IsZero() {
		snap.CreatedAt = now
	}

	res, err := s.db.Exec(
		`INSERT INTO eval_run_snapshots (host, eval_run_id, suite, model, score, pass_count, fail_count, duration_ms, payload, recorded_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snap.Host, snap.EvalRunID, snap.Suite, snap.Model, snap.Score, snap.PassCount, snap.FailCount,
		snap.DurationMs, string(payloadJSON), formatTime(snap.RecordedAt), formatTime(snap.CreatedAt),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetEvalRunSnapshot returns one eval snapshot by id.
func (s *Store) GetEvalRunSnapshot(id int64) (EvalRunSnapshot, error) {
	var snap EvalRunSnapshot
	var recordedAt, createdAt, payload sql.NullString
	err := s.db.QueryRow(
		`SELECT id, host, eval_run_id, suite, model, score, pass_count, fail_count, duration_ms, payload, recorded_at, created_at
		 FROM eval_run_snapshots WHERE id = ?`, id,
	).Scan(&snap.ID, &snap.Host, &snap.EvalRunID, &snap.Suite, &snap.Model, &snap.Score, &snap.PassCount,
		&snap.FailCount, &snap.DurationMs, &payload, &recordedAt, &createdAt)
	if err != nil {
		return EvalRunSnapshot{}, fmt.Errorf("eval run snapshot %d: %w", id, err)
	}
	snap.RecordedAt = parseTime(nullString(recordedAt))
	snap.CreatedAt = parseTime(nullString(createdAt))
	snap.PayloadJSON = []byte(nullString(payload))
	return snap, nil
}

// ListEvalRunSnapshots returns snapshots with recorded_at >= since, newest first.
func (s *Store) ListEvalRunSnapshots(host string, since time.Time) ([]EvalRunSnapshot, error) {
	query := `SELECT id, host, eval_run_id, suite, model, score, pass_count, fail_count, duration_ms, payload, recorded_at, created_at
		FROM eval_run_snapshots WHERE recorded_at >= ?`
	args := []any{formatTime(since)}
	if host != "" {
		query += ` AND host = ?`
		args = append(args, host)
	}
	query += ` ORDER BY recorded_at DESC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []EvalRunSnapshot
	for rows.Next() {
		var snap EvalRunSnapshot
		var recordedAt, createdAt, payload sql.NullString
		if err := rows.Scan(&snap.ID, &snap.Host, &snap.EvalRunID, &snap.Suite, &snap.Model, &snap.Score, &snap.PassCount,
			&snap.FailCount, &snap.DurationMs, &payload, &recordedAt, &createdAt); err != nil {
			return nil, err
		}
		snap.RecordedAt = parseTime(nullString(recordedAt))
		snap.CreatedAt = parseTime(nullString(createdAt))
		snap.PayloadJSON = []byte(nullString(payload))
		out = append(out, snap)
	}
	return out, rows.Err()
}
