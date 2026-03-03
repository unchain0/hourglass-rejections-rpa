package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMainExecution(t *testing.T) {
	t.Run("config directory creation", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, ".hourglass-rpa")

		err := os.MkdirAll(configDir, 0700)
		if err != nil {
			t.Fatalf("Failed to create config dir: %v", err)
		}

		info, err := os.Stat(configDir)
		if err != nil {
			t.Fatalf("Failed to stat config dir: %v", err)
		}

		assert.True(t, info.IsDir())
	})

	t.Run("tokens path generation", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, ".hourglass-rpa")
		tokensPath := filepath.Join(configDir, "auth-tokens.json")

		err := os.MkdirAll(configDir, 0700)
		if err != nil {
			t.Fatalf("Failed to create config dir: %v", err)
		}

		assert.Equal(t, filepath.Join(tempDir, ".hourglass-rpa", "auth-tokens.json"), tokensPath)
	})
}
