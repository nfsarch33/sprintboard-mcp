package sprintboard

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// FleetPROutcome stores pr-review poll merge/reject/block outcomes (Phase 5).
type FleetPROutcome struct {
	ID            int64     `json:"id"`
	Host          string    `json:"host"`
	Repo          string    `json:"repo"`
	PRNumber      int       `json:"pr_number"`
	Outcome       string    `json:"outcome"`
	Verdict       string    `json:"verdict,omitempty"`
	ReviewerAgent string    `json:"reviewer_agent,omitempty"`
	MergeSHA      string    `json:"merge_sha,omitempty"`
	Payload       any       `json:"payload"`
	PayloadJSON   []byte    `json:"-"`
	RecordedAt    time.Time `json:"recorded_at"`
	CreatedAt     time.Time `json:"created_at"`
}

func (s *Store) migrateFleetPROutcomes() error {
	schema := `
	CREATE TABLE IF NOT EXISTS fleet_pr_outcomes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		host TEXT NOT NULL,
		repo TEXT NOT NULL,
		pr_number INTEGER NOT NULL,
		outcome TEXT NOT NULL,
		verdict TEXT,
		reviewer_agent TEXT,
		merge_sha TEXT,
		payload TEXT NOT NULL,
		recorded_at TEXT NOT NULL,
		created_at TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_fleet_pr_outcomes_host_recorded
		ON fleet_pr_outcomes(host, recorded_at);
	CREATE INDEX IF NOT EXISTS idx_fleet_pr_outcomes_repo_pr
		ON fleet_pr_outcomes(repo, pr_number);
	`
	if _, err := s.db.ExecDDL(schema); err != nil {
		return fmt.Errorf("migrate fleet_pr_outcomes: %w", err)
	}
	return nil
}

// InsertFleetPROutcome persists one PR review outcome row.
func (s *Store) InsertFleetPROutcome(row FleetPROutcome) (int64, error) {
	if row.Host == "" {
		return 0, fmt.Errorf("host is required")
	}
	if row.Repo == "" {
		return 0, fmt.Errorf("repo is required")
	}
	if row.PRNumber <= 0 {
		return 0, fmt.Errorf("pr_number must be positive")
	}
	if row.Outcome == "" {
		return 0, fmt.Errorf("outcome is required")
	}
	if row.Payload == nil && len(row.PayloadJSON) == 0 {
		return 0, fmt.Errorf("payload is required")
	}

	payloadJSON := row.PayloadJSON
	if len(payloadJSON) == 0 {
		var err error
		payloadJSON, err = json.Marshal(row.Payload)
		if err != nil {
			return 0, fmt.Errorf("marshal payload: %w", err)
		}
	}

	now := time.Now().UTC()
	if row.RecordedAt.IsZero() {
		row.RecordedAt = now
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now
	}

	res, err := s.db.Exec(
		`INSERT INTO fleet_pr_outcomes (host, repo, pr_number, outcome, verdict, reviewer_agent, merge_sha, payload, recorded_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row.Host, row.Repo, row.PRNumber, row.Outcome, row.Verdict, row.ReviewerAgent, row.MergeSHA,
		string(payloadJSON), formatTime(row.RecordedAt), formatTime(row.CreatedAt),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetFleetPROutcome returns one outcome by id.
func (s *Store) GetFleetPROutcome(id int64) (FleetPROutcome, error) {
	var row FleetPROutcome
	var recordedAt, createdAt, payload sql.NullString
	err := s.db.QueryRow(
		`SELECT id, host, repo, pr_number, outcome, verdict, reviewer_agent, merge_sha, payload, recorded_at, created_at
		 FROM fleet_pr_outcomes WHERE id = ?`, id,
	).Scan(&row.ID, &row.Host, &row.Repo, &row.PRNumber, &row.Outcome, &row.Verdict,
		&row.ReviewerAgent, &row.MergeSHA, &payload, &recordedAt, &createdAt)
	if err != nil {
		return FleetPROutcome{}, fmt.Errorf("fleet pr outcome %d: %w", id, err)
	}
	row.RecordedAt = parseTime(nullString(recordedAt))
	row.CreatedAt = parseTime(nullString(createdAt))
	row.PayloadJSON = []byte(nullString(payload))
	return row, nil
}

// ListFleetPROutcomes returns outcomes since the given time, optionally filtered by host.
func (s *Store) ListFleetPROutcomes(host string, since time.Time) ([]FleetPROutcome, error) {
	query := `SELECT id, host, repo, pr_number, outcome, verdict, reviewer_agent, merge_sha, payload, recorded_at, created_at
		FROM fleet_pr_outcomes WHERE recorded_at >= ?`
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

	var out []FleetPROutcome
	for rows.Next() {
		var row FleetPROutcome
		var recordedAt, createdAt, payload sql.NullString
		if err := rows.Scan(&row.ID, &row.Host, &row.Repo, &row.PRNumber, &row.Outcome, &row.Verdict,
			&row.ReviewerAgent, &row.MergeSHA, &payload, &recordedAt, &createdAt); err != nil {
			return nil, err
		}
		row.RecordedAt = parseTime(nullString(recordedAt))
		row.CreatedAt = parseTime(nullString(createdAt))
		row.PayloadJSON = []byte(nullString(payload))
		out = append(out, row)
	}
	return out, rows.Err()
}
