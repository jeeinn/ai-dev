package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResetTask(t *testing.T) {
	db := newTestDB(t)

	agent := &Agent{
		Name: "t", GiteaUsername: "t-user", GiteaToken: "tok",
		Provider: "deepseek", Model: "m", MaxOutputTokens: 1024, MaxInputTokens: 2048,
		Temperature: 0.3, Timeout: "5m", Role: RoleCoder, Status: "active",
	}
	require.NoError(t, db.CreateAgent(agent))

	task := &Task{
		Event:    "issues",
		Repo:     "owner/repo",
		IssueID:  42,
		AgentID:  agent.ID,
		TaskType: "solve_comment",
		Status:   "running",
	}
	require.NoError(t, db.CreateTask(task))

	ok, err := db.HasPendingOrRunningTask("owner/repo", 42)
	require.NoError(t, err)
	assert.True(t, ok)

	updated, err := db.ResetTask(task.ID, "test reset")
	require.NoError(t, err)
	assert.Equal(t, "failed", updated.Status)
	assert.Equal(t, "test reset", updated.Error)

	ok, err = db.HasPendingOrRunningTask("owner/repo", 42)
	require.NoError(t, err)
	assert.False(t, ok)

	_, err = db.ResetTask(task.ID, "again")
	require.Error(t, err)
}

func TestResetTasksByIssue(t *testing.T) {
	db := newTestDB(t)

	agent := &Agent{
		Name: "t", GiteaUsername: "t-user", GiteaToken: "tok",
		Provider: "deepseek", Model: "m", MaxOutputTokens: 1024, MaxInputTokens: 2048,
		Temperature: 0.3, Timeout: "5m", Role: RoleCoder, Status: "active",
	}
	require.NoError(t, db.CreateAgent(agent))

	t1 := &Task{Event: "issues", Repo: "owner/repo", IssueID: 7, AgentID: agent.ID, TaskType: "solve_comment", Status: "running"}
	t2 := &Task{Event: "issues", Repo: "owner/repo", IssueID: 7, AgentID: agent.ID, TaskType: "solve_comment", Status: "pending"}
	t3 := &Task{Event: "issues", Repo: "owner/repo", IssueID: 8, AgentID: agent.ID, TaskType: "solve_comment", Status: "running"}
	require.NoError(t, db.CreateTask(t1))
	require.NoError(t, db.CreateTask(t2))
	require.NoError(t, db.CreateTask(t3))

	n, err := db.ResetTasksByIssue("owner/repo", 7, "workflow reset")
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	got1, err := db.GetTask(t1.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", got1.Status)

	got3, err := db.GetTask(t3.ID)
	require.NoError(t, err)
	assert.Equal(t, "running", got3.Status)
}

func TestFailOrphanedRunningTasks(t *testing.T) {
	db := newTestDB(t)
	agent := &Agent{
		Name: "t2", GiteaUsername: "t2-user", GiteaToken: "tok",
		Provider: "deepseek", Model: "m", MaxOutputTokens: 1024, MaxInputTokens: 2048,
		Temperature: 0.3, Timeout: "5m", Role: RoleAnalyze, Status: "active",
	}
	require.NoError(t, db.CreateAgent(agent))

	task := &Task{
		Event: "issues", Repo: "o/r", IssueID: 1, AgentID: agent.ID,
		TaskType: "analyze_issue", Status: "running",
	}
	require.NoError(t, db.CreateTask(task))

	n, err := db.FailOrphanedRunningTasks("restart")
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	got, err := db.GetTask(task.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", got.Status)
}
