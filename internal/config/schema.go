package config

// Config is the top-level application configuration.
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Gitea      GiteaConfig      `yaml:"gitea"`
	Workspace  WorkspaceConfig  `yaml:"workspace"`
	Dispatcher DispatcherConfig `yaml:"dispatcher"`
	Database   DatabaseConfig   `yaml:"database"`
	Logging    LoggingConfig    `yaml:"logging"`
	LLM        LLMConfig        `yaml:"llm"`
	API        APIConfig        `yaml:"api"`
	Auth       AuthConfig       `yaml:"auth"`
	Agents     AgentsConfig     `yaml:"agents"`
	Workflow   WorkflowConfig   `yaml:"workflow"`
	Session    SessionConfig    `yaml:"session"`
}

// WorkflowConfig contains workflow policy configuration.
type WorkflowConfig struct {
	Preset string            `yaml:"preset"` // free | standard | strict
	Gates  map[string]string `yaml:"gates"`  // gate_id → off|soft|hard
	Notify NotifyConfig      `yaml:"notify"`
}

// NotifyConfig controls L3 comment notifications.
type NotifyConfig struct {
	OnAnalyzeDone   bool `yaml:"on_analyze_done"`
	OnCoderPROpened bool `yaml:"on_coder_pr_opened"`
	OnGateSoft      bool `yaml:"on_gate_soft"`
	OnGateHard      bool `yaml:"on_gate_hard"`
}

// SessionConfig contains session lifecycle configuration.
type SessionConfig struct {
	IdleTTL            string `yaml:"idle_ttl"`            // Duration string, e.g. "168h" (7 days)
	WorkspaceRetention string `yaml:"workspace_retention"` // Duration string, e.g. "24h"
	PRClosedRetention  string `yaml:"pr_closed_retention"` // Duration string, e.g. "168h"
	MaxDiskPerRepo     string `yaml:"max_disk_per_repo"`   // e.g. "5GB"
}

// DefaultSessionConfig returns default session configuration.
func DefaultSessionConfig() SessionConfig {
	return SessionConfig{
		IdleTTL:            "168h", // 7 days
		WorkspaceRetention: "24h",  // 24 hours
		PRClosedRetention:  "168h", // 7 days
		MaxDiskPerRepo:     "5GB",
	}
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type GiteaConfig struct {
	URL           string `yaml:"url"`
	AdminToken    string `yaml:"admin_token"`
	WebhookSecret string `yaml:"webhook_secret"`
}

type WorkspaceConfig struct {
	BaseDir      string `yaml:"base_dir"`
	CleanupAfter string `yaml:"cleanup_after"`
	MaxDiskUsage string `yaml:"max_disk_usage"`
}

type DispatcherConfig struct {
	MaxConcurrent    int `yaml:"max_concurrent"`
	RetryCount       int `yaml:"retry_count"`
	QueueSize        int `yaml:"queue_size"`
	RateLimitBackoff int `yaml:"rate_limit_backoff"` // seconds to wait on HTTP 429; 0 = disabled
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type LoggingConfig struct {
	Level string `yaml:"level"`
	Path  string `yaml:"path"`
}

type LLMConfig struct {
	Providers map[string]ProviderConfig `yaml:"providers"`
	Defaults  LLMDefaultsConfig         `yaml:"defaults"`
}

type ProviderConfig struct {
	BaseURL string `yaml:"base_url" json:"base_url"`
	APIKey  string `yaml:"api_key" json:"api_key"`
}

// LLMDefaultsConfig holds LLM connectivity defaults (provider/model only).
type LLMDefaultsConfig struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
}

// AgentDefaultsConfig holds defaults for new agents and single-shot task budgets.
type AgentDefaultsConfig struct {
	Provider        string  `yaml:"provider"`
	Model           string  `yaml:"model"`
	MaxOutputTokens int     `yaml:"max_output_tokens"`
	MaxInputTokens  int     `yaml:"max_input_tokens"`
	Temperature     float64 `yaml:"temperature"`
	Timeout         string  `yaml:"timeout"` // Go duration, e.g. "5m" — single-shot tasks
}

// APIConfig contains API server configuration.
type APIConfig struct {
	AuthToken string `yaml:"auth_token"`
}

// AuthConfig contains authentication configuration.
type AuthConfig struct {
	JWTSecret            string `yaml:"jwt_secret"`
	JWTExpiration        string `yaml:"jwt_expiration"`
	DefaultAdminPassword string `yaml:"default_admin_password"`
}

// AgentsConfig contains agent templates and defaults.
type AgentsConfig struct {
	Defaults  AgentDefaultsConfig            `yaml:"defaults"`
	Templates map[string]AgentTemplateConfig `yaml:"templates"`
	Loop      AgentLoopConfig                `yaml:"loop"`
}

// AgentTemplateConfig is a template for creating agents.
type AgentTemplateConfig struct {
	Name         string   `yaml:"name"`
	Type         string   `yaml:"type"`
	SystemPrompt string   `yaml:"system_prompt"`
	UserTemplate string   `yaml:"user_template"`
	Permissions  []string `yaml:"permissions"`
}

// AgentLoopConfig contains agent loop configuration (multi-turn tasks only).
type AgentLoopConfig struct {
	MaxIterations     int    `yaml:"max_iterations"`     // Max iteration rounds (default 20)
	TotalTimeout      string `yaml:"total_timeout"`      // Total loop task timeout (default "30m")
	IterationInterval int    `yaml:"iteration_interval"` // Seconds between loop rounds (default 0)
}

// DefaultAgentLoopConfig returns default agent loop configuration.
func DefaultAgentLoopConfig() AgentLoopConfig {
	return AgentLoopConfig{
		MaxIterations:     20,
		TotalTimeout:      "30m",
		IterationInterval: 0,
	}
}

// DefaultAgentDefaults returns default agent budget/timeout settings.
func DefaultAgentDefaults() AgentDefaultsConfig {
	return AgentDefaultsConfig{
		Provider:        "deepseek",
		Model:           "deepseek-chat",
		MaxOutputTokens: 2048,
		MaxInputTokens:  65536,
		Temperature:     0.3,
		Timeout:         "5m",
	}
}
