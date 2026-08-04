package logger

import (
	"log/slog"
	"testing"

	charm "github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Output != "stdout" {
		t.Fatalf("expected stdout output, got %s", cfg.Output)
	}
	if cfg.Format != "charm" {
		t.Fatalf("expected charm format, got %s", cfg.Format)
	}
}

func TestForTerminal(t *testing.T) {
	cfg := ForTerminal()
	if cfg.Output != "stdout" {
		t.Fatalf("expected stdout output, got %s", cfg.Output)
	}
	if cfg.Level != "info" {
		t.Fatalf("expected info level, got %s", cfg.Level)
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{name: "charm stdout", config: Config{Level: "info", Format: "charm", Output: "stdout"}},
		{name: "pretty stderr", config: Config{Level: "debug", Format: "pretty", Output: "stderr"}},
		{name: "text default output", config: Config{Level: "warn", Format: "text"}},
		{name: "json unknown output", config: Config{Level: "error", Format: "json", Output: "unknown"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.NotNil(t, New(test.config))
		})
	}
}

func TestLevelMappings(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		slogLevel slog.Level
		charm     charm.Level
	}{
		{name: "debug", input: "debug", slogLevel: slog.LevelDebug, charm: charm.DebugLevel},
		{name: "info", input: "info", slogLevel: slog.LevelInfo, charm: charm.InfoLevel},
		{name: "warn", input: "warn", slogLevel: slog.LevelWarn, charm: charm.WarnLevel},
		{name: "error", input: "error", slogLevel: slog.LevelError, charm: charm.ErrorLevel},
		{name: "default", input: "unknown", slogLevel: slog.LevelInfo, charm: charm.InfoLevel},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			level := parseLevel(test.input)
			assert.Equal(t, test.slogLevel, level)
			assert.Equal(t, test.charm, charmLevel(level))
		})
	}

	assert.Equal(t, charm.InfoLevel, charmLevel(slog.Level(99)))
}
