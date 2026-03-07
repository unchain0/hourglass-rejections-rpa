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
	cfg := Config{
		DSN:         "invalid-dsn",
		Environment: "test",
		Release:     "1.0.0",
	}

	client, err := New(cfg)
	if err == nil {
		t.Error("expected error for invalid DSN")
	}
	if client != nil {
		t.Error("expected nil client for invalid DSN")
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

func TestNew_Enabled(t *testing.T) {
	// Use a test DSN that initializes but doesn't send real events
	cfg := Config{
		DSN:         "https://test@sentry.io/123",
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

	if !client.IsEnabled() {
		t.Error("expected client to be enabled when DSN is valid")
	}
	// Clean up
	client.Close()
}

func TestNew_IsEnabled(t *testing.T) {
	// Test disabled client
	disabledClient, err := New(Config{DSN: ""})
	if err != nil {
		t.Errorf("New() error = %v", err)
	}
	if disabledClient.IsEnabled() {
		t.Error("expected disabled client to return false for IsEnabled()")
	}

	// Test enabled client
	enabledClient, err := New(Config{DSN: "https://test@sentry.io/123"})
	if err != nil {
		t.Errorf("New() error = %v", err)
	}
	if !enabledClient.IsEnabled() {
		t.Error("expected enabled client to return true for IsEnabled()")
	}
	// Clean up
	enabledClient.Close()
}

func TestClient_CaptureError_Enabled(t *testing.T) {
	client, err := New(Config{DSN: "https://test@sentry.io/123"})
	if err != nil {
		t.Errorf("New() error = %v", err)
	}

	// Test with nil error - should not panic
	client.CaptureError(nil, nil)

	// Test with error and no extras
	testErr := errors.New("test error")
	client.CaptureError(testErr, nil)

	// Test with error and extras
	client.CaptureError(testErr, map[string]interface{}{
		"key1": "value1",
		"key2": 123,
		"key3": true,
	})

	// Clean up
	client.Close()
}

func TestClient_CaptureMessage_Enabled(t *testing.T) {
	client, err := New(Config{DSN: "https://test@sentry.io/123"})
	if err != nil {
		t.Errorf("New() error = %v", err)
	}

	// Test with different levels
	levels := []string{"debug", "info", "warn", "warning", "error", "fatal", "unknown"}
	for _, level := range levels {
		// Should not panic with any level
		client.CaptureMessage("test message", level)
	}

	// Clean up
	client.Close()
}

func TestClient_Flush_Enabled(t *testing.T) {
	client, err := New(Config{DSN: "https://test@sentry.io/123"})
	if err != nil {
		t.Errorf("New() error = %v", err)
	}

	// Should not panic when enabled
	client.Flush(1 * time.Second)

	// Clean up
	client.Close()
}

func TestClient_Close_Enabled(t *testing.T) {
	client, err := New(Config{DSN: "https://test@sentry.io/123"})
	if err != nil {
		t.Errorf("New() error = %v", err)
	}

	// Should not panic when enabled
	client.Close()
}

func TestParseLevel_EmptyString(t *testing.T) {
	// Empty string should default to Info level
	got := parseLevel("")
	if string(got) != "info" {
		t.Errorf("parseLevel(\"\") = %v, want info", got)
	}
}
