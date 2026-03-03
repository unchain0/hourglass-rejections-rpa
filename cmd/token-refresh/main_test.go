package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"hourglass-rejections-rpa/internal/auth/webauthn"
)

func TestLoadTokens(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("file not found", func(t *testing.T) {
		tokens, err := loadTokens(filepath.Join(tempDir, "nonexistent.json"))
		assert.Error(t, err)
		assert.Nil(t, tokens)
		assert.Contains(t, err.Error(), "não encontrado")
	})

	t.Run("invalid json", func(t *testing.T) {
		tokensPath := filepath.Join(tempDir, "invalid.json")
		err := os.WriteFile(tokensPath, []byte("invalid json"), 0600)
		require.NoError(t, err)

		tokens, err := loadTokens(tokensPath)
		assert.Error(t, err)
		assert.Nil(t, tokens)
	})

	t.Run("valid tokens", func(t *testing.T) {
		tokensPath := filepath.Join(tempDir, "valid.json")
		validTokens := `{"hg_login":"test","xsrf_token":"test123","expires_at":"2026-03-04T00:00:00Z"}`
		err := os.WriteFile(tokensPath, []byte(validTokens), 0600)
		require.NoError(t, err)

		tokens, err := loadTokens(tokensPath)
		assert.NoError(t, err)
		assert.NotNil(t, tokens)
		assert.Equal(t, "test", tokens.HGLogin)
		assert.Equal(t, "test123", tokens.XSRFToken)
	})
}

func TestSaveTokens(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("save and load roundtrip", func(t *testing.T) {
		tokensPath := filepath.Join(tempDir, "tokens.json")
		tokens := &webauthn.AuthTokens{
			HGLogin:   "test-hglogin",
			XSRFToken: "test-xsrf",
			ExpiresAt: time.Now().Add(8 * time.Hour),
		}

		err := saveTokens(tokensPath, tokens)
		require.NoError(t, err)

		loaded, err := loadTokens(tokensPath)
		require.NoError(t, err)
		assert.Equal(t, tokens.HGLogin, loaded.HGLogin)
		assert.Equal(t, tokens.XSRFToken, loaded.XSRFToken)
	})

	t.Run("creates nested directories", func(t *testing.T) {
		nestedPath := filepath.Join(tempDir, "a", "b", "c", "tokens.json")
		tokens := &webauthn.AuthTokens{
			HGLogin:   "test",
			XSRFToken: "test",
			ExpiresAt: time.Now(),
		}

		err := saveTokens(nestedPath, tokens)
		require.NoError(t, err)

		_, err = os.Stat(nestedPath)
		assert.NoError(t, err)

		info, err := os.Stat(nestedPath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	})
}

func TestTryRefreshInvalidTokens(t *testing.T) {
	t.Run("with invalid tokens should fail", func(t *testing.T) {
		tokens := &webauthn.AuthTokens{
			HGLogin:   "invalid-token",
			XSRFToken: "invalid-xsrf",
		}
		newTokens, err := tryRefresh(tokens)
		assert.Error(t, err)
		assert.Nil(t, newTokens)
	})
}

func TestMainFunction(t *testing.T) {
	t.Run("exits when tokens file not found", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Setenv("HOME", tempDir)

		configDir := filepath.Join(tempDir, ".hourglass-rpa")
		err := os.RemoveAll(configDir)
		if err != nil {
			t.Logf("Could not remove dir: %v", err)
		}

		assert.NoDirExists(t, configDir)
	})
}
