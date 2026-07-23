package dispatcher

import (
	"testing"

	"github.com/jeeinn/matea/internal/webhook"
)

func TestRenderTemplateIssue(t *testing.T) {
	tmpl := `请分析 Issue #{{.Issue.Number}}: {{.Issue.Title}}
作者: {{.Issue.User.Login}}
内容: {{.Issue.Body}}`

	data := &TemplateData{
		Issue: &webhook.Issue{
			Number: 42,
			Title:  "Bug: 程序崩溃",
			Body:   "点击按钮后程序崩溃",
			User:   webhook.User{Login: "user1"},
		},
	}

	result, err := RenderTemplate(tmpl, data)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	expected := `请分析 Issue #42: Bug: 程序崩溃
作者: user1
内容: 点击按钮后程序崩溃`

	if result != expected {
		t.Errorf("Template render mismatch.\nGot:\n%s\nExpected:\n%s", result, expected)
	}
}

func TestRenderTemplatePR(t *testing.T) {
	tmpl := `PR #{{.PR.Number}}: {{.PR.Title}}
分支: {{.PR.Head.Ref}} → {{.PR.Base.Ref}}`

	data := &TemplateData{
		PR: &webhook.PullRequest{
			Number: 10,
			Title:  "Add feature",
			Head:   webhook.Branch{Ref: "feature"},
			Base:   webhook.Branch{Ref: "main"},
		},
	}

	result, err := RenderTemplate(tmpl, data)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	expected := "PR #10: Add feature\n分支: feature → main"

	if result != expected {
		t.Errorf("Template render mismatch.\nGot:\n%s\nExpected:\n%s", result, expected)
	}
}

func TestRenderTemplateComment(t *testing.T) {
	tmpl := `评论者: {{.Comment.User.Login}}
内容: {{.Comment.Body}}`

	data := &TemplateData{
		Comment: &webhook.Comment{
			Body: "这个 bug 需要修复",
			User: webhook.User{Login: "reviewer"},
		},
	}

	result, err := RenderTemplate(tmpl, data)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	expected := "评论者: reviewer\n内容: 这个 bug 需要修复"

	if result != expected {
		t.Errorf("Template render mismatch.\nGot:\n%s\nExpected:\n%s", result, expected)
	}
}

func TestRenderTemplateEmpty(t *testing.T) {
	result, err := RenderTemplate("", nil)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}
	if result != "" {
		t.Errorf("Expected empty string, got: %s", result)
	}
}

func TestRenderTemplateInvalid(t *testing.T) {
	tmpl := `{{.Invalid.Too.Deep}}`
	data := &TemplateData{}

	_, err := RenderTemplate(tmpl, data)
	if err == nil {
		t.Error("Expected error for invalid template")
	}
}

func TestBuildTemplateData(t *testing.T) {
	evt := &webhook.WebhookEvent{
		Event:  "issues",
		Action: "assigned",
		Repo: webhook.Repository{
			FullName: "owner/repo",
		},
		Issue: &webhook.Issue{
			Number: 1,
			Title:  "Test Issue",
		},
		Sender: webhook.User{Login: "user1"},
	}

	data := BuildTemplateData(evt)

	if data.Event != evt {
		t.Error("Event not set correctly")
	}
	if data.Repo.FullName != "owner/repo" {
		t.Errorf("Expected repo=owner/repo, got %s", data.Repo.FullName)
	}
	if data.Issue == nil {
		t.Error("Issue should not be nil")
	}
	if data.Issue.Number != 1 {
		t.Errorf("Expected issue number=1, got %d", data.Issue.Number)
	}
	if data.Sender.Login != "user1" {
		t.Errorf("Expected sender=user1, got %s", data.Sender.Login)
	}
}

func TestRenderTemplateRepo(t *testing.T) {
	tmpl := `仓库: {{.Repo.FullName}}`

	data := &TemplateData{
		Repo: &webhook.Repository{
			FullName: "owner/repo",
		},
	}

	result, err := RenderTemplate(tmpl, data)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	expected := "仓库: owner/repo"

	if result != expected {
		t.Errorf("Template render mismatch.\nGot:\n%s\nExpected:\n%s", result, expected)
	}
}
