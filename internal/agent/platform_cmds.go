package agent

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/jeeinn/matea/internal/sandbox"
)

// isCommandMissing reports whether Execute failed because the binary is not on PATH.
func isCommandMissing(result *sandbox.Result) bool {
	if result == nil || result.Error == nil {
		return false
	}
	if errors.Is(result.Error, exec.ErrNotFound) {
		return true
	}
	msg := strings.ToLower(result.Error.Error() + " " + result.Stderr)
	return strings.Contains(msg, "executable file not found") ||
		strings.Contains(msg, "not found in %path%") ||
		strings.Contains(msg, "no such file or directory")
}

// Platform returns the sandbox host OS ("windows", "linux", "darwin", ...).
func Platform() string {
	return runtime.GOOS
}

// listFilesCmd returns the platform-specific command to list files under path (max depth 3).
func listFilesCmd(path string) (cmd string, args []string) {
	if path == "" {
		path = "."
	}
	if runtime.GOOS == "windows" {
		ps := fmt.Sprintf(
			`$root = (Resolve-Path -LiteralPath '%s').Path; Get-ChildItem -LiteralPath $root -Recurse -File -Depth 3 -ErrorAction SilentlyContinue | Where-Object { $_.FullName -notmatch '\\.git\\' } | ForEach-Object { $_.FullName.Substring($root.Length).TrimStart('\') }`,
			escapePSSingle(path),
		)
		return "powershell", []string{"-NoProfile", "-NonInteractive", "-Command", ps}
	}
	return "find", []string{path, "-maxdepth", "3", "-type", "f", "-not", "-path", "*/.git/*"}
}

// treeCmd returns the platform-specific command for a directory tree listing.
func treeCmd(path string, depth int) (cmd string, args []string) {
	if path == "" {
		path = "."
	}
	if depth <= 0 {
		depth = 3
	}
	if runtime.GOOS == "windows" {
		ps := fmt.Sprintf(
			`$root = (Resolve-Path -LiteralPath '%s').Path; Get-ChildItem -LiteralPath $root -Recurse -Depth %d -ErrorAction SilentlyContinue | Where-Object { $_.FullName -notmatch '\\.git\\' } | ForEach-Object { $rel = $_.FullName.Substring($root.Length).TrimStart('\'); if ($_.PSIsContainer) { $rel + '\' } else { $rel } }`,
			escapePSSingle(path), depth,
		)
		return "powershell", []string{"-NoProfile", "-NonInteractive", "-Command", ps}
	}
	return "find", []string{path, "-maxdepth", strconv.Itoa(depth), "-not", "-path", "*/.git/*", "(", "-type", "f", "-o", "-type", "d", ")"}
}

// searchCodeCmd returns the platform-specific command to search for pattern under path.
func searchCodeCmd(pattern, path string) (cmd string, args []string) {
	if path == "" {
		path = "."
	}
	if runtime.GOOS == "windows" {
		ps := fmt.Sprintf(
			`$root = (Resolve-Path -LiteralPath '%s').Path; $pat = '%s'; $ext = @('.go','.py','.js','.ts','.tsx','.jsx','.java','.rs','.md','.yaml','.yml','.json'); Get-ChildItem -LiteralPath $root -Recurse -File -ErrorAction SilentlyContinue | Where-Object { $ext -contains $_.Extension.ToLower() -and $_.FullName -notmatch '\\.git\\' } | Select-String -Pattern $pat | ForEach-Object { $rel = $_.Path.Substring($root.Length).TrimStart('\'); "${rel}:$($_.LineNumber):$($_.Line)" }`,
			escapePSSingle(path), escapePSSingle(pattern),
		)
		return "powershell", []string{"-NoProfile", "-NonInteractive", "-Command", ps}
	}
	return "grep", []string{
		"-rn",
		"--include=*.go", "--include=*.py", "--include=*.js", "--include=*.ts",
		"--include=*.tsx", "--include=*.jsx", "--include=*.java", "--include=*.rs",
		"--include=*.md", "--include=*.yaml", "--include=*.yml", "--include=*.json",
		pattern, path,
	}
}

// rgCmd returns ripgrep args for pattern under path with optional glob filter.
// Caller prepends command name "rg". Exit 1 means no matches (not an error).
func rgCmd(pattern, path, glob string) []string {
	if path == "" {
		path = "."
	}
	args := []string{
		"-n", "--no-heading", "-S",
		"--hidden", "--glob", "!.git",
	}
	if glob != "" {
		args = append(args, "--glob", glob)
	}
	args = append(args, "--", pattern, path)
	return args
}

// removeFileCmd returns platform-specific delete for a workspace-relative file.
func removeFileCmd(path string) (cmd string, args []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/C", "del", "/F", "/Q", path}
	}
	return "rm", []string{"-f", path}
}

// escapePSSingle escapes a string for use inside PowerShell single-quoted literals.
func escapePSSingle(s string) string {
	// In PowerShell single quotes, '' is the escape for '
	out := make([]byte, 0, len(s)+8)
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			out = append(out, '\'', '\'')
		} else {
			out = append(out, s[i])
		}
	}
	return string(out)
}
