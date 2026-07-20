package integration

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitea-agent-gateway/internal/store"
	"gitea-agent-gateway/internal/workflow"
)

func cleanupUnassignEnv(env *TestEnv) {
	env.Dispatcher.Shutdown()
	time.Sleep(200 * time.Millisecond) // let in-flight coder tasks finish before DB close
	env.Cleanup()
}

func assignIssuePayload(issueNum int, assigneeLogin string) map[string]interface{} {
	return map[string]interface{}{
		"action":   "assigned",
		"issue":    buildIssuePayload(issueNum, fmt.Sprintf("Issue %d", issueNum), nil),
		"assignee": map[string]interface{}{"id": 100, "login": assigneeLogin},
		"repository": map[string]interface{}{
			"id": 1, "name": "repo", "full_name": "owner/repo",
			"clone_url": "http://localhost:3000/owner/repo.git",
		},
		"sender": map[string]interface{}{"id": 1, "login": "human"},
	}
}

func waitForUnassign(t *testing.T, rec *RecordingGiteaMock, want int, timeout time.Duration) []GiteaUnassignCall {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		calls := rec.UnassignCalls()
		if len(calls) >= want {
			return calls
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("expected >= %d unassign call(s) within %v, got %d", want, timeout, len(rec.UnassignCalls()))
	return nil
}

// TestStageTransitionUnassign_CallsGitea verifies analyze→coder transition
// removes the previous agent via DELETE .../assignees when gate is soft/hard.
func TestStageTransitionUnassign_CallsGitea(t *testing.T) {
	env := NewTestEnv(t)
	defer cleanupUnassignEnv(env)
	rec := env.InstallRecordingGitea(t)

	analyze := env.CreateTestAgentWithRole(t, "analyze-007", "analyze-007", store.RoleAnalyze)
	env.CreateTestAgentWithRole(t, "coder-ds", "coder-ds", store.RoleCoder)
	env.EnableWorkflowV2WithPolicy(t, workflow.PresetStandard()) // stage_transition_unassign=soft

	require.NoError(t, env.Dispatcher.Start())

	issueNum := 201
	require.NoError(t, env.SendWebhook("issues", "unassign-analyze-001", assignIssuePayload(issueNum, "analyze-007")))
	env.WaitForTask(t, 1, "success", 10*time.Second)

	ctx, err := env.DB.GetWorkflowContext("owner/repo", issueNum)
	require.NoError(t, err)
	require.Equal(t, store.StageAnalyzed, ctx.Stage)
	require.Equal(t, analyze.ID, ctx.ActiveAgentID)

	require.NoError(t, env.SendWebhook("issues", "unassign-coder-001", assignIssuePayload(issueNum, "coder-ds")))

	calls := waitForUnassign(t, rec, 1, 5*time.Second)
	require.Len(t, calls, 1)
	assert.Equal(t, []string{"analyze-007"}, calls[0].Assignees)
	assert.Equal(t, fmt.Sprintf("/api/v1/repos/owner/repo/issues/%d/assignees", issueNum), calls[0].Path)

	// Coder task should still be enqueued (unassign failure would not apply here)
	require.Eventually(t, func() bool {
		tasks, err := env.DB.ListTasks(10, 0)
		return err == nil && len(tasks) >= 2
	}, 5*time.Second, 100*time.Millisecond)
}

// TestStageTransitionUnassign_GateOff skips Gitea unassign when policy is free.
func TestStageTransitionUnassign_GateOff(t *testing.T) {
	env := NewTestEnv(t)
	defer cleanupUnassignEnv(env)
	rec := env.InstallRecordingGitea(t)

	env.CreateTestAgentWithRole(t, "analyze-007", "analyze-007", store.RoleAnalyze)
	env.CreateTestAgentWithRole(t, "coder-ds", "coder-ds", store.RoleCoder)
	env.EnableWorkflowV2WithPolicy(t, workflow.PresetFree()) // stage_transition_unassign=off

	require.NoError(t, env.Dispatcher.Start())

	issueNum := 202
	require.NoError(t, env.SendWebhook("issues", "unassign-off-analyze", assignIssuePayload(issueNum, "analyze-007")))
	env.WaitForTask(t, 1, "success", 10*time.Second)

	require.NoError(t, env.SendWebhook("issues", "unassign-off-coder", assignIssuePayload(issueNum, "coder-ds")))

	require.Eventually(t, func() bool {
		tasks, err := env.DB.ListTasks(10, 0)
		return err == nil && len(tasks) >= 2
	}, 5*time.Second, 100*time.Millisecond)

	time.Sleep(300 * time.Millisecond) // allow any accidental async unassign
	assert.Empty(t, rec.UnassignCalls(), "gate off must not call Gitea unassign")
}

// TestStageTransitionUnassign_PerRepoOverride uses DB policy over global free preset.
func TestStageTransitionUnassign_PerRepoOverride(t *testing.T) {
	env := NewTestEnv(t)
	defer cleanupUnassignEnv(env)
	rec := env.InstallRecordingGitea(t)

	env.CreateTestAgentWithRole(t, "analyze-007", "analyze-007", store.RoleAnalyze)
	env.CreateTestAgentWithRole(t, "coder-ds", "coder-ds", store.RoleCoder)
	env.EnableWorkflowV2WithPolicy(t, workflow.PresetFree()) // global: unassign off

	require.NoError(t, env.DB.UpsertWorkflowPolicy("owner/repo", "standard",
		`{"stage_transition_unassign":"soft"}`))

	require.NoError(t, env.Dispatcher.Start())

	issueNum := 203
	require.NoError(t, env.SendWebhook("issues", "unassign-repo-analyze", assignIssuePayload(issueNum, "analyze-007")))
	env.WaitForTask(t, 1, "success", 10*time.Second)

	require.NoError(t, env.SendWebhook("issues", "unassign-repo-coder", assignIssuePayload(issueNum, "coder-ds")))

	calls := waitForUnassign(t, rec, 1, 5*time.Second)
	require.Len(t, calls, 1)
	assert.Equal(t, []string{"analyze-007"}, calls[0].Assignees)
}

// TestStageTransitionUnassign_HardFailurePostsWarning posts a gate comment when
// unassign fails under hard policy, without blocking the new task.
func TestStageTransitionUnassign_HardFailurePostsWarning(t *testing.T) {
	env := NewTestEnv(t)
	defer cleanupUnassignEnv(env)
	rec := env.InstallRecordingGitea(t)
	rec.FailUnassign = true

	env.CreateTestAgentWithRole(t, "analyze-007", "analyze-007", store.RoleAnalyze)
	env.CreateTestAgentWithRole(t, "coder-ds", "coder-ds", store.RoleCoder)

	// Keep standard gates (coder_switch_agent=soft) but force unassign hard for warning path.
	policy := workflow.BuildPolicy("standard", map[string]string{
		workflow.GateStageTransitionUnassign: "hard",
	})
	env.EnableWorkflowV2WithPolicy(t, policy)

	require.NoError(t, env.Dispatcher.Start())

	issueNum := 204
	require.NoError(t, env.SendWebhook("issues", "unassign-hard-analyze", assignIssuePayload(issueNum, "analyze-007")))
	env.WaitForTask(t, 1, "success", 10*time.Second)

	require.NoError(t, env.SendWebhook("issues", "unassign-hard-coder", assignIssuePayload(issueNum, "coder-ds")))

	_ = waitForUnassign(t, rec, 1, 5*time.Second)

	require.Eventually(t, func() bool {
		for _, c := range rec.CommentCalls() {
			if strings.Contains(c.Body, "未能从 Issue 移除") && strings.Contains(c.Body, "analyze-007") {
				return true
			}
		}
		return false
	}, 5*time.Second, 50*time.Millisecond)

	require.Eventually(t, func() bool {
		tasks, err := env.DB.ListTasks(10, 0)
		return err == nil && len(tasks) >= 2
	}, 5*time.Second, 100*time.Millisecond)
}
