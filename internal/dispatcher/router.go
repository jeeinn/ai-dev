package dispatcher

import (
	"log"

	"gitea-agent-gateway/internal/store"
	"gitea-agent-gateway/internal/webhook"
)

// Router matches webhook events to agents based on routes in DB.
type Router struct {
	db *store.DB
}

// NewRouter creates a new Router.
func NewRouter(db *store.DB) *Router {
	return &Router{db: db}
}

// MatchResult contains the matched agent and route.
type MatchResult struct {
	Agent *store.Agent
	Route *store.Route
}

// Match finds the first matching route for the given event.
// Matching priority: higher priority first, then by ID.
// Conditions: event type, action, assignee, mention.
// NOTE: Label matching removed in v2 — Label trigger is deprecated.
func (r *Router) Match(evt *webhook.WebhookEvent) *MatchResult {
	routes, err := r.db.ListRoutes()
	if err != nil {
		log.Printf("[ERROR] Failed to list routes: %v", err)
		return nil
	}

	for _, route := range routes {
		if route.Event != evt.Event {
			continue
		}
		if route.Action != "" && route.Action != evt.Action {
			continue
		}
		// v2: Label matching removed — Label trigger is deprecated
		if route.Assignee != "" && !evt.HasAssignee(route.Assignee) {
			continue
		}
		if route.Mention != "" && !evt.HasMention(route.Mention) {
			continue
		}

		agent, err := r.db.GetAgent(route.AgentID)
		if err != nil {
			log.Printf("[WARN] Agent %d not found for route %d", route.AgentID, route.ID)
			continue
		}
		if agent.Status != "active" {
			continue
		}

		return &MatchResult{Agent: agent, Route: route}
	}

	return nil
}
