package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/charmbracelet/log"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Config holds logger configuration.
type Config struct {
	Level      string
	Format     string // "json", "text", "pretty", or "charm"
	Output     string // "stdout", "stderr", or file path
	MaxSize    int    // megabytes
	MaxBackups int
	MaxAge     int // days
	Compress   bool
	NoColor    bool // Disable colors
}

// New creates a new logger with the given configuration.
// Supports multiple formats:
//   - "charm" or "pretty": Beautiful colored output using charmbracelet/log
//   - "text": Standard text format
//   - "json": JSON format for structured logging
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

	// Create logger based on format
	switch cfg.Format {
	case "charm", "pretty":
		// Use charmbracelet/log for beautiful terminal output
		logger := log.New(output)
		logger.SetLevel(charmLevel(level))
		if cfg.NoColor || os.Getenv("NO_COLOR") != "" {
			logger.SetStyles(log.DefaultStylesWithoutColor())
		}
		return slog.New(logger)
	case "text":
		return slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{Level: level}))
	default:
		return slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{Level: level}))
	}
}

// charmLevel converts slog.Level to charmbracelet/log Level.
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
		Format:     "charm", // Use charmbracelet/log by default
		Output:     "stdout",
		MaxSize:    10,
		MaxBackups: 3,
		MaxAge:     7,
		Compress:   true,
		NoColor:    false,
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

// ForFile returns a config optimized for file output.
func ForFile(path string) Config {
	return Config{
		Level:      "info",
		Format:     "json",
		Output:     path,
		MaxSize:    10,
		MaxBackups: 5,
		MaxAge:     30,
		Compress:   true,
		NoColor:    true,
	}
}
