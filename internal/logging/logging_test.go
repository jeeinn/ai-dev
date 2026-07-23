package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetLevel(t *testing.T) {
	tests := []struct {
		input   string
		debugOn bool
		infoOn  bool
	}{
		{"debug", true, true},
		{"info", false, true},
		{"warn", false, false},
		{"error", false, false},
	}

	for _, tt := range tests {
		SetLevel(tt.input)
		if LevelEnabled(LevelDebug) != tt.debugOn {
			t.Fatalf("level=%s debug=%v want %v", tt.input, LevelEnabled(LevelDebug), tt.debugOn)
		}
		if LevelEnabled(LevelInfo) != tt.infoOn {
			t.Fatalf("level=%s info=%v want %v", tt.input, LevelEnabled(LevelInfo), tt.infoOn)
		}
	}
}

func TestSetupOutputWritesToFile(t *testing.T) {
	dir := t.TempDir()

	cleanup, err := SetupOutput(dir)
	if err != nil {
		t.Fatalf("SetupOutput failed: %v", err)
	}
	defer cleanup()

	SetLevel("info")
	Infof("hello from test log")

	data, err := os.ReadFile(filepath.Join(dir, "matea.log"))
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(data), "hello from test log") {
		t.Fatalf("log file missing message: %s", string(data))
	}
}
