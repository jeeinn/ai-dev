package workflow

import (
	"log"
	"regexp"
	"strconv"

	"gitea-agent-gateway/internal/agents"
	"gitea-agent-gateway/internal/store"
	"gitea-agent-gateway/internal/webhook"
)

// ResolveResult holds the resolution of a webhook event.
type ResolveResult struct {
	Agent    *store.Agent
	TaskType string
	Role     string
	IssueID  int
	PRID     int
}

// Resolver resolves webhook events to agent + task type via the Assign model.
type Resolver struct {
	registry *agents.Registry
}

// NewResolver creates a new Event Resolver.
func NewResolver(registry *agents.Registry) *Resolver {
	return &Resolver{registry: registry}
}

// linkedIssuePattern matches "Fixes #N", "Closes #N", "Resolves #N" in PR bodies.
var linkedIssuePattern = regexp.MustCompile(`(?i)(?:fix(?:es|ed)?|close[sd]?|resolve[sd]?)\s+#(\d+)`)

// Resolve determines what to do with a webhook event.
// Returns nil if the event should be ignored.
func (r *Resolver) Resolve(evt *webhook.WebhookEvent) *ResolveResult {
	switch evt.Event {
	case "issues":
		return r.resolveIssue(evt)
	case "pull_request":
		return r.resolvePullRequest(evt)
	case "issue_comment", "pull_request_comment":
		// Phase 17: @mention resolution
		return nil
	default:
		return nil
	}
}

// resolveIssue handles issue events.
func (r *Resolver) resolveIssue(evt *webhook.WebhookEvent) *ResolveResult {
	switch evt.Action {
	case "assigned":
		return r.resolveAssigned(evt)
	case "unassigned":
		// v2: unassigned does not trigger tasks or revert stage
		return nil
	case "labeled", "label_updated":
		// v2: label events do not trigger tasks
		return nil
	default:
		return nil
	}
}

// resolveAssigned handles issues.assigned events.
// Uses ONLY the single assignee from the webhook payload (not the full assignees list).
func (r *Resolver) resolveAssigned(evt *webhook.WebhookEvent) *ResolveResult {
	if evt.Assignee == nil {
		log.Printf("[DEBUG] issues.assigned event with no assignee field, ignoring")
		return nil
	}

	username := evt.Assignee.Login
	agent := r.registry.GetByGiteaUsername(username)
	if agent == nil {
		log.Printf("[DEBUG] Assignee %q not in agent registry, ignoring", username)
		return nil
	}

	if agent.Role == "" {
		log.Printf("[WARN] Agent %q has no role configured, defaulting to analyze", agent.Name)
	}

	role := agent.Role
	if role == "" {
		role = store.RoleAnalyze
	}

	// Determine task type based on role
	taskType := r.taskTypeForRole(role, evt)

	issueID := 0
	if evt.Issue != nil {
		issueID = evt.Issue.Number
	}

	return &ResolveResult{
		Agent:    agent,
		TaskType: taskType,
		Role:     role,
		IssueID:  issueID,
	}
}

// resolvePullRequest handles pull_request events.
func (r *Resolver) resolvePullRequest(evt *webhook.WebhookEvent) *ResolveResult {
	if evt.PR == nil {
		return nil
	}

	// Only handle review_requested action (and opened with reviewers for convenience)
	if evt.Action != "review_requested" && evt.Action != "opened" {
		return nil
	}

	// Find a review agent among requested reviewers
	if evt.PR.RequestedReviewers == nil || len(evt.PR.RequestedReviewers) == 0 {
		return nil
	}

	for _, reviewer := range evt.PR.RequestedReviewers {
		agent := r.registry.GetByGiteaUsername(reviewer.Login)
		if agent != nil && agent.Role == store.RoleReview {
			// Try to resolve linked issue from PR body
			issueID := r.resolveLinkedIssue(evt)

			return &ResolveResult{
				Agent:    agent,
				TaskType: "review_pr",
				Role:     store.RoleReview,
				IssueID:  issueID,
				PRID:     evt.PR.Number,
			}
		}
	}

	return nil
}

// taskTypeForRole determines the task type based on agent role and event context.
func (r *Resolver) taskTypeForRole(role string, evt *webhook.WebhookEvent) string {
	switch role {
	case store.RoleAnalyze:
		return "analyze_issue"
	case store.RoleCoder:
		// Check for business label "bug" → fix_bug, otherwise solve_issue
		if evt.Issue != nil {
			for _, label := range evt.Issue.Labels {
				if label.Name == "bug" {
					return "fix_bug"
				}
			}
		}
		return "solve_issue"
	case store.RoleReview:
		return "review_pr"
	default:
		return "analyze_issue"
	}
}

// resolveLinkedIssue tries to extract the linked issue number from the PR body.
// Returns 0 if no linked issue is found.
func (r *Resolver) resolveLinkedIssue(evt *webhook.WebhookEvent) int {
	if evt.PR == nil || evt.PR.Body == "" {
		return 0
	}
	matches := linkedIssuePattern.FindStringSubmatch(evt.PR.Body)
	if len(matches) >= 2 {
		n, err := strconv.Atoi(matches[1])
		if err == nil {
			return n
		}
	}
	return 0
}

// IsAgentSender checks if the event sender is any active agent (to prevent self-trigger loops).
func (r *Resolver) IsAgentSender(evt *webhook.WebhookEvent) bool {
	return r.registry.GetByGiteaUsername(evt.Sender.Login) != nil
}
