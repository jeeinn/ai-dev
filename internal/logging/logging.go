package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Level represents log verbosity.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

var currentLevel = LevelInfo

// SetLevel configures the minimum log level from config (debug/info/warn/error).
func SetLevel(level string) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		currentLevel = LevelDebug
	case "warn", "warning":
		currentLevel = LevelWarn
	case "error":
		currentLevel = LevelError
	default:
		currentLevel = LevelInfo
	}
}

// LevelEnabled reports whether the given level would be emitted.
func LevelEnabled(level Level) bool {
	return level >= currentLevel
}

func Debugf(format string, args ...any) {
	if currentLevel <= LevelDebug {
		log.Printf("[DEBUG] "+format, args...)
	}
}

func Infof(format string, args ...any) {
	if currentLevel <= LevelInfo {
		log.Printf("[INFO] "+format, args...)
	}
}

func Warnf(format string, args ...any) {
	if currentLevel <= LevelWarn {
		log.Printf("[WARN] "+format, args...)
	}
}

func Errorf(format string, args ...any) {
	if currentLevel <= LevelError {
		log.Printf("[ERROR] "+format, args...)
	}
}

// SetupOutput configures log output to stdout and optionally appends to dir/matea.log.
// Returns a cleanup function that should be deferred on shutdown.
func SetupOutput(dir string) (func(), error) {
	if strings.TrimSpace(dir) == "" {
		return func() {}, nil
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	logPath := filepath.Join(dir, "matea.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file %s: %w", logPath, err)
	}

	log.SetOutput(io.MultiWriter(os.Stdout, f))
	log.Printf("[INFO] Logging to file: %s", logPath)

	return func() {
		_ = f.Close()
	}, nil
}
