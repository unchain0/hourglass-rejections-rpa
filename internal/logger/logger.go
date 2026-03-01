package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Config holds logger configuration.
type Config struct {
	Level      string
	Format     string // "json" or "text"
	Output     string // "stdout", "stderr", or file path
	MaxSize    int    // megabytes
	MaxBackups int
	MaxAge     int // days
	Compress   bool
}

// New creates a new slog.Logger with the given configuration.
func New(cfg Config) *slog.Logger {
	var handler slog.Handler
	var output io.Writer

	// Determine output
	switch cfg.Output {
	case "stdout":
		output = os.Stdout
	case "stderr":
		output = os.Stderr
	case "":
		output = os.Stdout
	default:
		// File output with rotation
		dir := filepath.Dir(cfg.Output)
		if dir != "" && dir != "." {
			os.MkdirAll(dir, 0755)
		}
		output = &lumberjack.Logger{
			Filename:   cfg.Output,
			MaxSize:    cfg.MaxSize,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge,
			Compress:   cfg.Compress,
		}
	}

	// Parse level
	level := parseLevel(cfg.Level)

	// Create handler
	opts := &slog.HandlerOptions{
		Level: level,
	}

	if cfg.Format == "text" {
		handler = slog.NewTextHandler(output, opts)
	} else {
		handler = slog.NewJSONHandler(output, opts)
	}

	return slog.New(handler)
}

// parseLevel parses a level string into slog.Level.
func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// DefaultConfig returns a default logger configuration.
func DefaultConfig() Config {
	return Config{
		Level:      "info",
		Format:     "json",
		Output:     "stdout",
		MaxSize:    10,
		MaxBackups: 3,
		MaxAge:     7,
		Compress:   true,
	}
}
