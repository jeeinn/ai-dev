package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitea-agent-gateway/internal/agents"
	"gitea-agent-gateway/internal/store"
	"gitea-agent-gateway/internal/webhook"
)

// setupRegistry creates a registry with test agents for all roles.
func setupRegistry() *agents.Registry {
	reg := agents.NewRegistry()
	reg.Refresh(&store.Agent{ID: 1, Name: "analyze-007", GiteaUsername: "analyze-007", Role: store.RoleAnalyze, Status: "active"})
	reg.Refresh(&store.Agent{ID: 2, Name: "coder-ds", GiteaUsername: "coder-ds", Role: store.RoleCoder, Status: "active"})
	reg.Refresh(&store.Agent{ID: 3, Name: "coder-claude", GiteaUsername: "coder-claude", Role: store.RoleCoder, Status: "active"})
	reg.Refresh(&store.Agent{ID: 4, Name: "reviewer-gpt", GiteaUsername: "reviewer-gpt", Role: store.RoleReview, Status: "active"})
	return reg
}

func buildIssueAssignedEvent(assignee string, labels []string) *webhook.WebhookEvent {
	var lbls []webhook.Label
	for i, l := range labels {
		lbls = append(lbls, webhook.Label{ID: i + 1, Name: l})
	}
	assigneeUser := &webhook.User{ID: 100, Login: assignee}
	return &webhook.WebhookEvent{
		Event:    "issues",
		Action:   "assigned",
		Assignee: assigneeUser,
		Repo:     webhook.Repository{FullName: "owner/repo"},
		Issue: &webhook.Issue{
			Number: 42,
			Labels: lbls,
		},
		Sender: webhook.User{ID: 1, Login: "human"},
	}
}

func buildPRReviewRequestedEvent(reviewers []string) *webhook.WebhookEvent {
	var revs []webhook.User
	for i, r := range reviewers {
		revs = append(revs, webhook.User{ID: 200 + i, Login: r})
	}
	return &webhook.WebhookEvent{
		Event:  "pull_request",
		Action: "review_requested",
		Repo:   webhook.Repository{FullName: "owner/repo"},
		PR: &webhook.PullRequest{
			Number:             10,
			Body:               "Fixes #5",
			RequestedReviewers: revs,
		},
		Sender: webhook.User{ID: 1, Login: "coder-ds"},
	}
}

func TestResolveAssignedAnalyze(t *testing.T) {
	reg := setupRegistry()
	resolver := NewResolver(reg)

	evt := buildIssueAssignedEvent("analyze-007", nil)
	result := resolver.Resolve(evt)

	require.NotNil(t, result)
	assert.Equal(t, int64(1), result.Agent.ID)
	assert.Equal(t, "analyze_issue", result.TaskType)
	assert.Equal(t, store.RoleAnalyze, result.Role)
	assert.Equal(t, 42, result.IssueID)
}

func TestResolveAssignedCoderNoBug(t *testing.T) {
	reg := setupRegistry()
	resolver := NewResolver(reg)

	evt := buildIssueAssignedEvent("coder-ds", []string{"feature"})
	result := resolver.Resolve(evt)

	require.NotNil(t, result)
	assert.Equal(t, int64(2), result.Agent.ID)
	assert.Equal(t, "solve_issue", result.TaskType)
	assert.Equal(t, store.RoleCoder, result.Role)
}

func TestResolveAssignedCoderWithBug(t *testing.T) {
	reg := setupRegistry()
	resolver := NewResolver(reg)

	evt := buildIssueAssignedEvent("coder-ds", []string{"bug", "backend"})
	result := resolver.Resolve(evt)

	require.NotNil(t, result)
	assert.Equal(t, "fix_bug", result.TaskType)
}

func TestResolveAssignedUnknownUser(t *testing.T) {
	reg := setupRegistry()
	resolver := NewResolver(reg)

	evt := buildIssueAssignedEvent("random-user", nil)
	result := resolver.Resolve(evt)

	assert.Nil(t, result)
}

func TestResolveAssignedNoAssigneeField(t *testing.T) {
	reg := setupRegistry()
	resolver := NewResolver(reg)

	evt := &webhook.WebhookEvent{
		Event:  "issues",
		Action: "assigned",
		Repo:   webhook.Repository{FullName: "owner/repo"},
		Issue:  &webhook.Issue{Number: 1},
		Sender: webhook.User{Login: "human"},
	}
	result := resolver.Resolve(evt)
	assert.Nil(t, result)
}

func TestResolveReviewRequested(t *testing.T) {
	reg := setupRegistry()
	resolver := NewResolver(reg)

	evt := buildPRReviewRequestedEvent([]string{"reviewer-gpt"})
	result := resolver.Resolve(evt)

	require.NotNil(t, result)
	assert.Equal(t, int64(4), result.Agent.ID)
	assert.Equal(t, "review_pr", result.TaskType)
	assert.Equal(t, store.RoleReview, result.Role)
	assert.Equal(t, 10, result.PRID)
	assert.Equal(t, 5, result.IssueID) // From "Fixes #5" in PR body
}

func TestResolveReviewRequestedUnknownReviewer(t *testing.T) {
	reg := setupRegistry()
	resolver := NewResolver(reg)

	evt := buildPRReviewRequestedEvent([]string{"random-person"})
	result := resolver.Resolve(evt)

	assert.Nil(t, result)
}

func TestResolveReviewRequestedNonReviewAgent(t *testing.T) {
	reg := setupRegistry()
	resolver := NewResolver(reg)

	// coder-ds is not a review agent
	evt := buildPRReviewRequestedEvent([]string{"coder-ds"})
	result := resolver.Resolve(evt)

	assert.Nil(t, result)
}

func TestResolvePROpenedWithReviewer(t *testing.T) {
	reg := setupRegistry()
	resolver := NewResolver(reg)

	evt := buildPRReviewRequestedEvent([]string{"reviewer-gpt"})
	evt.Action = "opened"
	result := resolver.Resolve(evt)

	require.NotNil(t, result)
	assert.Equal(t, "review_pr", result.TaskType)
}

func TestResolvePRSynchronizeIgnored(t *testing.T) {
	reg := setupRegistry()
	resolver := NewResolver(reg)

	evt := buildPRReviewRequestedEvent([]string{"reviewer-gpt"})
	evt.Action = "synchronize"
	result := resolver.Resolve(evt)

	assert.Nil(t, result)
}

func TestResolveUnassignedIgnored(t *testing.T) {
	reg := setupRegistry()
	resolver := NewResolver(reg)

	evt := &webhook.WebhookEvent{
		Event:    "issues",
		Action:   "unassigned",
		Assignee: &webhook.User{Login: "analyze-007"},
		Repo:     webhook.Repository{FullName: "owner/repo"},
		Issue:    &webhook.Issue{Number: 1},
		Sender:   webhook.User{Login: "human"},
	}
	result := resolver.Resolve(evt)
	assert.Nil(t, result)
}

func TestResolveLabeledIgnored(t *testing.T) {
	reg := setupRegistry()
	resolver := NewResolver(reg)

	evt := &webhook.WebhookEvent{
		Event:  "issues",
		Action: "labeled",
		Repo:   webhook.Repository{FullName: "owner/repo"},
		Issue:  &webhook.Issue{Number: 1},
		Sender: webhook.User{Login: "human"},
	}
	result := resolver.Resolve(evt)
	assert.Nil(t, result)
}

func TestResolveCommentWithMention(t *testing.T) {
	reg := setupRegistry()
	resolver := NewResolver(reg)

	evt := &webhook.WebhookEvent{
		Event:   "issue_comment",
		Action:  "created",
		Repo:    webhook.Repository{FullName: "owner/repo"},
		Issue:   &webhook.Issue{Number: 5},
		Comment: &webhook.Comment{Body: "@coder-ds please fix this"},
		Sender:  webhook.User{Login: "human"},
	}
	result := resolver.Resolve(evt)
	require.NotNil(t, result)
	assert.Equal(t, "coder-ds", result.Agent.GiteaUsername)
	assert.Equal(t, "solve_comment", result.TaskType)
}

func TestResolveCommentNoMention(t *testing.T) {
	reg := setupRegistry()
	resolver := NewResolver(reg)

	evt := &webhook.WebhookEvent{
		Event:   "issue_comment",
		Action:  "created",
		Repo:    webhook.Repository{FullName: "owner/repo"},
		Issue:   &webhook.Issue{Number: 5},
		Comment: &webhook.Comment{Body: "just a regular comment"},
		Sender:  webhook.User{Login: "human"},
	}
	result := resolver.Resolve(evt)
	assert.Nil(t, result) // No @mention → ignore
}

func TestResolveLinkedIssueFromPRBody(t *testing.T) {
	reg := setupRegistry()
	resolver := NewResolver(reg)

	tests := []struct {
		body     string
		expected int
	}{
		{"Fixes #42", 42},
		{"Closes #100", 100},
		{"Resolves #7", 7},
		{"fixes #99", 99}, // case insensitive
		{"Fixed #55", 55}, // past tense
		{"No linked issue", 0},
		{"", 0},
	}

	for _, tt := range tests {
		evt := buildPRReviewRequestedEvent([]string{"reviewer-gpt"})
		evt.PR.Body = tt.body
		result := resolver.Resolve(evt)
		require.NotNil(t, result, "body: %q", tt.body)
		assert.Equal(t, tt.expected, result.IssueID, "body: %q", tt.body)
	}
}

func TestIsAgentSender(t *testing.T) {
	reg := setupRegistry()
	resolver := NewResolver(reg)

	evt := &webhook.WebhookEvent{
		Sender: webhook.User{Login: "coder-ds"},
	}
	assert.True(t, resolver.IsAgentSender(evt))

	evt.Sender.Login = "human-user"
	assert.False(t, resolver.IsAgentSender(evt))
}

func TestResolveMultipleReviewersFirstMatch(t *testing.T) {
	reg := setupRegistry()
	resolver := NewResolver(reg)

	// Multiple reviewers, first non-agent is skipped, review agent is found
	evt := buildPRReviewRequestedEvent([]string{"random-person", "reviewer-gpt"})
	result := resolver.Resolve(evt)

	require.NotNil(t, result)
	assert.Equal(t, "reviewer-gpt", result.Agent.GiteaUsername)
}
