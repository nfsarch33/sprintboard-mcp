package sprintboard

import "fmt"

func (s *Store) migrateDAG() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS ticket_dependencies (
			ticket_id TEXT NOT NULL,
			depends_on TEXT NOT NULL,
			created_at TEXT NOT NULL,
			PRIMARY KEY (ticket_id, depends_on),
			FOREIGN KEY (ticket_id) REFERENCES tickets(id),
			FOREIGN KEY (depends_on) REFERENCES tickets(id)
		);
		CREATE INDEX IF NOT EXISTS idx_deps_ticket ON ticket_dependencies(ticket_id);
		CREATE INDEX IF NOT EXISTS idx_deps_depends ON ticket_dependencies(depends_on);
	`)
	return err
}

func (s *Store) AddDependency(ticketID, dependsOn string) error {
	if ticketID == dependsOn {
		return fmt.Errorf("ticket cannot depend on itself")
	}

	if s.wouldCreateCycle(ticketID, dependsOn) {
		return fmt.Errorf("adding dependency %s -> %s would create a cycle", ticketID, dependsOn)
	}

	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO ticket_dependencies (ticket_id, depends_on, created_at)
		 VALUES (?, ?, datetime('now'))`,
		ticketID, dependsOn,
	)
	return err
}

func (s *Store) RemoveDependency(ticketID, dependsOn string) error {
	_, err := s.db.Exec(
		`DELETE FROM ticket_dependencies WHERE ticket_id = ? AND depends_on = ?`,
		ticketID, dependsOn,
	)
	return err
}

func (s *Store) BlockedBy(ticketID string) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT d.depends_on FROM ticket_dependencies d
		 JOIN tickets t ON d.depends_on = t.id
		 WHERE d.ticket_id = ? AND t.status != 'done'`,
		ticketID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var blockers []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		blockers = append(blockers, id)
	}
	return blockers, rows.Err()
}

func (s *Store) ReadyTickets(sprintID string) ([]Ticket, error) {
	query := `
		SELECT t.id, t.sprint_id, t.title, t.description, t.status,
		       t.owner_agent, t.priority, t.acceptance_criteria,
		       t.handoff_doc_path, t.created_at, t.updated_at
		FROM tickets t
		WHERE t.status IN ('backlog', 'ready')
		  AND t.sprint_id = ?
		  AND NOT EXISTS (
			SELECT 1 FROM ticket_dependencies d
			JOIN tickets dep ON d.depends_on = dep.id
			WHERE d.ticket_id = t.id AND dep.status != 'done'
		  )
		ORDER BY t.priority DESC`

	rows, err := s.db.Query(query, sprintID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickets []Ticket
	for rows.Next() {
		var t Ticket
		var createdAt, updatedAt string
		var desc, owner, ac, handoff *string
		if err := rows.Scan(&t.ID, &t.SprintID, &t.Title, &desc, &t.Status,
			&owner, &t.Priority, &ac, &handoff, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if desc != nil {
			t.Description = *desc
		}
		if owner != nil {
			t.OwnerAgent = *owner
		}
		if ac != nil {
			t.AcceptanceCriteria = *ac
		}
		if handoff != nil {
			t.HandoffDocPath = *handoff
		}
		tickets = append(tickets, t)
	}
	return tickets, rows.Err()
}

func (s *Store) TopologicalSort(sprintID string) ([]string, error) {
	tickets, err := s.ListTickets(sprintID)
	if err != nil {
		return nil, err
	}

	deps := make(map[string][]string)
	inDegree := make(map[string]int)
	for _, t := range tickets {
		inDegree[t.ID] = 0
	}

	rows, err := s.db.Query(
		`SELECT d.ticket_id, d.depends_on FROM ticket_dependencies d
		 JOIN tickets t ON d.ticket_id = t.id
		 WHERE t.sprint_id = ?`, sprintID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var tid, dep string
		if err := rows.Scan(&tid, &dep); err != nil {
			return nil, err
		}
		deps[dep] = append(deps[dep], tid)
		inDegree[tid]++
	}

	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	var sorted []string
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		sorted = append(sorted, node)
		for _, next := range deps[node] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}

	if len(sorted) != len(tickets) {
		return nil, fmt.Errorf("cycle detected: sorted %d of %d tickets", len(sorted), len(tickets))
	}
	return sorted, nil
}

func (s *Store) wouldCreateCycle(ticketID, dependsOn string) bool {
	visited := make(map[string]bool)
	return s.dfsHasPath(dependsOn, ticketID, visited)
}

func (s *Store) dfsHasPath(from, to string, visited map[string]bool) bool {
	if from == to {
		return true
	}
	if visited[from] {
		return false
	}
	visited[from] = true

	rows, err := s.db.Query(
		`SELECT depends_on FROM ticket_dependencies WHERE ticket_id = ?`, from,
	)
	if err != nil {
		return false
	}

	var neighbors []string
	for rows.Next() {
		var dep string
		if err := rows.Scan(&dep); err != nil {
			continue
		}
		neighbors = append(neighbors, dep)
	}
	rows.Close()

	for _, dep := range neighbors {
		if s.dfsHasPath(dep, to, visited) {
			return true
		}
	}
	return false
}
