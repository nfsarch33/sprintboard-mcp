package sprintboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

type CoordinationHandoff struct {
	ID        int64     `json:"id"`
	TicketID  string    `json:"ticket_id"`
	FromAgent string    `json:"from_agent"`
	ToAgent   string    `json:"to_agent"`
	Summary   string    `json:"summary"`
	Branch    string    `json:"branch,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) PublishHandoff(h CoordinationHandoff) (int64, error) {
	now := time.Now()
	if h.CreatedAt.IsZero() {
		h.CreatedAt = now
	}

	res, err := s.db.Exec(
		`INSERT INTO handoffs (ticket_id, from_agent, to_agent, context_path, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		h.TicketID, h.FromAgent, h.ToAgent, h.Summary, formatTime(h.CreatedAt),
	)
	if err != nil {
		return 0, err
	}

	id, _ := res.LastInsertId()

	if err := bridgeToMem0(h); err != nil {
		fmt.Fprintf(os.Stderr, "mem0 bridge failed (non-fatal): %v\n", err)
	}

	return id, nil
}

func (s *Store) SubscribeHandoffs(agentID string, since time.Time) ([]CoordinationHandoff, error) {
	rows, err := s.db.Query(
		`SELECT id, ticket_id, from_agent, to_agent, context_path, created_at
		 FROM handoffs WHERE to_agent = ? AND created_at >= ?
		 ORDER BY created_at DESC`,
		agentID, formatTime(since),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var handoffs []CoordinationHandoff
	for rows.Next() {
		var h CoordinationHandoff
		var createdAt, summary string
		if err := rows.Scan(&h.ID, &h.TicketID, &h.FromAgent, &h.ToAgent, &summary, &createdAt); err != nil {
			return nil, err
		}
		h.Summary = summary
		h.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		handoffs = append(handoffs, h)
	}
	return handoffs, rows.Err()
}

func bridgeToMem0(h CoordinationHandoff) error {
	mem0URL := os.Getenv("MEM0_BASE_URL")
	mem0Key := os.Getenv("MEM0_API_KEY")
	if mem0URL == "" {
		return nil
	}

	payload := map[string]interface{}{
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": fmt.Sprintf("handoff from %s to %s: %s (ticket: %s)", h.FromAgent, h.ToAgent, h.Summary, h.TicketID),
			},
		},
		"user_id":  "nfsarch33",
		"app_id":   "cursor-coordination",
		"metadata": map[string]string{"category": "handoff", "ticket_id": h.TicketID},
		"infer":    false,
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", mem0URL+"/v1/memories/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if mem0Key != "" {
		req.Header.Set("X-API-Key", mem0Key)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("mem0 returned %d", resp.StatusCode)
	}
	return nil
}
