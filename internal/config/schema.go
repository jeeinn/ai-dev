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
	MaxConcurrent int `yaml:"max_concurrent"`
	RetryCount    int `yaml:"retry_count"`
	Timeout       int `yaml:"timeout"`
	QueueSize     int `yaml:"queue_size"`
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
	Defaults  AgentDefaultsConfig      `yaml:"defaults"`
}

type ProviderConfig struct {
	BaseURL string `yaml:"base_url"`
	APIKey  string `yaml:"api_key"`
}

type AgentDefaultsConfig struct {
	Provider    string  `yaml:"provider"`
	Model       string  `yaml:"model"`
	MaxTokens   int     `yaml:"max_tokens"`
	Temperature float64 `yaml:"temperature"`
}
