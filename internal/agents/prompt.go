package agents

import (
	"fmt"
	"log"

	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/store"
)

// PromptManager manages prompt loading with priority: DB > config > built-in.
type PromptManager struct {
	db      *store.DB
	config  *config.AgentsConfig
	builtin map[string]PromptTemplate
}

// PromptTemplate represents a prompt template.
type PromptTemplate struct {
	SystemPrompt string
	UserTemplate string
}

// NewPromptManager creates a new PromptManager.
func NewPromptManager(db *store.DB, cfg *config.AgentsConfig) *PromptManager {
	pm := &PromptManager{
		db:      db,
		config:  cfg,
		builtin: make(map[string]PromptTemplate),
	}

	// Register built-in templates
	pm.registerBuiltinTemplates()

	return pm
}

// GetPrompt returns the prompt for an agent, following the priority: DB > agent > config > built-in.
func (pm *PromptManager) GetPrompt(agent *store.Agent, taskType string) (systemPrompt, userTemplate string) {
	// 1. Try DB (active prompt version)
	if pm.db != nil {
		if dbPrompt, err := pm.db.GetActivePrompt(agent.ID); err == nil && dbPrompt != nil {
			log.Printf("[DEBUG] Using DB prompt for agent %d (version %d)", agent.ID, dbPrompt.Version)
			return dbPrompt.SystemPrompt, dbPrompt.UserTemplate
		}
	}

	// 2. Try agent's own prompt (stored in agents table)
	if agent.SystemPrompt != "" {
		log.Printf("[DEBUG] Using agent prompt for agent %d", agent.ID)
		return agent.SystemPrompt, agent.UserTemplate
	}

	// 3. Try config templates
	if pm.config != nil {
		if tmpl, ok := pm.config.Templates[taskType]; ok {
			log.Printf("[DEBUG] Using config template '%s' for agent %d", taskType, agent.ID)
			return tmpl.SystemPrompt, tmpl.UserTemplate
		}
	}

	// 4. Fall back to built-in templates
	if tmpl, ok := pm.builtin[taskType]; ok {
		log.Printf("[DEBUG] Using built-in template '%s' for agent %d", taskType, agent.ID)
		return tmpl.SystemPrompt, tmpl.UserTemplate
	}

	// 5. Default fallback
	log.Printf("[DEBUG] Using default prompt for agent %d", agent.ID)
	return pm.builtin["default"].SystemPrompt, pm.builtin["default"].UserTemplate
}

// SavePrompt saves a new prompt version for an agent.
func (pm *PromptManager) SavePrompt(agentID int64, systemPrompt, userTemplate, note, createdBy string) (*store.PromptVersion, error) {
	pv := &store.PromptVersion{
		AgentID:      agentID,
		SystemPrompt: systemPrompt,
		UserTemplate: userTemplate,
		Note:         note,
		CreatedBy:    createdBy,
	}

	if err := pm.db.CreatePromptVersion(pv); err != nil {
		return nil, fmt.Errorf("create prompt version: %w", err)
	}

	return pv, nil
}

// ListVersions returns all prompt versions for an agent.
func (pm *PromptManager) ListVersions(agentID int64) ([]*store.PromptVersion, error) {
	return pm.db.ListPromptVersions(agentID)
}

// Rollback rolls back to a specific prompt version.
func (pm *PromptManager) Rollback(versionID int64) error {
	return pm.db.ActivatePromptVersion(versionID)
}

// DeleteVersion deletes a prompt version.
func (pm *PromptManager) DeleteVersion(versionID int64) error {
	return pm.db.DeletePromptVersion(versionID)
}

// registerBuiltinTemplates registers built-in prompt templates.
func (pm *PromptManager) registerBuiltinTemplates() {
	pm.builtin["default"] = PromptTemplate{
		SystemPrompt: `You are a helpful AI assistant. Analyze the provided context and respond appropriately.`,
		UserTemplate: `{{.Context}}`,
	}

	pm.builtin["analyze_issue"] = PromptTemplate{
		SystemPrompt: `You are a senior software engineer and requirements analyst.
Your task is to analyze the given issue and provide a structured analysis.

Focus on:
1. Understanding the requirement
2. Identifying potential issues or gaps
3. Suggesting implementation approach
4. Estimating complexity

Be concise and actionable.`,
		UserTemplate: `Please analyze the following issue:

## Issue #{{.Issue.Number}}: {{.Issue.Title}}

**Repository:** {{.Repo.FullName}}
**Author:** {{.Issue.User.Login}}
**State:** {{.Issue.State}}

{{.Issue.Body}}`,
	}

	pm.builtin["review_pr"] = PromptTemplate{
		SystemPrompt: `You are a senior code reviewer.
Review the given PR diff and provide constructive feedback.

Focus on:
1. Code quality and readability
2. Potential bugs or issues
3. Performance implications
4. Security concerns
5. Suggestions for improvement

Be specific and cite line numbers when possible.`,
		UserTemplate: `Please review the following PR:

## PR #{{.PR.Number}}: {{.PR.Title}}

**Repository:** {{.Repo.FullName}}
**Author:** {{.PR.User.Login}}
**Branch:** {{.PR.Head.Ref}} → {{.PR.Base.Ref}}

{{.PR.Body}}`,
	}

	pm.builtin["reply_comment"] = PromptTemplate{
		SystemPrompt: `You are a helpful AI assistant participating in a discussion.
Reply to the comment in a friendly and professional manner.
Be concise and address the specific points raised.`,
		UserTemplate: `Please reply to the following comment:

**Author:** {{.Comment.User.Login}}
**Content:**
{{.Comment.Body}}`,
	}

	pm.builtin["solve_issue"] = PromptTemplate{
		SystemPrompt: `You are a senior software engineer.
Your task is to implement a solution for the given issue.
Write clean, well-documented code that follows best practices.

After making changes:
1. Run tests to verify your changes
2. Ensure the code compiles
3. Create a clear commit message`,
		UserTemplate: `Please implement a solution for the following issue:

## Issue #{{.Issue.Number}}: {{.Issue.Title}}

**Repository:** {{.Repo.FullName}}

{{.Issue.Body}}`,
	}

	pm.builtin["fix_bug"] = PromptTemplate{
		SystemPrompt: `You are a senior software engineer specializing in debugging.
Your task is to fix the reported bug.
Identify the root cause and implement a minimal fix.

After fixing:
1. Verify the fix works
2. Ensure no regressions
3. Add tests if appropriate`,
		UserTemplate: `Please fix the following bug:

## Bug Report: Issue #{{.Issue.Number}}

**Repository:** {{.Repo.FullName}}

{{.Issue.Body}}`,
	}
}

// GetAvailableTemplates returns the names of all available templates.
func (pm *PromptManager) GetAvailableTemplates() []string {
	templates := make([]string, 0, len(pm.builtin))
	for name := range pm.builtin {
		templates = append(templates, name)
	}
	return templates
}

// GetBuiltinTemplate returns a built-in template by name.
func (pm *PromptManager) GetBuiltinTemplate(name string) (systemPrompt, userTemplate string) {
	if tmpl, ok := pm.builtin[name]; ok {
		return tmpl.SystemPrompt, tmpl.UserTemplate
	}
	return "", ""
}
