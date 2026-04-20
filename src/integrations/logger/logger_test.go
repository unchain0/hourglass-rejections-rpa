package logger

import (
	"testing"
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
	logger := New(ForTerminal())
	if logger == nil {
		t.Fatal("expected logger to be created")
	}
}
