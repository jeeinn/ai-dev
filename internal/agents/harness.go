package agents

import (
	"fmt"
	"log"
	"strings"

	"gitea-agent-gateway/internal/sandbox"
)

// workspaceProgressSnapshot returns a fingerprint of uncommitted workspace state.
// Used by AgentLoop no-progress detection (git status --porcelain).
func workspaceProgressSnapshot(sb *sandbox.Sandbox) string {
	if sb == nil {
		return ""
	}
	res := sb.Execute("git", "status", "--porcelain")
	if res == nil {
		return ""
	}
	return strings.TrimSpace(res.Stdout)
}

// runHarnessVerify runs configured shell commands in the workspace after coding
// and before commit/PR. Empty cmds is a no-op. First failing command aborts.
func runHarnessVerify(sb *sandbox.Sandbox, cmds []string) error {
	if sb == nil || len(cmds) == 0 {
		return nil
	}
	for _, raw := range cmds {
		cmd := strings.TrimSpace(raw)
		if cmd == "" {
			continue
		}
		log.Printf("[INFO] Harness verify: %s", cmd)
		res := sb.ExecuteShell(cmd)
		if res.Error != nil || res.ExitCode != 0 {
			out := strings.TrimSpace(res.Stdout)
			errOut := strings.TrimSpace(res.Stderr)
			detail := errOut
			if detail == "" {
				detail = out
			}
			if detail == "" && res.Error != nil {
				detail = res.Error.Error()
			}
			if len(detail) > 4000 {
				detail = detail[:4000] + "…(truncated)"
			}
			return fmt.Errorf("verify gate failed (exit=%d): %s\n%s", res.ExitCode, cmd, detail)
		}
		log.Printf("[INFO] Harness verify OK: %s", cmd)
	}
	return nil
}
