package webauthn

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func restoreBrowserAuthHooks(t *testing.T) {
	t.Helper()

	originalOsStat := osStat
	originalChromedpRun := chromedpRun
	originalSleepFn := sleepFn
	originalGetCookies := getCookies
	originalEvaluateAuthPageState := evaluateAuthPageState
	originalTriggerWebAuthnPrompt := triggerWebAuthnPrompt
	originalExtractAuthTokens := extractAuthTokens

	t.Cleanup(func() {
		osStat = originalOsStat
		chromedpRun = originalChromedpRun
		sleepFn = originalSleepFn
		getCookies = originalGetCookies
		evaluateAuthPageState = originalEvaluateAuthPageState
		triggerWebAuthnPrompt = originalTriggerWebAuthnPrompt
		extractAuthTokens = originalExtractAuthTokens
	})
}

func browserAuthCookies() []*network.Cookie {
	return []*network.Cookie{
		{Name: "hglogin", Value: "test-hglogin"},
		{Name: "X-Hourglass-XSRF-Token", Value: "test-xsrf"},
	}
}

func TestBrowserAuthConstructors(t *testing.T) {
	ba := NewBrowserAuth("https://example.com")
	assert.Equal(t, "https://example.com", ba.baseURL)
	assert.True(t, ba.headless)

	assert.Same(t, ba, ba.WithHeadless(false))
	assert.False(t, ba.headless)
}

func TestBrowserAuthTimingHelpers(t *testing.T) {
	t.Run("getPollInterval defaults", func(t *testing.T) {
		assert.Equal(t, defaultPollInterval, getPollInterval())
	})

	t.Run("getPollInterval uses test interval", func(t *testing.T) {
		t.Setenv("TEST_TIMEOUT_SHORT", "1")
		assert.Equal(t, testPollInterval, getPollInterval())
	})

	t.Run("getAuthAttemptTimeout defaults", func(t *testing.T) {
		assert.Equal(t, defaultAuthTimeout, getAuthAttemptTimeout())
	})

	t.Run("getAuthAttemptTimeout uses ci timeout", func(t *testing.T) {
		t.Setenv("CI", "true")
		assert.Equal(t, shortTestAuthTimeout, getAuthAttemptTimeout())
	})

	t.Run("getAuthAttemptTimeout uses short timeout", func(t *testing.T) {
		t.Setenv("TEST_TIMEOUT_SHORT", "1")
		assert.Equal(t, testAuthTimeout, getAuthAttemptTimeout())
	})

	t.Run("getRetryDelay defaults", func(t *testing.T) {
		assert.Equal(t, 2*time.Second, getRetryDelay(2))
	})

	t.Run("getRetryDelay uses test delay", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "true")
		assert.Equal(t, 100*time.Millisecond, getRetryDelay(3))
	})
}

func TestGetChromePath(t *testing.T) {
	t.Run("prefers CHROME_BIN", func(t *testing.T) {
		restoreBrowserAuthHooks(t)
		t.Setenv("CHROME_BIN", "/tmp/chrome-bin")
		t.Setenv("CHROME_PATH", "/tmp/chrome-path")
		assert.Equal(t, "/tmp/chrome-bin", getChromePath())
	})

	t.Run("uses CHROME_PATH", func(t *testing.T) {
		restoreBrowserAuthHooks(t)
		t.Setenv("CHROME_BIN", "")
		t.Setenv("CHROME_PATH", "/tmp/chrome-path")
		assert.Equal(t, "/tmp/chrome-path", getChromePath())
	})

	t.Run("uses discovered path", func(t *testing.T) {
		restoreBrowserAuthHooks(t)
		t.Setenv("CHROME_BIN", "")
		t.Setenv("CHROME_PATH", "")
		osStat = func(path string) (os.FileInfo, error) {
			if path == "/usr/bin/google-chrome" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		assert.Equal(t, "/usr/bin/google-chrome", getChromePath())
	})

	t.Run("returns empty when chrome missing", func(t *testing.T) {
		restoreBrowserAuthHooks(t)
		t.Setenv("CHROME_BIN", "")
		t.Setenv("CHROME_PATH", "")
		osStat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		assert.Empty(t, getChromePath())
	})
}

func TestBrowserAuthAuthenticate(t *testing.T) {
	t.Run("retries transient error and succeeds", func(t *testing.T) {
		restoreBrowserAuthHooks(t)
		t.Setenv("CHROME_BIN", "/tmp/chrome")

		navigateCalls := 0
		sleepCalls := 0
		chromedpRun = func(ctx context.Context, actions ...chromedp.Action) error {
			navigateCalls++
			if navigateCalls == 1 {
				return errors.New("navigation timeout")
			}
			return nil
		}
		sleepFn = func(time.Duration) { sleepCalls++ }
		evaluateAuthPageState = func(ctx context.Context, state *authPageState) error {
			*state = authPageState{HasAuthButton: true}
			return nil
		}
		triggerWebAuthnPrompt = func(ctx context.Context, clicked *bool) error {
			*clicked = true
			return nil
		}
		getCookies = func(ctx context.Context) ([]*network.Cookie, error) {
			return browserAuthCookies(), nil
		}

		tokens, err := NewBrowserAuth("https://example.com").Authenticate()
		require.NoError(t, err)
		require.NotNil(t, tokens)
		assert.Equal(t, "test-hglogin", tokens.HGLogin)
		assert.Equal(t, "test-xsrf", tokens.XSRFToken)
		assert.Equal(t, 2, navigateCalls)
		assert.Equal(t, 1, sleepCalls)
	})

	t.Run("stops after max transient attempts", func(t *testing.T) {
		restoreBrowserAuthHooks(t)
		t.Setenv("CHROME_BIN", "/tmp/chrome")

		navigateCalls := 0
		sleepCalls := 0
		chromedpRun = func(ctx context.Context, actions ...chromedp.Action) error {
			navigateCalls++
			return context.DeadlineExceeded
		}
		sleepFn = func(time.Duration) { sleepCalls++ }

		tokens, err := NewBrowserAuth("https://example.com").Authenticate()
		require.Error(t, err)
		assert.Nil(t, tokens)
		assert.Contains(t, err.Error(), "browser authentication failed after 3 attempts")
		assert.Equal(t, 3, navigateCalls)
		assert.Equal(t, 2, sleepCalls)
	})

	t.Run("fails fast on non transient error", func(t *testing.T) {
		restoreBrowserAuthHooks(t)
		t.Setenv("CHROME_BIN", "")
		t.Setenv("CHROME_PATH", "")
		osStat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		tokens, err := NewBrowserAuth("https://example.com").Authenticate()
		require.Error(t, err)
		assert.Nil(t, tokens)
		assert.Contains(t, err.Error(), "chrome/chromium not found")
	})
}

func TestBrowserAuthAuthenticateAttempt(t *testing.T) {
	t.Run("returns navigation error", func(t *testing.T) {
		restoreBrowserAuthHooks(t)
		t.Setenv("CHROME_BIN", "/tmp/chrome")
		chromedpRun = func(ctx context.Context, actions ...chromedp.Action) error {
			return errors.New("boom")
		}

		tokens, err := NewBrowserAuth("https://example.com").authenticateAttempt()
		require.Error(t, err)
		assert.Nil(t, tokens)
		assert.Contains(t, err.Error(), "failed to navigate to login page")
	})

	t.Run("returns wait error", func(t *testing.T) {
		restoreBrowserAuthHooks(t)
		t.Setenv("CHROME_BIN", "/tmp/chrome")
		chromedpRun = func(ctx context.Context, actions ...chromedp.Action) error {
			return nil
		}
		evaluateAuthPageState = func(ctx context.Context, state *authPageState) error {
			return errors.New("inspect failed")
		}

		tokens, err := NewBrowserAuth("https://example.com").authenticateAttempt()
		require.Error(t, err)
		assert.Nil(t, tokens)
		assert.Contains(t, err.Error(), "failed to complete webauthn flow")
	})

	t.Run("returns token extraction error", func(t *testing.T) {
		restoreBrowserAuthHooks(t)
		t.Setenv("CHROME_BIN", "/tmp/chrome")
		chromedpRun = func(ctx context.Context, actions ...chromedp.Action) error {
			return nil
		}
		evaluateAuthPageState = func(ctx context.Context, state *authPageState) error {
			*state = authPageState{HasAuthButton: true}
			return nil
		}
		triggerWebAuthnPrompt = func(ctx context.Context, clicked *bool) error {
			*clicked = true
			return nil
		}
		getCookies = func(ctx context.Context) ([]*network.Cookie, error) {
			return browserAuthCookies(), nil
		}
		extractAuthTokens = func(cookies []*network.Cookie) (*AuthTokens, error) {
			return nil, errors.New("token extraction failed")
		}

		tokens, err := NewBrowserAuth("https://example.com").authenticateAttempt()
		require.Error(t, err)
		assert.Nil(t, tokens)
		assert.Contains(t, err.Error(), "token extraction failed")
	})
}

func TestBrowserAuthWaitForAuthentication(t *testing.T) {
	t.Run("returns timeout when context already cancelled", func(t *testing.T) {
		restoreBrowserAuthHooks(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := NewBrowserAuth("https://example.com").waitForAuthentication(ctx, new([]*network.Cookie))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "authentication timeout reached")
	})

	t.Run("returns inspect error", func(t *testing.T) {
		restoreBrowserAuthHooks(t)
		evaluateAuthPageState = func(ctx context.Context, state *authPageState) error {
			return errors.New("inspect failed")
		}

		err := NewBrowserAuth("https://example.com").waitForAuthentication(context.Background(), new([]*network.Cookie))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to inspect authentication page")
	})

	t.Run("returns trigger error", func(t *testing.T) {
		restoreBrowserAuthHooks(t)
		evaluateAuthPageState = func(ctx context.Context, state *authPageState) error {
			*state = authPageState{HasAuthButton: true}
			return nil
		}
		triggerWebAuthnPrompt = func(ctx context.Context, clicked *bool) error {
			return errors.New("click failed")
		}

		err := NewBrowserAuth("https://example.com").waitForAuthentication(context.Background(), new([]*network.Cookie))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to trigger webauthn prompt")
	})

	t.Run("returns context cancelled before reading cookies", func(t *testing.T) {
		restoreBrowserAuthHooks(t)
		ctx, cancel := context.WithCancel(context.Background())
		evaluateAuthPageState = func(ctx context.Context, state *authPageState) error {
			cancel()
			*state = authPageState{}
			return nil
		}

		err := NewBrowserAuth("https://example.com").waitForAuthentication(ctx, new([]*network.Cookie))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "context cancelled before reading cookies")
	})

	t.Run("returns cookie read error", func(t *testing.T) {
		restoreBrowserAuthHooks(t)
		evaluateAuthPageState = func(ctx context.Context, state *authPageState) error {
			*state = authPageState{}
			return nil
		}
		getCookies = func(ctx context.Context) ([]*network.Cookie, error) {
			return nil, errors.New("cookies failed")
		}

		err := NewBrowserAuth("https://example.com").waitForAuthentication(context.Background(), new([]*network.Cookie))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read browser cookies")
	})

	t.Run("waits on authenticated url then succeeds", func(t *testing.T) {
		restoreBrowserAuthHooks(t)
		stateCalls := 0
		sleepCalls := 0
		evaluateAuthPageState = func(ctx context.Context, state *authPageState) error {
			stateCalls++
			if stateCalls == 1 {
				*state = authPageState{IsAuthenticatedURL: true}
				return nil
			}
			*state = authPageState{HasAuthButton: true}
			return nil
		}
		triggerWebAuthnPrompt = func(ctx context.Context, clicked *bool) error {
			*clicked = true
			return nil
		}
		getCookies = func(ctx context.Context) ([]*network.Cookie, error) {
			if stateCalls == 1 {
				return []*network.Cookie{{Name: "hglogin", Value: ""}}, nil
			}
			return browserAuthCookies(), nil
		}
		sleepFn = func(time.Duration) { sleepCalls++ }

		var cookies []*network.Cookie
		err := NewBrowserAuth("https://example.com").waitForAuthentication(context.Background(), &cookies)
		require.NoError(t, err)
		assert.Len(t, cookies, 2)
		assert.Equal(t, 1, sleepCalls)
	})

	t.Run("keeps polling until cookies appear", func(t *testing.T) {
		restoreBrowserAuthHooks(t)
		stateCalls := 0
		sleepCalls := 0
		evaluateAuthPageState = func(ctx context.Context, state *authPageState) error {
			stateCalls++
			*state = authPageState{HasWebAuthnPrompt: true}
			return nil
		}
		getCookies = func(ctx context.Context) ([]*network.Cookie, error) {
			if stateCalls == 1 {
				return []*network.Cookie{{Name: "other", Value: "value"}}, nil
			}
			return browserAuthCookies(), nil
		}
		sleepFn = func(time.Duration) { sleepCalls++ }

		var cookies []*network.Cookie
		err := NewBrowserAuth("https://example.com").waitForAuthentication(context.Background(), &cookies)
		require.NoError(t, err)
		assert.Len(t, cookies, 2)
		assert.Equal(t, 1, sleepCalls)
	})
}

func TestBrowserAuthPageHelpers(t *testing.T) {
	t.Run("getPageState success", func(t *testing.T) {
		restoreBrowserAuthHooks(t)
		evaluateAuthPageState = func(ctx context.Context, state *authPageState) error {
			*state = authPageState{
				URL:                "https://example.com/v2/page/app",
				HasAuthButton:      true,
				HasWebAuthnPrompt:  true,
				IsAuthenticatedURL: true,
			}
			return nil
		}

		state, err := NewBrowserAuth("https://example.com").getPageState(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "https://example.com/v2/page/app", state.URL)
		assert.True(t, state.HasAuthButton)
		assert.True(t, state.HasWebAuthnPrompt)
		assert.True(t, state.IsAuthenticatedURL)
	})

	t.Run("getPageState error", func(t *testing.T) {
		restoreBrowserAuthHooks(t)
		evaluateAuthPageState = func(ctx context.Context, state *authPageState) error {
			return errors.New("boom")
		}

		state, err := NewBrowserAuth("https://example.com").getPageState(context.Background())
		require.Error(t, err)
		assert.Nil(t, state)
		assert.Contains(t, err.Error(), "failed to evaluate auth page state")
	})

	t.Run("tryTriggerWebAuthn success", func(t *testing.T) {
		restoreBrowserAuthHooks(t)
		triggerWebAuthnPrompt = func(ctx context.Context, clicked *bool) error {
			*clicked = true
			return nil
		}

		clicked, err := NewBrowserAuth("https://example.com").tryTriggerWebAuthn(context.Background())
		require.NoError(t, err)
		assert.True(t, clicked)
	})

	t.Run("tryTriggerWebAuthn error", func(t *testing.T) {
		restoreBrowserAuthHooks(t)
		triggerWebAuthnPrompt = func(ctx context.Context, clicked *bool) error {
			return errors.New("boom")
		}

		clicked, err := NewBrowserAuth("https://example.com").tryTriggerWebAuthn(context.Background())
		require.Error(t, err)
		assert.False(t, clicked)
		assert.Contains(t, err.Error(), "failed to execute auth trigger script")
	})
}

func TestBrowserAuthDefaultHookClosures(t *testing.T) {
	restoreBrowserAuthHooks(t)

	defaultGetCookies := getCookies
	defaultEvaluateAuthPageState := evaluateAuthPageState
	defaultTriggerWebAuthnPrompt := triggerWebAuthnPrompt

	chromedpRun = func(ctx context.Context, actions ...chromedp.Action) error {
		return nil
	}

	state := authPageState{}
	require.NoError(t, defaultEvaluateAuthPageState(context.Background(), &state))

	clicked := false
	require.NoError(t, defaultTriggerWebAuthnPrompt(context.Background(), &clicked))

	_, err := defaultGetCookies(context.Background())
	require.Error(t, err)
}

func TestExtractTokens(t *testing.T) {
	t.Run("extracts tokens from cookies", func(t *testing.T) {
		tokens, err := extractTokens(browserAuthCookies())
		require.NoError(t, err)
		require.NotNil(t, tokens)
		assert.Equal(t, "test-hglogin", tokens.HGLogin)
		assert.Equal(t, "test-xsrf", tokens.XSRFToken)
		assert.False(t, tokens.IsExpired())
	})

	t.Run("returns error when cookies missing", func(t *testing.T) {
		tokens, err := extractTokens([]*network.Cookie{{Name: "other", Value: "value"}})
		require.Error(t, err)
		assert.Nil(t, tokens)
	})
}

func TestHasAuthCookies(t *testing.T) {
	assert.True(t, hasAuthCookies(browserAuthCookies()))
	assert.False(t, hasAuthCookies([]*network.Cookie{{Name: "hglogin", Value: "test"}}))
	assert.False(t, hasAuthCookies([]*network.Cookie{{Name: "X-Hourglass-XSRF-Token", Value: "test"}}))
	assert.False(t, hasAuthCookies([]*network.Cookie{{Name: "hglogin", Value: ""}}))
	assert.False(t, hasAuthCookies(nil))
}

func TestIsTransientAuthError(t *testing.T) {
	assert.True(t, isTransientAuthError(context.DeadlineExceeded))
	assert.True(t, isTransientAuthError(context.Canceled))
	assert.True(t, isTransientAuthError(errors.New("net::ERR_CONNECTION_RESET")))
	assert.True(t, isTransientAuthError(errors.New("target closed")))
	assert.False(t, isTransientAuthError(errors.New("invalid credentials")))
	assert.False(t, isTransientAuthError(nil))
}
