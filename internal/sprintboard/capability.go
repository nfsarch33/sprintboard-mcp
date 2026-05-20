package sprintboard

import (
	"sort"
	"strings"
)

type CapabilityMatch struct {
	AgentID      string   `json:"agent_id"`
	MatchCount   int      `json:"match_count"`
	TotalRequired int     `json:"total_required"`
	MatchScore   float64  `json:"match_score"`
	MissingCaps  []string `json:"missing_caps,omitempty"`
}

type GapReport struct {
	TicketID     string   `json:"ticket_id"`
	Required     []string `json:"required"`
	MissingCaps  []string `json:"missing_caps"`
}

func parseCaps(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	var caps []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			caps = append(caps, trimmed)
		}
	}
	return caps
}

func (s *Store) SuggestAgent(ticketID string) ([]CapabilityMatch, error) {
	ticket, err := s.GetTicket(ticketID)
	if err != nil {
		return nil, err
	}

	required := parseCaps(ticket.AcceptanceCriteria)
	if len(required) == 0 {
		agents, err := s.ListActiveAgents()
		if err != nil {
			return nil, err
		}
		var matches []CapabilityMatch
		for _, a := range agents {
			matches = append(matches, CapabilityMatch{
				AgentID:       a.ID,
				MatchCount:    0,
				TotalRequired: 0,
				MatchScore:    1.0,
			})
		}
		return matches, nil
	}

	agents, err := s.ListActiveAgents()
	if err != nil {
		return nil, err
	}

	reqSet := make(map[string]bool)
	for _, r := range required {
		reqSet[r] = true
	}

	var matches []CapabilityMatch
	for _, a := range agents {
		agentCaps := make(map[string]bool)
		for _, c := range parseCaps(a.Capabilities) {
			agentCaps[c] = true
		}

		matchCount := 0
		var missing []string
		for _, r := range required {
			if agentCaps[r] {
				matchCount++
			} else {
				missing = append(missing, r)
			}
		}

		score := 0.0
		if len(required) > 0 {
			score = float64(matchCount) / float64(len(required))
		}

		matches = append(matches, CapabilityMatch{
			AgentID:       a.ID,
			MatchCount:    matchCount,
			TotalRequired: len(required),
			MatchScore:    score,
			MissingCaps:   missing,
		})
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].MatchScore > matches[j].MatchScore
	})

	return matches, nil
}

func (s *Store) CapabilityGapReport(sprintID string) ([]GapReport, error) {
	tickets, err := s.ListTickets(sprintID)
	if err != nil {
		return nil, err
	}

	agents, err := s.ListActiveAgents()
	if err != nil {
		return nil, err
	}

	allCaps := make(map[string]bool)
	for _, a := range agents {
		for _, c := range parseCaps(a.Capabilities) {
			allCaps[c] = true
		}
	}

	var gaps []GapReport
	for _, t := range tickets {
		required := parseCaps(t.AcceptanceCriteria)
		if len(required) == 0 {
			continue
		}

		var missing []string
		for _, r := range required {
			if !allCaps[r] {
				missing = append(missing, r)
			}
		}

		if len(missing) > 0 {
			gaps = append(gaps, GapReport{
				TicketID:    t.ID,
				Required:    required,
				MissingCaps: missing,
			})
		}
	}

	return gaps, nil
}
