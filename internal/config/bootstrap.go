package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// WriteBootstrapConfig writes a minimal bootstrap YAML when the config file is missing.
// jwt_secret is randomly generated (never "change-me").
func WriteBootstrapConfig(path string) error {
	secret, err := generateJWTSecret()
	if err != nil {
		return fmt.Errorf("generate jwt_secret: %w", err)
	}

	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create config directory: %w", err)
		}
	}

	content := fmt.Sprintf(`# Auto-generated bootstrap config — do not commit secrets.
# Configure Gitea / LLM in Web UI (System Config) after login.
# Full reference: config.example.yaml / config.full-example.yaml

server:
  host: "0.0.0.0"
  port: 8080

database:
  path: "./data/matea.db"

workspace:
  base_dir: "./data/work"

logging:
  level: "info"
  path: "./data"

auth:
  jwt_secret: %q
  default_admin_password: "admin123"
`, secret)

	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return fmt.Errorf("write bootstrap config: %w", err)
	}
	return nil
}

func generateJWTSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
