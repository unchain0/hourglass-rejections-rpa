package logger

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/log"
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

func TestNew_OutputVariations(t *testing.T) {
	tests := []struct {
		name   string
		output string
		cfg    Config
	}{
		{
			name:   "stdout output",
			output: "stdout",
			cfg: Config{
				Level:  "info",
				Format: "text",
				Output: "stdout",
			},
		},
		{
			name:   "stderr output",
			output: "stderr",
			cfg: Config{
				Level:  "info",
				Format: "text",
				Output: "stderr",
			},
		},
		{
			name:   "empty output defaults to stdout",
			output: "",
			cfg: Config{
				Level:  "info",
				Format: "text",
				Output: "",
			},
		},
		{
			name:   "file output",
			output: "file",
			cfg: Config{
				Level:      "info",
				Format:     "text",
				Output:     filepath.Join(t.TempDir(), "test.log"),
				MaxSize:    1,
				MaxBackups: 1,
				MaxAge:     1,
				Compress:   false,
			},
		},
		{
			name:   "file with nested directory",
			output: "nested",
			cfg: Config{
				Level:      "info",
				Format:     "text",
				Output:     filepath.Join(t.TempDir(), "logs", "app", "test.log"),
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
			// Logger should be usable
			logger.Info("test message")
		})
	}
}

func TestNew_FormatVariations(t *testing.T) {
	tests := []struct {
		name   string
		format string
		cfg    Config
	}{
		{
			name:   "json format",
			format: "json",
			cfg: Config{
				Level:  "info",
				Format: "json",
				Output: "stdout",
			},
		},
		{
			name:   "text format",
			format: "text",
			cfg: Config{
				Level:  "info",
				Format: "text",
				Output: "stdout",
			},
		},
		{
			name:   "charm format",
			format: "charm",
			cfg: Config{
				Level:  "info",
				Format: "charm",
				Output: "stdout",
			},
		},
		{
			name:   "pretty format",
			format: "pretty",
			cfg: Config{
				Level:  "info",
				Format: "pretty",
				Output: "stdout",
			},
		},
		{
			name:   "empty format defaults to json",
			format: "",
			cfg: Config{
				Level:  "info",
				Format: "",
				Output: "stdout",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := New(tt.cfg)
			if logger == nil {
				t.Error("New() returned nil")
			}
			// Logger should be usable
			logger.Info("test message")
		})
	}
}

func TestNew_LevelVariations(t *testing.T) {
	tests := []struct {
		name  string
		level string
		cfg   Config
	}{
		{
			name:  "debug level",
			level: "debug",
			cfg: Config{
				Level:  "debug",
				Format: "text",
				Output: "stdout",
			},
		},
		{
			name:  "info level",
			level: "info",
			cfg: Config{
				Level:  "info",
				Format: "text",
				Output: "stdout",
			},
		},
		{
			name:  "warn level",
			level: "warn",
			cfg: Config{
				Level:  "warn",
				Format: "text",
				Output: "stdout",
			},
		},
		{
			name:  "error level",
			level: "error",
			cfg: Config{
				Level:  "error",
				Format: "text",
				Output: "stdout",
			},
		},
		{
			name:  "empty level defaults to info",
			level: "",
			cfg: Config{
				Level:  "",
				Format: "text",
				Output: "stdout",
			},
		},
		{
			name:  "unknown level defaults to info",
			level: "unknown",
			cfg: Config{
				Level:  "unknown",
				Format: "text",
				Output: "stdout",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := New(tt.cfg)
			if logger == nil {
				t.Error("New() returned nil")
			}
			// Logger should be usable
			logger.Info("test message")
		})
	}
}

func TestForTerminal(t *testing.T) {
	cfg := ForTerminal()

	if cfg.Level != "info" {
		t.Errorf("expected Level 'info', got %q", cfg.Level)
	}

	if cfg.Format != "charm" {
		t.Errorf("expected Format 'charm', got %q", cfg.Format)
	}

	if cfg.Output != "stdout" {
		t.Errorf("expected Output 'stdout', got %q", cfg.Output)
	}

	if cfg.NoColor {
		t.Errorf("expected NoColor false, got %v", cfg.NoColor)
	}

	// Verify it can create a working logger
	logger := New(cfg)
	if logger == nil {
		t.Error("New(ForTerminal()) returned nil")
	}
}

func TestForFile(t *testing.T) {
	testPath := filepath.Join(t.TempDir(), "logs", "app.log")

	cfg := ForFile(testPath)

	if cfg.Level != "info" {
		t.Errorf("expected Level 'info', got %q", cfg.Level)
	}

	if cfg.Format != "json" {
		t.Errorf("expected Format 'json', got %q", cfg.Format)
	}

	if cfg.Output != testPath {
		t.Errorf("expected Output %q, got %q", testPath, cfg.Output)
	}

	if cfg.MaxSize != 10 {
		t.Errorf("expected MaxSize 10, got %d", cfg.MaxSize)
	}

	if cfg.MaxBackups != 5 {
		t.Errorf("expected MaxBackups 5, got %d", cfg.MaxBackups)
	}

	if cfg.MaxAge != 30 {
		t.Errorf("expected MaxAge 30, got %d", cfg.MaxAge)
	}

	if !cfg.Compress {
		t.Errorf("expected Compress true, got %v", cfg.Compress)
	}

	if !cfg.NoColor {
		t.Errorf("expected NoColor true, got %v", cfg.NoColor)
	}

	// Verify it can create a working logger
	logger := New(cfg)
	if logger == nil {
		t.Error("New(ForFile()) returned nil")
	}
}

func TestCharmLevel(t *testing.T) {
	tests := []struct {
		name     string
		level    slog.Level
		expected string // string representation for debugging
	}{
		{
			name:     "debug level",
			level:    slog.LevelDebug,
			expected: "debug",
		},
		{
			name:     "info level",
			level:    slog.LevelInfo,
			expected: "info",
		},
		{
			name:     "warn level",
			level:    slog.LevelWarn,
			expected: "warn",
		},
		{
			name:     "error level",
			level:    slog.LevelError,
			expected: "error",
		},
		{
			name:     "unknown level defaults to info",
			level:    slog.Level(999),
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := charmLevel(tt.level)
			// Verify the function returns a valid log.Level
			if got.String() == "" {
				t.Errorf("charmLevel(%v) returned invalid level", tt.level)
			}
			// Verify it can be used in a logger
			logger := log.New(os.Stdout)
			logger.SetLevel(got)
			if logger == nil {
				t.Error("Failed to set charm level")
			}
		})
	}
}

func TestDefaultConfig_Complete(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Level != "info" {
		t.Errorf("expected Level 'info', got %q", cfg.Level)
	}

	if cfg.Format != "charm" {
		t.Errorf("expected Format 'charm', got %q", cfg.Format)
	}

	if cfg.Output != "stdout" {
		t.Errorf("expected Output 'stdout', got %q", cfg.Output)
	}

	if cfg.MaxSize != 10 {
		t.Errorf("expected MaxSize 10, got %d", cfg.MaxSize)
	}

	if cfg.MaxBackups != 3 {
		t.Errorf("expected MaxBackups 3, got %d", cfg.MaxBackups)
	}

	if cfg.MaxAge != 7 {
		t.Errorf("expected MaxAge 7, got %d", cfg.MaxAge)
	}

	if !cfg.Compress {
		t.Errorf("expected Compress true, got %v", cfg.Compress)
	}

	if cfg.NoColor {
		t.Errorf("expected NoColor false, got %v", cfg.NoColor)
	}

	// Verify it can create a working logger
	logger := New(cfg)
	if logger == nil {
		t.Error("New(DefaultConfig()) returned nil")
	}
}

func TestParseLevel_Complete(t *testing.T) {
	tests := []struct {
		level     string
		wantLevel slog.Level
		wantStr   string // string representation
	}{
		{"debug", slog.LevelDebug, "DEBUG"},
		{"info", slog.LevelInfo, "INFO"},
		{"warn", slog.LevelWarn, "WARN"},
		{"error", slog.LevelError, "ERROR"},
		{"", slog.LevelInfo, "INFO"},
		{"unknown", slog.LevelInfo, "INFO"},
		{"DEBUG", slog.LevelInfo, "INFO"}, // case sensitive
		{"Info", slog.LevelInfo, "INFO"},  // case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			got := parseLevel(tt.level)
			if got != tt.wantLevel {
				t.Errorf("parseLevel(%q) = %v (%s), want %v (%s)", tt.level, got, got.String(), tt.wantLevel, tt.wantStr)
			}
		})
	}
}
