package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"hourglass-rejections-rpa/internal/auth/webauthn"
)

func TestSetupOptions(t *testing.T) {
	t.Run("default options", func(t *testing.T) {
		opts := setupOptions{
			getenv:        os.Getenv,
			osUserHomeDir: os.UserHomeDir,
		}
		assert.NotNil(t, opts.getenv)
		assert.NotNil(t, opts.osUserHomeDir)
	})
}

func TestCheckExistingTokens(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("no existing tokens", func(t *testing.T) {
		tokensPath := filepath.Join(tempDir, "nonexistent.json")
		tokens, err := checkExistingTokens(tokensPath)
		assert.NoError(t, err)
		assert.Nil(t, tokens)
	})

	t.Run("valid tokens", func(t *testing.T) {
		tokensPath := filepath.Join(tempDir, "tokens.json")
		validTokens := &webauthn.AuthTokens{
			HGLogin:   "test",
			XSRFToken: "test123",
			ExpiresAt: time.Now().Add(8 * time.Hour),
		}
		data, _ := json.Marshal(validTokens)
		err := os.WriteFile(tokensPath, data, 0600)
		require.NoError(t, err)

		tokens, err := checkExistingTokens(tokensPath)
		assert.NoError(t, err)
		assert.NotNil(t, tokens)
		assert.False(t, tokens.IsExpired())
	})

	t.Run("expired tokens", func(t *testing.T) {
		tokensPath := filepath.Join(tempDir, "expired.json")
		expiredTokens := &webauthn.AuthTokens{
			HGLogin:   "test",
			XSRFToken: "test123",
			ExpiresAt: time.Now().Add(-1 * time.Hour),
		}
		data, _ := json.Marshal(expiredTokens)
		err := os.WriteFile(tokensPath, data, 0600)
		require.NoError(t, err)

		tokens, err := checkExistingTokens(tokensPath)
		assert.NoError(t, err)
		assert.NotNil(t, tokens)
		assert.True(t, tokens.IsExpired())
	})

	t.Run("invalid json", func(t *testing.T) {
		tokensPath := filepath.Join(tempDir, "invalid.json")
		err := os.WriteFile(tokensPath, []byte("invalid"), 0600)
		require.NoError(t, err)

		tokens, err := checkExistingTokens(tokensPath)
		assert.Error(t, err)
		assert.Nil(t, tokens)
	})
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "shorter than max",
			input:    "hello",
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "exactly max",
			input:    "hello world",
			maxLen:   11,
			expected: "hello world",
		},
		{
			name:     "longer than max",
			input:    "hello world this is a long string",
			maxLen:   10,
			expected: "hello worl...",
		},
		{
			name:     "empty string",
			input:    "",
			maxLen:   10,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncate(tt.input, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCopyTokensToVPS(t *testing.T) {
	t.Run("dry run", func(t *testing.T) {
		tempDir := t.TempDir()
		tokensPath := filepath.Join(tempDir, "tokens.json")
		tokens := &webauthn.AuthTokens{
			HGLogin:   "test",
			XSRFToken: "test123",
			ExpiresAt: time.Now().Add(8 * time.Hour),
		}
		data, _ := json.Marshal(tokens)
		err := os.WriteFile(tokensPath, data, 0600)
		require.NoError(t, err)

		assert.FileExists(t, tokensPath)
	})
}

func TestConstants(t *testing.T) {
	assert.Equal(t, "https://app.hourglass-app.com/api/v0.2", defaultBaseURL)
	assert.Equal(t, ".hourglass-rpa", defaultConfigDir)
	assert.Equal(t, "auth-tokens.json", defaultTokensFile)
}
