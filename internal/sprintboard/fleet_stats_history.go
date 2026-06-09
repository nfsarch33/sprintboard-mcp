package sprintboard

import (
	"encoding/json"
	"fmt"
	"time"
)

// FleetHistoryKind identifies a fleet stats history slice.
type FleetHistoryKind string

const (
	FleetHistoryAll             FleetHistoryKind = "all"
	FleetHistoryFleetReport     FleetHistoryKind = "fleet_report"
	FleetHistoryEvalRun         FleetHistoryKind = "eval_run"
	FleetHistoryTerminalSession FleetHistoryKind = "terminal_session"
	FleetHistoryPROutcome       FleetHistoryKind = "pr_outcome"
)

// FleetHistoryItem is a unified row for GET /api/v1/fleet-stats/history.
type FleetHistoryItem struct {
	Kind       FleetHistoryKind `json:"kind"`
	ID         int64            `json:"id"`
	Host       string           `json:"host"`
	RecordedAt time.Time        `json:"recorded_at"`
	Summary    map[string]any   `json:"summary"`
	Payload    json.RawMessage  `json:"payload,omitempty"`
}

// ListFleetStatsHistory returns unified history across fleet stat tables.
func (s *Store) ListFleetStatsHistory(kind FleetHistoryKind, host string, days int) ([]FleetHistoryItem, error) {
	if days <= 0 {
		days = 7
	}
	if days > 90 {
		days = 90
	}
	since := time.Now().UTC().AddDate(0, 0, -days)

	var items []FleetHistoryItem
	var err error

	switch kind {
	case FleetHistoryAll:
		for _, k := range []FleetHistoryKind{
			FleetHistoryFleetReport, FleetHistoryEvalRun, FleetHistoryTerminalSession, FleetHistoryPROutcome,
		} {
			part, e := s.ListFleetStatsHistory(k, host, days)
			if e != nil {
				return nil, e
			}
			items = append(items, part...)
		}
		sortFleetHistoryItems(items)
		return items, nil
	case FleetHistoryFleetReport:
		items, err = s.listFleetReportHistory(host, since)
	case FleetHistoryEvalRun:
		items, err = s.listEvalRunHistory(host, since)
	case FleetHistoryTerminalSession:
		items, err = s.listTerminalSessionHistory(host, since)
	case FleetHistoryPROutcome:
		items, err = s.listPROutcomeHistory(host, since)
	default:
		return nil, fmt.Errorf("unknown kind %q", kind)
	}
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []FleetHistoryItem{}
	}
	return items, nil
}

func (s *Store) listFleetReportHistory(host string, since time.Time) ([]FleetHistoryItem, error) {
	snaps, err := s.ListFleetReportSnapshots(host, since)
	if err != nil {
		return nil, err
	}
	items := make([]FleetHistoryItem, 0, len(snaps))
	for _, snap := range snaps {
		items = append(items, FleetHistoryItem{
			Kind:       FleetHistoryFleetReport,
			ID:         snap.ID,
			Host:       snap.Host,
			RecordedAt: snap.WindowEnd,
			Summary: map[string]any{
				"report_kind":  snap.ReportKind,
				"window_start": snap.WindowStart,
				"window_end":   snap.WindowEnd,
			},
			Payload: snap.PayloadJSON,
		})
	}
	return items, nil
}

func (s *Store) listEvalRunHistory(host string, since time.Time) ([]FleetHistoryItem, error) {
	snaps, err := s.ListEvalRunSnapshots(host, since)
	if err != nil {
		return nil, err
	}
	items := make([]FleetHistoryItem, 0, len(snaps))
	for _, snap := range snaps {
		items = append(items, FleetHistoryItem{
			Kind:       FleetHistoryEvalRun,
			ID:         snap.ID,
			Host:       snap.Host,
			RecordedAt: snap.RecordedAt,
			Summary: map[string]any{
				"eval_run_id": snap.EvalRunID,
				"suite":       snap.Suite,
				"model":       snap.Model,
				"score":       snap.Score,
				"pass_count":  snap.PassCount,
				"fail_count":  snap.FailCount,
				"duration_ms": snap.DurationMs,
			},
			Payload: snap.PayloadJSON,
		})
	}
	return items, nil
}

func (s *Store) listTerminalSessionHistory(host string, since time.Time) ([]FleetHistoryItem, error) {
	events, err := s.ListTerminalSessionEvents(host, since)
	if err != nil {
		return nil, err
	}
	items := make([]FleetHistoryItem, 0, len(events))
	for _, ev := range events {
		summary := map[string]any{
			"session_id":    ev.SessionID,
			"command_class": ev.CommandClass,
			"status":        ev.Status,
			"duration_ms":   ev.DurationMs,
		}
		if ev.ExitCode != nil {
			summary["exit_code"] = *ev.ExitCode
		}
		items = append(items, FleetHistoryItem{
			Kind:       FleetHistoryTerminalSession,
			ID:         ev.ID,
			Host:       ev.Host,
			RecordedAt: ev.CreatedAt,
			Summary:    summary,
			Payload:    ev.PayloadJSON,
		})
	}
	return items, nil
}

func (s *Store) listPROutcomeHistory(host string, since time.Time) ([]FleetHistoryItem, error) {
	rows, err := s.ListFleetPROutcomes(host, since)
	if err != nil {
		return nil, err
	}
	items := make([]FleetHistoryItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, FleetHistoryItem{
			Kind:       FleetHistoryPROutcome,
			ID:         row.ID,
			Host:       row.Host,
			RecordedAt: row.RecordedAt,
			Summary: map[string]any{
				"repo":           row.Repo,
				"pr_number":      row.PRNumber,
				"outcome":        row.Outcome,
				"verdict":        row.Verdict,
				"reviewer_agent": row.ReviewerAgent,
				"merge_sha":      row.MergeSHA,
			},
			Payload: row.PayloadJSON,
		})
	}
	return items, nil
}

func sortFleetHistoryItems(items []FleetHistoryItem) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].RecordedAt.After(items[j-1].RecordedAt); j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}
