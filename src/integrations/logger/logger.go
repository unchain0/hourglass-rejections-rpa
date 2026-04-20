// Package logger provides slog logger construction helpers.
package logger

import (
	"io"
	"log/slog"
	"os"

	"github.com/charmbracelet/log"
)

// Config holds logger configuration.
type Config struct {
	Level   string
	Format  string // "json", "text", "pretty", or "charm"
	Output  string // "stdout" or "stderr"
	NoColor bool   // Disable colors
}

// New creates a new logger with the given configuration.
func New(cfg Config) *slog.Logger {
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
		output = os.Stdout
	}

	// Parse level
	level := parseLevel(cfg.Level)

	// Create logger based on format
	switch cfg.Format {
	case "charm", "pretty":
		logger := log.New(output)
		logger.SetLevel(charmLevel(level))
		logger.SetReportTimestamp(true)
		logger.SetTimeFormat("2006-01-02 15:04:05")
		return slog.New(logger)
	case "text":
		return slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{Level: level}))
	default:
		return slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{Level: level}))
	}
}

func charmLevel(level slog.Level) log.Level {
	switch level {
	case slog.LevelDebug:
		return log.DebugLevel
	case slog.LevelInfo:
		return log.InfoLevel
	case slog.LevelWarn:
		return log.WarnLevel
	case slog.LevelError:
		return log.ErrorLevel
	default:
		return log.InfoLevel
	}
}

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
		Level:   "info",
		Format:  "charm",
		Output:  "stdout",
		NoColor: false,
	}
}

// ForTerminal returns a config optimized for terminal output.
func ForTerminal() Config {
	return Config{
		Level:   "info",
		Format:  "charm",
		Output:  "stdout",
		NoColor: false,
	}
}
