package sentry

import (
	"errors"
	"testing"
	"time"
)

func TestNew_Disabled(t *testing.T) {
	cfg := Config{
		DSN:         "",
		Environment: "test",
		Release:     "1.0.0",
	}

	client, err := New(cfg)
	if err != nil {
		t.Errorf("New() error = %v", err)
	}

	if client == nil {
		t.Fatal("New() returned nil")
	}

	if client.IsEnabled() {
		t.Error("expected client to be disabled when DSN is empty")
	}
}

func TestNew_InvalidDSN(t *testing.T) {
	// This would actually try to connect to Sentry
	// In real tests, we might want to mock this
	t.Skip("Skipping test that requires Sentry connection")

	cfg := Config{
		DSN:         "invalid-dsn",
		Environment: "test",
		Release:     "1.0.0",
	}

	_, err := New(cfg)
	if err == nil {
		t.Error("expected error for invalid DSN")
	}
}

func TestClient_CaptureError_Disabled(t *testing.T) {
	client := &Client{enabled: false}

	// Should not panic when disabled
	client.CaptureError(errors.New("test error"), nil)
	client.CaptureError(errors.New("test error"), map[string]interface{}{"key": "value"})
}

func TestClient_CaptureMessage_Disabled(t *testing.T) {
	client := &Client{enabled: false}

	// Should not panic when disabled
	client.CaptureMessage("test message", "info")
}

func TestClient_Flush_Disabled(t *testing.T) {
	client := &Client{enabled: false}

	// Should not panic when disabled
	client.Flush(1 * time.Second)
}

func TestClient_Close_Disabled(t *testing.T) {
	client := &Client{enabled: false}

	// Should not panic when disabled
	client.Close()
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		level string
		want  string
	}{
		{"debug", "debug"},
		{"info", "info"},
		{"warning", "warning"},
		{"warn", "warning"},
		{"error", "error"},
		{"fatal", "fatal"},
		{"unknown", "info"},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			got := parseLevel(tt.level)
			// The returned type is sentry.Level which is a string type
			// We can compare by converting to string
			if string(got) != tt.want {
				t.Errorf("parseLevel(%q) = %v, want %v", tt.level, got, tt.want)
			}
		})
	}
}
