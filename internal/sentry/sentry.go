package sentry

import (
	"fmt"
	"time"

	"github.com/getsentry/sentry-go"
)

// Client wraps Sentry client functionality.
type Client struct {
	enabled bool
}

// Config holds Sentry configuration.
type Config struct {
	DSN         string
	Environment string
	Release     string
}

// New initializes Sentry with the given configuration.
func New(cfg Config) (*Client, error) {
	if cfg.DSN == "" {
		return &Client{enabled: false}, nil
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:         cfg.DSN,
		Environment: cfg.Environment,
		Release:     cfg.Release,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize sentry: %w", err)
	}

	return &Client{enabled: true}, nil
}

// CaptureError captures an error to Sentry.
func (c *Client) CaptureError(err error, extras map[string]interface{}) {
	if !c.enabled || err == nil {
		return
	}

	if extras != nil {
		scope := sentry.NewScope()
		for key, value := range extras {
			scope.SetExtra(key, value)
		}
		sentry.CaptureException(err)
	} else {
		sentry.CaptureException(err)
	}
}

// CaptureMessage captures a message to Sentry.
func (c *Client) CaptureMessage(message string, level string) {
	if !c.enabled {
		return
	}

	sentry.WithScope(func(scope *sentry.Scope) {
		scope.SetLevel(parseLevel(level))
		sentry.CaptureMessage(message)
	})
}

// Flush waits for Sentry to send pending events.
func (c *Client) Flush(timeout time.Duration) {
	if !c.enabled {
		return
	}

	sentry.Flush(timeout)
}

// Close closes the Sentry client.
func (c *Client) Close() {
	if !c.enabled {
		return
	}

	sentry.Flush(2 * time.Second)
}

// parseLevel converts a string level to Sentry level.
func parseLevel(level string) sentry.Level {
	switch level {
	case "debug":
		return sentry.LevelDebug
	case "info":
		return sentry.LevelInfo
	case "warning", "warn":
		return sentry.LevelWarning
	case "error":
		return sentry.LevelError
	case "fatal":
		return sentry.LevelFatal
	default:
		return sentry.LevelInfo
	}
}

// IsEnabled returns true if Sentry is enabled.
func (c *Client) IsEnabled() bool {
	return c.enabled
}
