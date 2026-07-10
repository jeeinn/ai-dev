package dispatcher

import (
	"errors"
	"strings"
	"testing"

	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/store"
)

func TestWritebackTargetID(t *testing.T) {
	t.Run("review_pr uses PRID when issue missing", func(t *testing.T) {
		id, ok := writebackTargetID(&store.Task{TaskType: "review_pr", PRID: 3, IssueID: 0})
		if !ok || id != 3 {
			t.Fatalf("got id=%d ok=%v, want 3 true", id, ok)
		}
	})
	t.Run("review_pr prefers PRID over issue", func(t *testing.T) {
		id, ok := writebackTargetID(&store.Task{TaskType: "review_pr", PRID: 3, IssueID: 2})
		if !ok || id != 3 {
			t.Fatalf("got id=%d ok=%v, want 3 true", id, ok)
		}
	})
	t.Run("issue task uses issue ID", func(t *testing.T) {
		id, ok := writebackTargetID(&store.Task{TaskType: "solve_comment", IssueID: 2})
		if !ok || id != 2 {
			t.Fatalf("got id=%d ok=%v, want 2 true", id, ok)
		}
	})
	t.Run("no target when both zero", func(t *testing.T) {
		_, ok := writebackTargetID(&store.Task{TaskType: "review_pr", IssueID: 0, PRID: 0})
		if ok {
			t.Fatal("expected no writeback target")
		}
	})
}

func TestFormatFailureComment(t *testing.T) {
	task := &store.Task{
		ID:       23,
		AgentID:  3,
		TaskType: "analyze_issue",
	}
	err := errors.New(`runner execution: LLM call: API error 404: {"error":{"message":"model is not found"}}`)

	body := formatFailureComment(task, err)

	if !strings.Contains(body, "任务执行失败") {
		t.Fatalf("missing failure title: %s", body)
	}
	if !strings.Contains(body, "model is not found") {
		t.Fatalf("missing error detail: %s", body)
	}
	if !strings.Contains(body, "Task ID: 23") {
		t.Fatalf("missing task metadata: %s", body)
	}
}

func TestHandleTaskPanicMarksTaskFailed(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	agent := createTestAgent(t, db)
	task := &store.Task{
		AgentID:  agent.ID,
		TaskType: "analyze_issue",
		Status:   "running",
		Repo:     "owner/repo",
		IssueID:  1,
	}
	if err := db.CreateTask(task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	var failedTask *store.Task
	e := NewExecutor(1, 0, nil, db, config.DefaultAgentDefaults(), config.DefaultAgentLoopConfig())
	e.SetOnFailed(func(t *store.Task) {
		failedTask = t
	})

	e.handleTaskPanic(task, "slice bounds out of range [49:45]")

	updated, err := db.GetTask(task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if updated.Status != "failed" {
		t.Fatalf("status=%q, want failed", updated.Status)
	}
	if !strings.Contains(updated.Error, "task panicked") {
		t.Fatalf("error=%q, want panic message", updated.Error)
	}
	if failedTask == nil || failedTask.ID != task.ID {
		t.Fatal("onFailed callback not invoked")
	}
}
