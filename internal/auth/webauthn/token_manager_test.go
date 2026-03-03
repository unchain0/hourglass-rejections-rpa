package webauthn

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenManagerSaveAndLoadTokens(t *testing.T) {
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "credentials.json")
	tokensPath := filepath.Join(tempDir, "tokens.json")

	tm, err := NewTokenManager(storagePath, "https://example.com", WithTokensPath(tokensPath))
	require.NoError(t, err)

	t.Run("save tokens", func(t *testing.T) {
		tokens := &AuthTokens{
			HGLogin:   "test-hglogin",
			XSRFToken: "test-xsrf",
			ExpiresAt: time.Now().Add(8 * time.Hour),
		}

		err := tm.SaveTokens(tokens)
		assert.NoError(t, err)

		info, err := os.Stat(tokensPath)
		assert.NoError(t, err)
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	})

	t.Run("load tokens", func(t *testing.T) {
		loaded, err := tm.LoadTokens()
		assert.NoError(t, err)
		assert.NotNil(t, loaded)
		assert.Equal(t, "test-hglogin", loaded.HGLogin)
		assert.Equal(t, "test-xsrf", loaded.XSRFToken)
	})

	t.Run("load non-existent", func(t *testing.T) {
		nonExistentPath := filepath.Join(tempDir, "non-existent", "tokens.json")
		tm2, _ := NewTokenManager(storagePath, "https://example.com", WithTokensPath(nonExistentPath))
		loaded, err := tm2.LoadTokens()
		assert.NoError(t, err)
		assert.Nil(t, loaded)
	})
}

func TestTokenManagerOptions(t *testing.T) {
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "credentials.json")

	t.Run("with browser auth", func(t *testing.T) {
		ba := NewBrowserAuth("https://example.com")
		tm, err := NewTokenManager(storagePath, "https://example.com", WithBrowserAuth(ba))
		assert.NoError(t, err)
		assert.NotNil(t, tm)
		assert.NotNil(t, tm.browserAuth)
	})

	t.Run("with renewal threshold", func(t *testing.T) {
		tm, err := NewTokenManager(storagePath, "https://example.com", WithRenewalThreshold(30*time.Minute))
		assert.NoError(t, err)
		assert.NotNil(t, tm)
		assert.Equal(t, 30*time.Minute, tm.renewalThreshold)
	})

	t.Run("with callbacks", func(t *testing.T) {
		renewedCalled := false
		errorCalled := false

		tm, err := NewTokenManager(
			storagePath,
			"https://example.com",
			WithOnTokenRenewed(func(tokens *AuthTokens) {
				renewedCalled = true
			}),
			WithOnError(func(err error) {
				errorCalled = true
			}),
		)
		assert.NoError(t, err)
		assert.NotNil(t, tm)
		assert.NotNil(t, tm.onTokenRenewed)
		assert.NotNil(t, tm.onError)

		tm.onTokenRenewed(nil)
		tm.onError(nil)
		assert.True(t, renewedCalled)
		assert.True(t, errorCalled)
	})
}

func TestTokenManagerWithTokensPath(t *testing.T) {
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "credentials.json")
	customPath := filepath.Join(tempDir, "custom", "tokens.json")

	tm, err := NewTokenManager(storagePath, "https://example.com", WithTokensPath(customPath))
	require.NoError(t, err)
	assert.Equal(t, customPath, tm.tokensPath)
}

func TestTokenManagerSaveTokensErrors(t *testing.T) {
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "credentials.json")
	tokensPath := filepath.Join(tempDir, "tokens.json")

	tm, _ := NewTokenManager(storagePath, "https://example.com", WithTokensPath(tokensPath))

	t.Run("nil tokens", func(t *testing.T) {
		err := tm.SaveTokens(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tokens cannot be nil")
	})

	t.Run("empty path", func(t *testing.T) {
		tm2, _ := NewTokenManager(storagePath, "https://example.com")
		tm2.tokensPath = ""
		tokens := &AuthTokens{}
		err := tm2.SaveTokens(tokens)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tokens path is not configured")
	})
}
