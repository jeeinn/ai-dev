//go:build !windows

package sandbox

// ExecuteShell runs a shell command via sh -c (Unix/macOS).
func (s *Sandbox) ExecuteShell(command string) *Result {
	return s.Execute("sh", "-c", command)
}
