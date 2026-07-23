package workflow

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeeinn/matea/internal/store"
)

func newTestDB(t *testing.T) *store.DB {
	t.Helper()
	tmpDB, err := os.CreateTemp("", "workflow-test-*.db")
	require.NoError(t, err)
	tmpDB.Close()

	db, err := store.Open(tmpDB.Name())
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
		os.Remove(tmpDB.Name())
	})

	return db
}

func TestTransitionAnalyze(t *testing.T) {
	db := newTestDB(t)
	mgr := NewWorkflowManager(db)

	tests := []struct {
		name          string
		currentStage  string
		expectAllowed bool
		expectStage   string
	}{
		{"idle → analyzing", store.StageIdle, true, store.StageAnalyzing},
		{"analyzed → analyzing", store.StageAnalyzed, true, store.StageAnalyzing},
		{"done → analyzing", store.StageDone, true, store.StageAnalyzing},
		{"analyzing → skip", store.StageAnalyzing, false, ""},
		{"developing → analyzing (soft)", store.StageDeveloping, true, store.StageAnalyzing},
		{"reviewing → analyzing (soft)", store.StageReviewing, true, store.StageAnalyzing},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &store.WorkflowContext{Stage: tt.currentStage}
			result := mgr.Transition(ctx, store.RoleAnalyze)
			assert.Equal(t, tt.expectAllowed, result.Allowed)
			if tt.expectAllowed {
				assert.Equal(t, tt.expectStage, result.NewStage)
			}
		})
	}
}

func TestTransitionCoder(t *testing.T) {
	db := newTestDB(t)
	mgr := NewWorkflowManager(db)

	tests := []struct {
		name          string
		currentStage  string
		expectAllowed bool
		expectStage   string
	}{
		{"idle → developing", store.StageIdle, true, store.StageDeveloping},
		{"analyzed → developing", store.StageAnalyzed, true, store.StageDeveloping},
		{"developing → developing (re-run)", store.StageDeveloping, true, store.StageDeveloping},
		{"reviewing → developing", store.StageReviewing, true, store.StageDeveloping},
		{"done → developing", store.StageDone, true, store.StageDeveloping},
		{"analyzing → blocked", store.StageAnalyzing, false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &store.WorkflowContext{Stage: tt.currentStage}
			result := mgr.Transition(ctx, store.RoleCoder)
			assert.Equal(t, tt.expectAllowed, result.Allowed)
			if tt.expectAllowed {
				assert.Equal(t, tt.expectStage, result.NewStage)
			}
		})
	}
}

func TestTransitionReview(t *testing.T) {
	db := newTestDB(t)
	mgr := NewWorkflowManager(db)

	// Review is always allowed (L1 checks structural requirements)
	stages := []string{store.StageIdle, store.StageAnalyzing, store.StageAnalyzed, store.StageDeveloping, store.StageReviewing, store.StageDone}
	for _, stage := range stages {
		ctx := &store.WorkflowContext{Stage: stage}
		result := mgr.Transition(ctx, store.RoleReview)
		assert.True(t, result.Allowed, "stage %s should allow review", stage)
		assert.Equal(t, store.StageReviewing, result.NewStage)
	}
}

func TestTransitionUnknownRole(t *testing.T) {
	db := newTestDB(t)
	mgr := NewWorkflowManager(db)

	ctx := &store.WorkflowContext{Stage: store.StageIdle}
	result := mgr.Transition(ctx, "unknown")
	assert.False(t, result.Allowed)
}

func TestOnTaskCompleteAnalyze(t *testing.T) {
	db := newTestDB(t)
	mgr := NewWorkflowManager(db)

	// Create context
	ctx, err := db.GetOrCreateWorkflowContext("owner/repo", 1)
	require.NoError(t, err)
	ctx.Stage = store.StageAnalyzing
	require.NoError(t, db.UpdateWorkflowContext(ctx))

	// Complete analyze task
	err = mgr.OnTaskComplete(ctx, "analyze_issue", 0, "sess-1")
	require.NoError(t, err)

	// Verify stage changed to analyzed
	got, err := db.GetWorkflowContext("owner/repo", 1)
	require.NoError(t, err)
	assert.Equal(t, store.StageAnalyzed, got.Stage)
}

func TestOnTaskCompleteSolveIssue(t *testing.T) {
	db := newTestDB(t)
	mgr := NewWorkflowManager(db)

	ctx, err := db.GetOrCreateWorkflowContext("owner/repo", 2)
	require.NoError(t, err)
	ctx.Stage = store.StageDeveloping
	require.NoError(t, db.UpdateWorkflowContext(ctx))

	// Complete solve_issue with PR
	err = mgr.OnTaskComplete(ctx, "solve_issue", 88, "sess-2")
	require.NoError(t, err)

	got, err := db.GetWorkflowContext("owner/repo", 2)
	require.NoError(t, err)
	assert.Equal(t, store.StageDeveloping, got.Stage) // Stays developing
	assert.Equal(t, 88, got.PRID)                     // PR ID written
}

func TestOnTaskCompleteReview(t *testing.T) {
	db := newTestDB(t)
	mgr := NewWorkflowManager(db)

	ctx, err := db.GetOrCreateWorkflowContext("owner/repo", 3)
	require.NoError(t, err)
	ctx.Stage = store.StageReviewing
	require.NoError(t, db.UpdateWorkflowContext(ctx))

	err = mgr.OnTaskComplete(ctx, "review_pr", 0, "sess-3")
	require.NoError(t, err)

	got, err := db.GetWorkflowContext("owner/repo", 3)
	require.NoError(t, err)
	assert.Equal(t, store.StageReviewing, got.Stage) // Stays reviewing
	assert.Equal(t, 0, got.PRID)                     // No PRID passed
}

func TestOnTaskCompleteReviewWithPRID(t *testing.T) {
	db := newTestDB(t)
	mgr := NewWorkflowManager(db)

	ctx, err := db.GetOrCreateWorkflowContext("owner/repo", 30)
	require.NoError(t, err)
	ctx.Stage = store.StageReviewing
	require.NoError(t, db.UpdateWorkflowContext(ctx))

	// Complete review_pr with PR ID (P0 fix: PRID should be written)
	err = mgr.OnTaskComplete(ctx, "review_pr", 42, "sess-30")
	require.NoError(t, err)

	got, err := db.GetWorkflowContext("owner/repo", 30)
	require.NoError(t, err)
	assert.Equal(t, store.StageReviewing, got.Stage) // Stays reviewing
	assert.Equal(t, 42, got.PRID)                    // PR ID written
}

func TestApplyTransition(t *testing.T) {
	db := newTestDB(t)
	mgr := NewWorkflowManager(db)

	ctx, err := db.GetOrCreateWorkflowContext("owner/repo", 10)
	require.NoError(t, err)

	agent := &store.Agent{Name: "analyzer", GiteaUsername: "analyzer", GiteaToken: "t", Role: store.RoleAnalyze, Status: "active"}
	require.NoError(t, db.CreateAgent(agent))

	result := mgr.Transition(ctx, store.RoleAnalyze)
	require.True(t, result.Allowed)

	err = mgr.ApplyTransition(ctx, result, agent.ID, store.RoleAnalyze, "sess-apply")
	require.NoError(t, err)

	got, err := db.GetWorkflowContext("owner/repo", 10)
	require.NoError(t, err)
	assert.Equal(t, store.StageAnalyzing, got.Stage)
	assert.Equal(t, agent.ID, got.ActiveAgentID)
	assert.Equal(t, store.RoleAnalyze, got.ActiveRole)
	assert.Equal(t, "sess-apply", got.SessionID)
}

func TestOnTaskFailedAnalyzeFromIdle(t *testing.T) {
	db := newTestDB(t)
	mgr := NewWorkflowManager(db)

	ctx, err := db.GetOrCreateWorkflowContext("owner/repo", 20)
	require.NoError(t, err)

	agent := &store.Agent{Name: "analyzer", GiteaUsername: "analyzer", GiteaToken: "t", Role: store.RoleAnalyze, Status: "active"}
	require.NoError(t, db.CreateAgent(agent))

	result := mgr.Transition(ctx, store.RoleAnalyze)
	require.True(t, result.Allowed)
	require.NoError(t, mgr.ApplyTransition(ctx, result, agent.ID, store.RoleAnalyze, "sess-fail"))

	got, err := db.GetWorkflowContext("owner/repo", 20)
	require.NoError(t, err)
	assert.Equal(t, store.StageAnalyzing, got.Stage)
	assert.Equal(t, store.StageIdle, got.PreviousStage)

	require.NoError(t, mgr.OnTaskFailed(got, "analyze_issue"))

	got, err = db.GetWorkflowContext("owner/repo", 20)
	require.NoError(t, err)
	assert.Equal(t, store.StageIdle, got.Stage)
	assert.Equal(t, "", got.PreviousStage)
	assert.Equal(t, int64(0), got.ActiveAgentID)
}

func TestOnTaskFailedAnalyzeFromAnalyzed(t *testing.T) {
	db := newTestDB(t)
	mgr := NewWorkflowManager(db)

	ctx, err := db.GetOrCreateWorkflowContext("owner/repo", 21)
	require.NoError(t, err)
	ctx.Stage = store.StageAnalyzed
	require.NoError(t, db.UpdateWorkflowContext(ctx))

	agent := &store.Agent{Name: "analyzer", GiteaUsername: "analyzer", GiteaToken: "t", Role: store.RoleAnalyze, Status: "active"}
	require.NoError(t, db.CreateAgent(agent))

	result := mgr.Transition(ctx, store.RoleAnalyze)
	require.True(t, result.Allowed)
	require.NoError(t, mgr.ApplyTransition(ctx, result, agent.ID, store.RoleAnalyze, "sess-fail2"))

	require.NoError(t, mgr.OnTaskFailed(ctx, "analyze_issue"))

	got, err := db.GetWorkflowContext("owner/repo", 21)
	require.NoError(t, err)
	assert.Equal(t, store.StageAnalyzed, got.Stage)
}

func TestOnTaskFailedAnalyzeFromDeveloping(t *testing.T) {
	db := newTestDB(t)
	mgr := NewWorkflowManager(db)

	ctx, err := db.GetOrCreateWorkflowContext("owner/repo", 22)
	require.NoError(t, err)
	ctx.Stage = store.StageDeveloping
	require.NoError(t, db.UpdateWorkflowContext(ctx))

	agent := &store.Agent{Name: "analyzer", GiteaUsername: "analyzer", GiteaToken: "t", Role: store.RoleAnalyze, Status: "active"}
	require.NoError(t, db.CreateAgent(agent))

	result := mgr.Transition(ctx, store.RoleAnalyze)
	require.True(t, result.Allowed)
	require.NoError(t, mgr.ApplyTransition(ctx, result, agent.ID, store.RoleAnalyze, "sess-fail3"))

	require.NoError(t, mgr.OnTaskFailed(ctx, "analyze_issue"))

	got, err := db.GetWorkflowContext("owner/repo", 22)
	require.NoError(t, err)
	assert.Equal(t, store.StageDeveloping, got.Stage)
}

func TestOnTaskFailedSolveIssueNoStageChange(t *testing.T) {
	db := newTestDB(t)
	mgr := NewWorkflowManager(db)

	ctx, err := db.GetOrCreateWorkflowContext("owner/repo", 23)
	require.NoError(t, err)
	ctx.Stage = store.StageDeveloping
	require.NoError(t, db.UpdateWorkflowContext(ctx))

	require.NoError(t, mgr.OnTaskFailed(ctx, "solve_issue"))

	got, err := db.GetWorkflowContext("owner/repo", 23)
	require.NoError(t, err)
	assert.Equal(t, store.StageDeveloping, got.Stage)
}
