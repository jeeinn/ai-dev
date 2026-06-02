package agents

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/store"
)

func TestPromptManagerGetPromptFromBuiltin(t *testing.T) {
	// Create a mock DB (nil for this test, we'll use built-in templates)
	cfg := &config.AgentsConfig{
		Templates: map[string]config.AgentTemplateConfig{},
	}

	pm := NewPromptManager(nil, cfg)

	agent := &store.Agent{
		ID:           1,
		SystemPrompt: "",
		UserTemplate: "",
	}

	// Should use built-in template
	systemPrompt, userTemplate := pm.GetPrompt(agent, "analyze_issue")
	assert.NotEmpty(t, systemPrompt)
	assert.NotEmpty(t, userTemplate)
	assert.Contains(t, systemPrompt, "senior software engineer")
}

func TestPromptManagerGetPromptFromAgent(t *testing.T) {
	cfg := &config.AgentsConfig{
		Templates: map[string]config.AgentTemplateConfig{},
	}

	pm := NewPromptManager(nil, cfg)

	agent := &store.Agent{
		ID:           1,
		SystemPrompt: "Custom system prompt",
		UserTemplate: "Custom user template",
	}

	// Should use agent's own prompt
	systemPrompt, userTemplate := pm.GetPrompt(agent, "analyze_issue")
	assert.Equal(t, "Custom system prompt", systemPrompt)
	assert.Equal(t, "Custom user template", userTemplate)
}

func TestPromptManagerGetPromptFromConfig(t *testing.T) {
	cfg := &config.AgentsConfig{
		Templates: map[string]config.AgentTemplateConfig{
			"analyze_issue": {
				SystemPrompt: "Config system prompt",
				UserTemplate: "Config user template",
			},
		},
	}

	pm := NewPromptManager(nil, cfg)

	agent := &store.Agent{
		ID:           1,
		SystemPrompt: "",
		UserTemplate: "",
	}

	// Should use config template
	systemPrompt, userTemplate := pm.GetPrompt(agent, "analyze_issue")
	assert.Equal(t, "Config system prompt", systemPrompt)
	assert.Equal(t, "Config user template", userTemplate)
}

func TestPromptManagerGetPromptDefault(t *testing.T) {
	cfg := &config.AgentsConfig{
		Templates: map[string]config.AgentTemplateConfig{},
	}

	pm := NewPromptManager(nil, cfg)

	agent := &store.Agent{
		ID:           1,
		SystemPrompt: "",
		UserTemplate: "",
	}

	// Should use default template for unknown task type
	systemPrompt, userTemplate := pm.GetPrompt(agent, "unknown_task")
	assert.NotEmpty(t, systemPrompt)
	assert.NotEmpty(t, userTemplate)
}

func TestPromptManagerGetAvailableTemplates(t *testing.T) {
	cfg := &config.AgentsConfig{
		Templates: map[string]config.AgentTemplateConfig{},
	}

	pm := NewPromptManager(nil, cfg)

	templates := pm.GetAvailableTemplates()
	assert.Contains(t, templates, "default")
	assert.Contains(t, templates, "analyze_issue")
	assert.Contains(t, templates, "review_pr")
	assert.Contains(t, templates, "reply_comment")
	assert.Contains(t, templates, "solve_issue")
	assert.Contains(t, templates, "fix_bug")
}

func TestPromptManagerBuiltinTemplatesContent(t *testing.T) {
	cfg := &config.AgentsConfig{
		Templates: map[string]config.AgentTemplateConfig{},
	}

	pm := NewPromptManager(nil, cfg)

	// Test analyze_issue template
	agent := &store.Agent{ID: 1}
	systemPrompt, _ := pm.GetPrompt(agent, "analyze_issue")
	assert.Contains(t, systemPrompt, "requirements analyst")

	// Test review_pr template
	systemPrompt, _ = pm.GetPrompt(agent, "review_pr")
	assert.Contains(t, systemPrompt, "code reviewer")

	// Test solve_issue template
	systemPrompt, _ = pm.GetPrompt(agent, "solve_issue")
	assert.Contains(t, systemPrompt, "implement a solution")

	// Test fix_bug template
	systemPrompt, _ = pm.GetPrompt(agent, "fix_bug")
	assert.Contains(t, systemPrompt, "fix the reported bug")
}

func TestPromptManagerPriority(t *testing.T) {
	// Test priority: agent > config > built-in
	cfg := &config.AgentsConfig{
		Templates: map[string]config.AgentTemplateConfig{
			"analyze_issue": {
				SystemPrompt: "Config prompt",
				UserTemplate: "Config template",
			},
		},
	}

	pm := NewPromptManager(nil, cfg)

	// Agent with custom prompt should take priority
	agent := &store.Agent{
		ID:           1,
		SystemPrompt: "Agent prompt",
		UserTemplate: "Agent template",
	}

	systemPrompt, userTemplate := pm.GetPrompt(agent, "analyze_issue")
	assert.Equal(t, "Agent prompt", systemPrompt)
	assert.Equal(t, "Agent template", userTemplate)

	// Agent without custom prompt should use config
	agent2 := &store.Agent{
		ID:           2,
		SystemPrompt: "",
		UserTemplate: "",
	}

	systemPrompt, userTemplate = pm.GetPrompt(agent2, "analyze_issue")
	assert.Equal(t, "Config prompt", systemPrompt)
	assert.Equal(t, "Config template", userTemplate)
}
