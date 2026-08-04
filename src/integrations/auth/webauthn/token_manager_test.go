package webauthn

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func restoreTokenManagerHooks(t *testing.T) {
	t.Helper()

	originalAuthenticatorAuthenticate := authenticatorAuthenticate
	originalAuthenticatorSetCookies := authenticatorSetCookies
	originalBrowserAuthenticate := browserAuthenticate

	t.Cleanup(func() {
		authenticatorAuthenticate = originalAuthenticatorAuthenticate
		authenticatorSetCookies = originalAuthenticatorSetCookies
		browserAuthenticate = originalBrowserAuthenticate
	})
}

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

	t.Run("normalizes api base url", func(t *testing.T) {
		tm, err := NewTokenManager(storagePath, "https://example.com/api/v0.2")
		assert.NoError(t, err)
		assert.Equal(t, "https://example.com", tm.baseURL)
	})

	t.Run("keeps browser auth in headless mode when profile dir configured", func(t *testing.T) {
		t.Setenv("DISPLAY", "")
		t.Setenv("SSH_CONNECTION", "1")
		profileDir := filepath.Join(t.TempDir(), "chrome-profile")

		tm, err := NewTokenManager(storagePath, "https://example.com", WithBrowserProfileDir(profileDir))
		require.NoError(t, err)
		require.NotNil(t, tm.browserAuth)
		assert.Equal(t, profileDir, tm.browserAuth.profileDir)
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

func TestWithBrowserProfileDir_EmptyPathDoesNothing(t *testing.T) {
	tm := &TokenManager{baseURL: "https://example.com"}
	WithBrowserProfileDir("")(tm)
	assert.Empty(t, tm.browserProfileDir)
	assert.Nil(t, tm.browserAuth)
}

func TestWithBrowserProfileDir_UpdatesExistingBrowserAuth(t *testing.T) {
	tm := &TokenManager{baseURL: "https://example.com", browserAuth: NewBrowserAuth("https://example.com")}
	WithBrowserProfileDir("/tmp/profile")(tm)
	assert.Equal(t, "/tmp/profile", tm.browserProfileDir)
	assert.Equal(t, "/tmp/profile", tm.browserAuth.profileDir)
}

func TestNewTokenManager_UsesEnvCredentialAndProfilePaths(t *testing.T) {
	credentialsPath := filepath.Join(t.TempDir(), "env-credentials.json")
	tokensPath := filepath.Join(t.TempDir(), "env-tokens.json")
	profileDir := filepath.Join(t.TempDir(), "env-profile")
	t.Setenv("WEBAUTHN_CREDENTIALS_PATH", credentialsPath)
	t.Setenv("WEBAUTHN_TOKENS_PATH", tokensPath)
	t.Setenv("CHROME_PROFILE_DIR", profileDir)
	tm, err := NewTokenManager("", "https://example.com")
	require.NoError(t, err)
	assert.Equal(t, credentialsPath, tm.storagePath)
	assert.Equal(t, tokensPath, tm.tokensPath)
	assert.Equal(t, profileDir, tm.browserProfileDir)
	assert.NotNil(t, tm.browserAuth)
}

func TestAuthenticateWithFallback_UsesExplicitTokens(t *testing.T) {
	restoreTokenManagerHooks(t)
	storagePath := filepath.Join(t.TempDir(), "credentials.json")
	storage, err := NewStorage(storagePath)
	require.NoError(t, err)
	require.NoError(t, storage.Save(&StoredCredentials{Version: 1, Credentials: []Credential{{ID: "cred-1"}}}))
	tm := &TokenManager{authenticator: &Authenticator{storage: storage}, storagePath: storagePath}
	called := false
	authenticatorSetCookies = func(a *Authenticator, xsrfToken, hgLogin string) {
		called = true
		assert.Equal(t, "explicit-xsrf", xsrfToken)
		assert.Equal(t, "explicit-hg", hgLogin)
	}
	authenticatorAuthenticate = func(a *Authenticator) (*AuthTokens, error) {
		return &AuthTokens{HGLogin: "renewed", XSRFToken: "renewed", ExpiresAt: time.Now().Add(time.Hour)}, nil
	}
	tm.authenticator = &Authenticator{}

	tokens, err := tm.authenticateWithFallback(&AuthTokens{HGLogin: "explicit-hg", XSRFToken: "explicit-xsrf", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)
	assert.True(t, called)
	assert.NotNil(t, tokens)
}

func TestAuthenticateWithCurrentTokens_PreferWebAuthnWithoutBrowserFallback(t *testing.T) {
	restoreTokenManagerHooks(t)
	t.Setenv("DISPLAY", "")
	t.Setenv("SSH_CONNECTION", "1")
	storagePath := filepath.Join(t.TempDir(), "credentials.json")
	storage, err := NewStorage(storagePath)
	require.NoError(t, err)
	require.NoError(t, storage.Save(&StoredCredentials{Version: 1, Credentials: []Credential{{ID: "cred-1"}}}))
	tm := &TokenManager{authenticator: &Authenticator{storage: storage}, storagePath: storagePath}
	authenticatorAuthenticate = func(a *Authenticator) (*AuthTokens, error) { return nil, errors.New("webauthn failed") }

	tokens, err := tm.authenticateWithCurrentTokens(&AuthTokens{HGLogin: "h", XSRFToken: "x", ExpiresAt: time.Now().Add(time.Hour)})
	assert.Nil(t, tokens)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "webauthn failed")
}

func TestAuthenticateWithCurrentTokens_BrowserSuccessUsesCurrentTokens(t *testing.T) {
	restoreTokenManagerHooks(t)
	called := false
	authenticatorSetCookies = func(a *Authenticator, xsrfToken, hgLogin string) {
		called = true
		assert.Equal(t, "current-xsrf", xsrfToken)
		assert.Equal(t, "current-hg", hgLogin)
	}
	browserAuthenticate = func(b *BrowserAuth) (*AuthTokens, error) {
		return &AuthTokens{HGLogin: "browser", XSRFToken: "browser", ExpiresAt: time.Now().Add(time.Hour)}, nil
	}
	tm := &TokenManager{authenticator: &Authenticator{}, browserAuth: NewBrowserAuth("https://example.com")}
	tokens, err := tm.authenticateWithCurrentTokens(&AuthTokens{HGLogin: "current-hg", XSRFToken: "current-xsrf", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, "browser", tokens.HGLogin)
}

func TestAuthenticateWithCurrentTokens_EmptyHeadlessProfileFailsWithoutBrowserWait(t *testing.T) {
	restoreTokenManagerHooks(t)
	t.Setenv("DISPLAY", "")
	t.Setenv("SSH_CONNECTION", "1")

	profileDir := filepath.Join(t.TempDir(), "chrome-profile")
	require.NoError(t, os.MkdirAll(profileDir, 0o700))

	browserCalled := false
	browserAuthenticate = func(*BrowserAuth) (*AuthTokens, error) {
		browserCalled = true
		return &AuthTokens{HGLogin: "unexpected", XSRFToken: "unexpected"}, nil
	}

	tm := &TokenManager{
		authenticator:     &Authenticator{},
		browserAuth:       NewBrowserAuth("https://example.com").WithProfileDir(profileDir),
		browserProfileDir: profileDir,
		storagePath:       filepath.Join(t.TempDir(), "missing-credentials.json"),
	}

	tokens, err := tm.authenticateWithCurrentTokens(nil)
	require.Error(t, err)
	assert.Nil(t, tokens)
	assert.False(t, browserCalled)
	assert.Contains(t, err.Error(), "persistent browser profile has no cookie store")
}

func TestBrowserProfileHasCookieStore(t *testing.T) {
	profileDir := t.TempDir()
	assert.False(t, browserProfileHasCookieStore(profileDir))

	cookieDir := filepath.Join(profileDir, "Default", "Network")
	require.NoError(t, os.MkdirAll(cookieDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(cookieDir, "Cookies"), []byte("sqlite"), 0o600))
	assert.True(t, browserProfileHasCookieStore(profileDir))
}

func TestAuthenticateWithCurrentTokens_BrowserFailureFallsBackToWebAuthn(t *testing.T) {
	restoreTokenManagerHooks(t)
	storagePath := filepath.Join(t.TempDir(), "credentials.json")
	storage, err := NewStorage(storagePath)
	require.NoError(t, err)
	require.NoError(t, storage.Save(&StoredCredentials{Version: 1, Credentials: []Credential{{ID: "cred-1"}}}))
	browserAuthenticate = func(b *BrowserAuth) (*AuthTokens, error) { return nil, errors.New("browser failed") }
	authenticatorAuthenticate = func(a *Authenticator) (*AuthTokens, error) {
		return &AuthTokens{HGLogin: "native", XSRFToken: "native", ExpiresAt: time.Now().Add(time.Hour)}, nil
	}
	tm := &TokenManager{authenticator: &Authenticator{storage: storage}, storagePath: storagePath, browserAuth: NewBrowserAuth("https://example.com")}
	tokens, err := tm.authenticateWithCurrentTokens(&AuthTokens{HGLogin: "current-hg", XSRFToken: "current-xsrf", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)
	assert.Equal(t, "native", tokens.HGLogin)
}

func TestAuthenticateWithCurrentTokens_PreferWebAuthnFallsBackToBrowser(t *testing.T) {
	restoreTokenManagerHooks(t)
	t.Setenv("DISPLAY", "")
	t.Setenv("SSH_CONNECTION", "1")
	storagePath := filepath.Join(t.TempDir(), "credentials.json")
	storage, err := NewStorage(storagePath)
	require.NoError(t, err)
	require.NoError(t, storage.Save(&StoredCredentials{Version: 1, Credentials: []Credential{{ID: "cred-1"}}}))
	authenticatorAuthenticate = func(a *Authenticator) (*AuthTokens, error) { return nil, errors.New("webauthn failed") }
	browserAuthenticate = func(b *BrowserAuth) (*AuthTokens, error) {
		return &AuthTokens{HGLogin: "browser", XSRFToken: "browser", ExpiresAt: time.Now().Add(time.Hour)}, nil
	}
	tm := &TokenManager{authenticator: &Authenticator{storage: storage}, storagePath: storagePath, browserAuth: NewBrowserAuth("https://example.com")}
	tokens, err := tm.authenticateWithCurrentTokens(&AuthTokens{HGLogin: "current-hg", XSRFToken: "current-xsrf", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)
	assert.Equal(t, "browser", tokens.HGLogin)
}

func TestAuthenticateWithCurrentTokens_PrefersWebAuthnWhenDisplayIsAvailable(t *testing.T) {
	restoreTokenManagerHooks(t)
	t.Setenv("DISPLAY", ":1")
	storagePath := filepath.Join(t.TempDir(), "credentials.json")
	storage, err := NewStorage(storagePath)
	require.NoError(t, err)
	require.NoError(t, storage.Save(&StoredCredentials{Version: 1, Credentials: []Credential{{ID: "cred-1"}}}))

	browserAuthenticate = func(*BrowserAuth) (*AuthTokens, error) {
		t.Fatal("browser authentication should not run")
		return nil, nil
	}
	authenticatorAuthenticate = func(*Authenticator) (*AuthTokens, error) {
		return &AuthTokens{HGLogin: "renewed", XSRFToken: "renewed", ExpiresAt: time.Now().Add(time.Hour)}, nil
	}
	tm := &TokenManager{
		authenticator: &Authenticator{storage: storage},
		storagePath:   storagePath,
		browserAuth:   NewBrowserAuth("https://example.com"),
	}

	tokens, err := tm.authenticateWithCurrentTokens(&AuthTokens{HGLogin: "current-hg", XSRFToken: "current-xsrf", ExpiresAt: time.Now()})
	require.NoError(t, err)
	assert.Equal(t, "renewed", tokens.HGLogin)
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

func TestTokenManagerStopIsIdempotent(t *testing.T) {
	tm := &TokenManager{stopChan: make(chan struct{})}
	assert.NotPanics(t, func() {
		tm.Stop()
		tm.Stop()
	})
}

func TestTokenManagerForceRenewal_PrefersWebAuthnInHeadlessMode(t *testing.T) {
	restoreTokenManagerHooks(t)
	t.Setenv("DISPLAY", "")
	t.Setenv("SSH_CONNECTION", "1")

	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "credentials.json")
	profileDir := filepath.Join(tempDir, "chrome-profile")

	storage, err := NewStorage(storagePath)
	require.NoError(t, err)
	require.NoError(t, storage.Save(&StoredCredentials{
		Version: 1,
		Credentials: []Credential{{
			ID: "cred-1",
		}},
	}))

	tm, err := NewTokenManager(storagePath, "https://example.com", WithBrowserProfileDir(profileDir))
	require.NoError(t, err)
	tm.PrimeTokens(&AuthTokens{
		HGLogin:   "stale-hg-login",
		XSRFToken: "stale-xsrf",
		ExpiresAt: time.Now().Add(-time.Hour),
	})

	browserCalls := 0
	browserAuthenticate = func(b *BrowserAuth) (*AuthTokens, error) {
		browserCalls++
		return &AuthTokens{HGLogin: "browser", XSRFToken: "browser", ExpiresAt: time.Now().Add(time.Hour)}, nil
	}

	var gotHGLogin string
	var gotXSRF string
	authenticatorSetCookies = func(a *Authenticator, xsrfToken, hgLogin string) {
		gotXSRF = xsrfToken
		gotHGLogin = hgLogin
	}
	authenticatorAuthenticate = func(a *Authenticator) (*AuthTokens, error) {
		return &AuthTokens{HGLogin: "renewed-hg-login", XSRFToken: "renewed-xsrf", ExpiresAt: time.Now().Add(2 * time.Hour)}, nil
	}

	tokens, err := tm.ForceRenewal()
	require.NoError(t, err)
	assert.Equal(t, "stale-hg-login", gotHGLogin)
	assert.Equal(t, "stale-xsrf", gotXSRF)
	assert.Equal(t, 0, browserCalls)
	assert.Equal(t, "renewed-hg-login", tokens.HGLogin)
	assert.Equal(t, "renewed-xsrf", tokens.XSRFToken)
}
