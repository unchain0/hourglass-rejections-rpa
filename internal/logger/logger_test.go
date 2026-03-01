package logger

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "json stdout",
			cfg: Config{
				Level:  "info",
				Format: "json",
				Output: "stdout",
			},
		},
		{
			name: "text stderr",
			cfg: Config{
				Level:  "debug",
				Format: "text",
				Output: "stderr",
			},
		},
		{
			name: "file output",
			cfg: Config{
				Level:      "info",
				Format:     "json",
				Output:     filepath.Join(t.TempDir(), "test.log"),
				MaxSize:    1,
				MaxBackups: 1,
				MaxAge:     1,
				Compress:   false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := New(tt.cfg)
			if logger == nil {
				t.Error("New() returned nil")
			}
		})
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		level string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"unknown", slog.LevelInfo},
		{"", slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			got := parseLevel(tt.level)
			if got != tt.want {
				t.Errorf("parseLevel(%q) = %v, want %v", tt.level, got, tt.want)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Level != "info" {
		t.Errorf("expected Level 'info', got %q", cfg.Level)
	}

	if cfg.Format != "charm" {
		t.Errorf("expected Format 'charm', got %q", cfg.Format)
		t.Errorf("expected Format 'json', got %q", cfg.Format)
	}

	if cfg.Output != "stdout" {
		t.Errorf("expected Output 'stdout', got %q", cfg.Output)
	}

	if cfg.MaxSize != 10 {
		t.Errorf("expected MaxSize 10, got %d", cfg.MaxSize)
	}
}

func TestNew_FileRotation(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "logs", "app.log")

	cfg := Config{
		Level:      "info",
		Format:     "json",
		Output:     logFile,
		MaxSize:    1,
		MaxBackups: 2,
		MaxAge:     1,
		Compress:   false,
	}

	logger := New(cfg)
	if logger == nil {
		t.Fatal("New() returned nil")
	}

	// Write some logs
	logger.Info("test message")

	// Check if log directory was created
	if _, err := os.Stat(filepath.Dir(logFile)); os.IsNotExist(err) {
		t.Error("log directory was not created")
	}
}
