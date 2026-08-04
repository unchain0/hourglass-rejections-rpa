package telemetry

import (
	"errors"
	"testing"
	"time"
)

func TestNew_Disabled(t *testing.T) {
	client, err := New(Config{Level: "info"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if client == nil {
		t.Fatal("New() returned nil")
	}
	if client.IsEnabled() {
		t.Fatal("expected telemetry client to be disabled")
	}
	if client.Logger() == nil {
		t.Fatal("expected fallback logger")
	}
}

func TestClient_DisabledMethods(t *testing.T) {
	client, err := New(Config{Level: "debug"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	client.CaptureError(errors.New("test error"), map[string]any{"key": "value"})
	client.CaptureMessage("test message", "info")
	client.Flush(time.Millisecond)
	client.Close()
}

func TestParseHeaders(t *testing.T) {
	headers := parseHeaders("Authorization=Bearer token, stream-name=default, invalid")
	if headers["Authorization"] != "Bearer token" {
		t.Fatalf("unexpected Authorization header: %q", headers["Authorization"])
	}
	if headers["stream-name"] != "default" {
		t.Fatalf("unexpected stream-name header: %q", headers["stream-name"])
	}
	if _, ok := headers["invalid"]; ok {
		t.Fatal("invalid header entry should be ignored")
	}
}

func TestParseLevel(t *testing.T) {
	tests := map[string]int{
		"debug":  -4,
		"info":   0,
		"warn":   4,
		"error":  8,
		"random": 0,
	}

	for input, want := range tests {
		if got := int(parseLevel(input)); got != want {
			t.Fatalf("parseLevel(%q) = %d, want %d", input, got, want)
		}
	}
}
