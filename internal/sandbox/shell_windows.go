//go:build windows

package sandbox

// ExecuteShell runs a shell command via cmd /C (Windows).
// Prefer cmd for broad availability; PowerShell is still whitelisted for explicit use.
func (s *Sandbox) ExecuteShell(command string) *Result {
	return s.Execute("cmd", "/C", command)
}
