package store

import (
	"fmt"
	"time"
)

// Route represents a routing rule that maps webhook events to agents.
type Route struct {
	ID        int64     `json:"id"`
	Event     string    `json:"event"`
	Action    string    `json:"action"`
	Label     string    `json:"label"`
	Assignee  string    `json:"assignee"`
	Mention   string    `json:"mention"`
	AgentID   int64     `json:"agent_id"`
	Priority  int       `json:"priority"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateRoute inserts a new route into the database.
func (db *DB) CreateRoute(r *Route) error {
	result, err := db.Exec(`INSERT INTO routes (event, action, label, assignee, mention, agent_id, priority)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.Event, r.Action, r.Label, r.Assignee, r.Mention, r.AgentID, r.Priority)
	if err != nil {
		return fmt.Errorf("insert route: %w", err)
	}
	id, _ := result.LastInsertId()
	r.ID = id
	return nil
}

// ListRoutes returns all routes.
func (db *DB) ListRoutes() ([]*Route, error) {
	rows, err := db.Query(`SELECT id, event, action, label, assignee, mention, agent_id, priority, created_at
		FROM routes ORDER BY priority DESC, id`)
	if err != nil {
		return nil, fmt.Errorf("list routes: %w", err)
	}
	defer rows.Close()

	var routes []*Route
	for rows.Next() {
		var r Route
		if err := rows.Scan(&r.ID, &r.Event, &r.Action, &r.Label, &r.Assignee, &r.Mention, &r.AgentID, &r.Priority, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan route: %w", err)
		}
		routes = append(routes, &r)
	}
	return routes, nil
}

// ListRoutesByAgentID returns all routes for a specific agent.
func (db *DB) ListRoutesByAgentID(agentID int64) ([]*Route, error) {
	rows, err := db.Query(`SELECT id, event, action, label, assignee, mention, agent_id, priority, created_at
		FROM routes WHERE agent_id = ? ORDER BY priority DESC, id`, agentID)
	if err != nil {
		return nil, fmt.Errorf("list routes by agent: %w", err)
	}
	defer rows.Close()

	var routes []*Route
	for rows.Next() {
		var r Route
		if err := rows.Scan(&r.ID, &r.Event, &r.Action, &r.Label, &r.Assignee, &r.Mention, &r.AgentID, &r.Priority, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan route: %w", err)
		}
		routes = append(routes, &r)
	}
	return routes, nil
}

// UpdateRoute updates an existing route.
func (db *DB) UpdateRoute(r *Route) error {
	_, err := db.Exec(`UPDATE routes SET event=?, action=?, label=?, assignee=?, mention=?, agent_id=?, priority=? WHERE id=?`,
		r.Event, r.Action, r.Label, r.Assignee, r.Mention, r.AgentID, r.Priority, r.ID)
	if err != nil {
		return fmt.Errorf("update route: %w", err)
	}
	return nil
}

// DeleteRoute deletes a route by ID.
func (db *DB) DeleteRoute(id int64) error {
	_, err := db.Exec("DELETE FROM routes WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete route: %w", err)
	}
	return nil
}
