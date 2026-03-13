package main

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"hourglass-rejections-rpa/internal/auth/webauthn"
)

type mockTokenManager struct {
	loadedTokens  *webauthn.AuthTokens
	loadErr       error
	ensuredTokens *webauthn.AuthTokens
	ensureErr     error
	loadCalls     int
	ensureCalls   int
}

func (m *mockTokenManager) LoadTokens() (*webauthn.AuthTokens, error) {
	m.loadCalls++
	return m.loadedTokens, m.loadErr
}

func (m *mockTokenManager) EnsureValidTokens() (*webauthn.AuthTokens, error) {
	m.ensureCalls++
	return m.ensuredTokens, m.ensureErr
}

func TestNewTokenRefresher_Defaults(t *testing.T) {
	tr := newTokenRefresher()
	assert.NotNil(t, tr.userHomeDir)
	assert.NotNil(t, tr.getenv)
	assert.NotNil(t, tr.tokenManagerFactory)
	assert.Equal(t, defaultBaseURL, tr.baseURL)
}

func TestNewTokenRefresher_DefaultFactory(t *testing.T) {
	tr := newTokenRefresher()
	tempDir := t.TempDir()
	manager, err := tr.tokenManagerFactory(filepath.Join(tempDir, "credentials.json"), defaultBaseURL)
	require.NoError(t, err)
	require.NotNil(t, manager)
}

func TestTokenRefresher_Run_RenewsTokens(t *testing.T) {
	currentTokens := &webauthn.AuthTokens{
		HGLogin:   "old-hglogin",
		XSRFToken: "old-xsrf",
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}
	newTokens := &webauthn.AuthTokens{
		HGLogin:   "new-hglogin",
		XSRFToken: "new-xsrf",
		ExpiresAt: time.Now().Add(8 * time.Hour),
	}
	manager := &mockTokenManager{
		loadedTokens:  currentTokens,
		ensuredTokens: newTokens,
	}

	var gotCredentialsPath string
	var gotBaseURL string

	tr := &tokenRefresher{
		userHomeDir: func() (string, error) { return "/home/test", nil },
		getenv:      func(string) string { return "" },
		tokenManagerFactory: func(credentialsPath, baseURL string, _ ...webauthn.TokenManagerOption) (tokenManager, error) {
			gotCredentialsPath = credentialsPath
			gotBaseURL = baseURL
			return manager, nil
		},
		baseURL: defaultBaseURL,
	}

	err := tr.Run()
	require.NoError(t, err)
	assert.Equal(t, "/home/test/.hourglass-rpa/webauthn-credentials.json", gotCredentialsPath)
	assert.Equal(t, defaultBaseURL, gotBaseURL)
	assert.Equal(t, 1, manager.loadCalls)
	assert.Equal(t, 1, manager.ensureCalls)
}

func TestTokenRefresher_Run_AlreadyValid(t *testing.T) {
	tokens := &webauthn.AuthTokens{
		HGLogin:   "same-hglogin",
		XSRFToken: "same-xsrf",
		ExpiresAt: time.Now().Add(8 * time.Hour),
	}
	manager := &mockTokenManager{
		loadedTokens:  tokens,
		ensuredTokens: tokens,
	}

	tr := &tokenRefresher{
		userHomeDir: func() (string, error) { return "/home/test", nil },
		getenv:      func(string) string { return "" },
		tokenManagerFactory: func(string, string, ...webauthn.TokenManagerOption) (tokenManager, error) {
			return manager, nil
		},
		baseURL: defaultBaseURL,
	}

	err := tr.Run()
	require.NoError(t, err)
	assert.Equal(t, 1, manager.loadCalls)
	assert.Equal(t, 1, manager.ensureCalls)
}

func TestTokenRefresher_Run_ConfigDirError(t *testing.T) {
	tr := &tokenRefresher{
		userHomeDir: func() (string, error) { return "", errors.New("home dir error") },
		getenv:      func(string) string { return "" },
	}

	err := tr.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "erro ao obter diretório de configuração")
}

func TestTokenRefresher_Run_LoadError(t *testing.T) {
	manager := &mockTokenManager{loadErr: errors.New("load failed")}
	tr := &tokenRefresher{
		userHomeDir: func() (string, error) { return "/home/test", nil },
		getenv:      func(string) string { return "" },
		tokenManagerFactory: func(string, string, ...webauthn.TokenManagerOption) (tokenManager, error) {
			return manager, nil
		},
		baseURL: defaultBaseURL,
	}

	err := tr.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "erro ao carregar tokens atuais")
}

func TestTokenRefresher_Run_FactoryError(t *testing.T) {
	tr := &tokenRefresher{
		userHomeDir: func() (string, error) { return "/home/test", nil },
		getenv:      func(string) string { return "" },
		tokenManagerFactory: func(string, string, ...webauthn.TokenManagerOption) (tokenManager, error) {
			return nil, errors.New("factory failed")
		},
		baseURL: defaultBaseURL,
	}

	err := tr.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "erro ao criar gerenciador de tokens")
}

func TestTokenRefresher_Run_EnsureError(t *testing.T) {
	manager := &mockTokenManager{ensureErr: errors.New("renew failed")}
	tr := &tokenRefresher{
		userHomeDir: func() (string, error) { return "/home/test", nil },
		getenv:      func(string) string { return "" },
		tokenManagerFactory: func(string, string, ...webauthn.TokenManagerOption) (tokenManager, error) {
			return manager, nil
		},
		baseURL: defaultBaseURL,
	}

	err := tr.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "falha na renovação real dos tokens")
}

func TestTokenRefresher_tokensPathPriority(t *testing.T) {
	tr := &tokenRefresher{
		getenv: func(key string) string {
			switch key {
			case "WEBAUTHN_TOKENS_PATH":
				return "/tmp/webauthn-tokens.json"
			case "TOKENS_PATH":
				return "/tmp/tokens.json"
			default:
				return ""
			}
		},
	}

	assert.Equal(t, "/tmp/webauthn-tokens.json", tr.tokensPath("/home/test/.hourglass-rpa"))
}

func TestTokenRefresher_tokensPathFallbackToTokensEnv(t *testing.T) {
	tr := &tokenRefresher{
		getenv: func(key string) string {
			if key == "TOKENS_PATH" {
				return "/tmp/tokens.json"
			}
			return ""
		},
	}

	assert.Equal(t, "/tmp/tokens.json", tr.tokensPath("/home/test/.hourglass-rpa"))
}

func TestTokenRefresher_credentialsPathPriority(t *testing.T) {
	tr := &tokenRefresher{
		getenv: func(key string) string {
			if key == "WEBAUTHN_CREDENTIALS_PATH" {
				return "/tmp/webauthn-credentials.json"
			}
			return ""
		},
	}

	assert.Equal(t, "/tmp/webauthn-credentials.json", tr.credentialsPath("/home/test/.hourglass-rpa"))
}

func TestTokenRefresher_renewalThreshold(t *testing.T) {
	t.Run("uses env interval", func(t *testing.T) {
		tr := &tokenRefresher{
			getenv: func(key string) string {
				if key == "REFRESH_INTERVAL" {
					return "4h"
				}
				return ""
			},
		}

		assert.Equal(t, 4*time.Hour, tr.renewalThreshold())
	})

	t.Run("falls back on invalid interval", func(t *testing.T) {
		tr := &tokenRefresher{
			getenv: func(key string) string {
				if key == "REFRESH_INTERVAL" {
					return "invalid"
				}
				return ""
			},
		}

		assert.Equal(t, defaultRefreshThreshold, tr.renewalThreshold())
	})
}

func TestTokensEqual(t *testing.T) {
	now := time.Now().UTC()
	left := &webauthn.AuthTokens{HGLogin: "h", XSRFToken: "x", ExpiresAt: now}
	right := &webauthn.AuthTokens{HGLogin: "h", XSRFToken: "x", ExpiresAt: now}
	assert.True(t, tokensEqual(left, right))
	assert.False(t, tokensEqual(left, &webauthn.AuthTokens{HGLogin: "other", XSRFToken: "x", ExpiresAt: now}))
	assert.True(t, tokensEqual(nil, nil))
	assert.False(t, tokensEqual(left, nil))
}

func TestMain_ErrorExits(t *testing.T) {
	origExit := osExit
	origNewTokenRefresher := newTokenRefresherFunc
	defer func() { osExit = origExit }()
	defer func() { newTokenRefresherFunc = origNewTokenRefresher }()

	exitCode := 0
	osExit = func(code int) {
		exitCode = code
		panic("exit")
	}
	newTokenRefresherFunc = func() *tokenRefresher {
		return &tokenRefresher{
			userHomeDir: func() (string, error) { return "/home/test", nil },
			getenv:      func(string) string { return "" },
			tokenManagerFactory: func(string, string, ...webauthn.TokenManagerOption) (tokenManager, error) {
				return nil, errors.New("factory failed")
			},
			baseURL: defaultBaseURL,
		}
	}

	assert.PanicsWithValue(t, "exit", func() {
		main()
	})
	assert.Equal(t, 1, exitCode)
}
