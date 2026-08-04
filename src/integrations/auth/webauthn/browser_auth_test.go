package webauthn

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/chromedp/cdproto"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/go-json-experiment/json/jsontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func restoreBrowserAuthHooks(t *testing.T) {
	t.Helper()

	originalOsStat := osStat
	originalChromedpRun := chromedpRun
	originalNewExecAllocator := newExecAllocator
	originalNewBrowserContext := newBrowserContext
	originalBrowserContextFromContext := browserContextFromContext
	originalSleepFn := sleepFn
	originalDefaultReadCookies := defaultReadCookies
	originalChromedpCancel := chromedpCancel
	originalGetCookies := getCookies
	originalEvaluateAuthPageState := evaluateAuthPageState
	originalTriggerWebAuthnPrompt := triggerWebAuthnPrompt
	originalExtractAuthTokens := extractAuthTokens
	originalProcessExists := processExists
	originalChromeVersionOutput := chromeVersionOutput

	newExecAllocator = func(parent context.Context, _ ...chromedp.ExecAllocatorOption) (context.Context, context.CancelFunc) {
		return parent, func() {}
	}
	newBrowserContext = func(parent context.Context, _ ...chromedp.ContextOption) (context.Context, context.CancelFunc) {
		return parent, func() {}
	}
	chromedpCancel = func(ctx context.Context) error {
		_, hasDeadline := ctx.Deadline()
		assert.True(t, hasDeadline)
		return nil
	}
	chromeVersionOutput = func(string) ([]byte, error) { return []byte("Chromium 149.0.0.0"), nil }

	t.Cleanup(func() {
		osStat = originalOsStat
		chromedpRun = originalChromedpRun
		newExecAllocator = originalNewExecAllocator
		newBrowserContext = originalNewBrowserContext
		browserContextFromContext = originalBrowserContextFromContext
		sleepFn = originalSleepFn
		defaultReadCookies = originalDefaultReadCookies
		chromedpCancel = originalChromedpCancel
		getCookies = originalGetCookies
		evaluateAuthPageState = originalEvaluateAuthPageState
		triggerWebAuthnPrompt = originalTriggerWebAuthnPrompt
		extractAuthTokens = originalExtractAuthTokens
		processExists = originalProcessExists
		chromeVersionOutput = originalChromeVersionOutput
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
	assert.Empty(t, ba.profileDir)

	assert.Same(t, ba, ba.WithHeadless(false))
	assert.False(t, ba.headless)
	assert.Same(t, ba, ba.WithProfileDir("/tmp/hourglass-profile"))
	assert.Equal(t, "/tmp/hourglass-profile", ba.profileDir)
}

func TestCDPDecodesSupportedCookiePartitionKeyObject(t *testing.T) {
	msg := &cdproto.Message{
		Method: cdproto.EventNetworkResponseReceivedExtraInfo,
		Params: jsontext.Value(`{
			"requestId":"request-1",
			"blockedCookies":[],
			"headers":{},
			"resourceIPAddressSpace":"Unknown",
			"statusCode":200,
			"cookiePartitionKey":{
				"topLevelSite":"https://app.hourglass-app.com",
				"hasCrossSiteAncestor":false
			},
			"cookiePartitionKeyOpaque":false
		}`),
	}

	event, err := cdproto.UnmarshalMessage(msg, chromedp.DefaultUnmarshalOptions)
	require.NoError(t, err)
	require.IsType(t, &network.EventResponseReceivedExtraInfo{}, event)
}

func TestBrowserAuthRejectsChromeOlderThanCookiePartitionKeySchema(t *testing.T) {
	restoreBrowserAuthHooks(t)

	t.Setenv("CHROME_BIN", "/usr/bin/chromium-browser")
	chromeVersionOutput = func(string) ([]byte, error) { return []byte("Chromium 124.0.6367.78"), nil }

	browserStarted := false
	chromedpRun = func(context.Context, ...chromedp.Action) error {
		browserStarted = true
		return errors.New("browser should not start")
	}

	tokens, err := NewBrowserAuth("https://example.com").authenticateAttempt()
	require.Error(t, err)
	assert.Nil(t, tokens)
	assert.False(t, browserStarted)
	assert.Contains(t, err.Error(), "requires Chromium 128 or newer")
}

func TestChromeMajorVersion(t *testing.T) {
	major, err := chromeMajorVersion("Chromium 149.0.7827.53")
	require.NoError(t, err)
	assert.Equal(t, 149, major)

	_, err = chromeMajorVersion("Chromium unknown")
	require.Error(t, err)
}

func TestBrowserAuthTimingHelpers(t *testing.T) {
	t.Run("getPollInterval defaults", func(t *testing.T) {
		t.Setenv("CI", "")
		t.Setenv("GITHUB_ACTIONS", "")
		t.Setenv("TEST_TIMEOUT_SHORT", "")
		assert.Equal(t, defaultPollInterval, getPollInterval())
	})

	t.Run("getPollInterval uses test interval", func(t *testing.T) {
		t.Setenv("CI", "")
		t.Setenv("GITHUB_ACTIONS", "")
		t.Setenv("TEST_TIMEOUT_SHORT", "1")
		assert.Equal(t, testPollInterval, getPollInterval())
	})

	t.Run("getAuthAttemptTimeout defaults", func(t *testing.T) {
		t.Setenv("CI", "")
		t.Setenv("GITHUB_ACTIONS", "")
		t.Setenv("TEST_TIMEOUT_SHORT", "")
		assert.Equal(t, defaultAuthTimeout, getAuthAttemptTimeout())
	})

	t.Run("getAuthAttemptTimeout uses ci timeout", func(t *testing.T) {
		t.Setenv("CI", "true")
		t.Setenv("TEST_TIMEOUT_SHORT", "")
		assert.Equal(t, shortTestAuthTimeout, getAuthAttemptTimeout())
	})

	t.Run("getAuthAttemptTimeout uses short timeout", func(t *testing.T) {
		t.Setenv("CI", "")
		t.Setenv("GITHUB_ACTIONS", "")
		t.Setenv("TEST_TIMEOUT_SHORT", "1")
		assert.Equal(t, testAuthTimeout, getAuthAttemptTimeout())
	})

	t.Run("getRetryDelay defaults", func(t *testing.T) {
		t.Setenv("CI", "")
		t.Setenv("GITHUB_ACTIONS", "")
		t.Setenv("TEST_TIMEOUT_SHORT", "")
		assert.Equal(t, 2*time.Second, getRetryDelay(2))
	})

	t.Run("getRetryDelay uses test delay", func(t *testing.T) {
		t.Setenv("CI", "")
		t.Setenv("TEST_TIMEOUT_SHORT", "")
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

	t.Run("creates configured profile directory before browser run", func(t *testing.T) {
		restoreBrowserAuthHooks(t)
		t.Setenv("CHROME_BIN", "/tmp/chrome")

		profileDir := filepath.Join(t.TempDir(), "profiles", "hourglass")
		chromedpRun = func(ctx context.Context, actions ...chromedp.Action) error {
			return errors.New("boom")
		}

		_, err := NewBrowserAuth("https://example.com").WithProfileDir(profileDir).authenticateAttempt()
		require.Error(t, err)

		info, statErr := os.Stat(profileDir)
		require.NoError(t, statErr)
		assert.True(t, info.IsDir())
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
		assert.Contains(t, err.Error(), "failed to complete browser auth flow")
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

func TestBrowserAuthExtractTokensFromProfile(t *testing.T) {
	t.Run("extracts tokens without triggering auth button", func(t *testing.T) {
		restoreBrowserAuthHooks(t)
		t.Setenv("CHROME_BIN", "/tmp/chrome")

		triggered := false
		stateCalls := 0
		evaluateAuthPageState = func(ctx context.Context, state *authPageState) error {
			stateCalls++
			*state = authPageState{HasAuthButton: true, IsAuthenticatedURL: true}
			return nil
		}
		triggerWebAuthnPrompt = func(ctx context.Context, clicked *bool) error {
			triggered = true
			*clicked = true
			return nil
		}
		getCookies = func(ctx context.Context) ([]*network.Cookie, error) {
			if stateCalls == 1 {
				return []*network.Cookie{{Name: "other", Value: "value"}}, nil
			}
			return browserAuthCookies(), nil
		}
		sleepFn = func(time.Duration) {}
		chromedpRun = func(ctx context.Context, actions ...chromedp.Action) error {
			return nil
		}

		tokens, err := NewBrowserAuth("https://example.com").WithProfileDir(t.TempDir()).ExtractTokensFromProfile()
		require.NoError(t, err)
		assert.False(t, triggered)
		assert.Equal(t, "test-hglogin", tokens.HGLogin)
		assert.Equal(t, "test-xsrf", tokens.XSRFToken)
	})

	t.Run("retries transient error then succeeds", func(t *testing.T) {
		restoreBrowserAuthHooks(t)
		t.Setenv("CHROME_BIN", "/tmp/chrome")
		attempts := 0
		sleeps := 0
		sleepFn = func(time.Duration) { sleeps++ }
		chromedpRun = func(ctx context.Context, actions ...chromedp.Action) error { return nil }
		evaluateAuthPageState = func(ctx context.Context, state *authPageState) error {
			attempts++
			*state = authPageState{IsAuthenticatedURL: true}
			return nil
		}
		getCookies = func(ctx context.Context) ([]*network.Cookie, error) {
			if attempts == 1 {
				return nil, context.DeadlineExceeded
			}
			return browserAuthCookies(), nil
		}

		tokens, err := NewBrowserAuth("https://example.com").ExtractTokensFromProfile()
		require.NoError(t, err)
		assert.Equal(t, 1, sleeps)
		assert.Equal(t, "test-hglogin", tokens.HGLogin)
	})

	t.Run("stops on non transient error", func(t *testing.T) {
		restoreBrowserAuthHooks(t)
		t.Setenv("CHROME_BIN", "/tmp/chrome")
		chromedpRun = func(ctx context.Context, actions ...chromedp.Action) error { return nil }
		evaluateAuthPageState = func(ctx context.Context, state *authPageState) error {
			*state = authPageState{IsAuthenticatedURL: true}
			return nil
		}
		getCookies = func(ctx context.Context) ([]*network.Cookie, error) {
			return nil, errors.New("hard failure")
		}

		tokens, err := NewBrowserAuth("https://example.com").ExtractTokensFromProfile()
		assert.Nil(t, tokens)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "hard failure")
	})
}

func TestExtractTokens_UsesEarliestCookieExpiry(t *testing.T) {
	expiresAt := time.Now().Add(90 * time.Minute).Round(time.Second)
	tokens, err := extractTokens([]*network.Cookie{
		{Name: "hglogin", Value: "test-hglogin", Expires: cdpTime(expiresAt)},
		{Name: "X-Hourglass-XSRF-Token", Value: "test-xsrf", Expires: cdpTime(expiresAt.Add(30 * time.Minute))},
	})
	require.NoError(t, err)
	assert.Equal(t, expiresAt, tokens.ExpiresAt)
}

func cdpTime(ts time.Time) float64 {
	return float64(ts.Unix())
}

func TestBrowserAuthWaitForAuthentication(t *testing.T) {
	t.Run("returns timeout when context already cancelled", func(t *testing.T) {
		restoreBrowserAuthHooks(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := NewBrowserAuth("https://example.com").waitForAuthentication(ctx, new([]*network.Cookie), true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "authentication timeout reached")
	})

	t.Run("returns inspect error", func(t *testing.T) {
		restoreBrowserAuthHooks(t)
		evaluateAuthPageState = func(ctx context.Context, state *authPageState) error {
			return errors.New("inspect failed")
		}

		err := NewBrowserAuth("https://example.com").waitForAuthentication(context.Background(), new([]*network.Cookie), true)
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

		err := NewBrowserAuth("https://example.com").waitForAuthentication(context.Background(), new([]*network.Cookie), true)
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

		err := NewBrowserAuth("https://example.com").waitForAuthentication(ctx, new([]*network.Cookie), true)
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

		err := NewBrowserAuth("https://example.com").waitForAuthentication(context.Background(), new([]*network.Cookie), true)
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
		err := NewBrowserAuth("https://example.com").waitForAuthentication(context.Background(), &cookies, true)
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
		err := NewBrowserAuth("https://example.com").waitForAuthentication(context.Background(), &cookies, true)
		require.NoError(t, err)
		assert.Len(t, cookies, 2)
		assert.Equal(t, 1, sleepCalls)
	})
}

func TestBrowserAuthWaitForBrowserConfirmation(t *testing.T) {
	t.Run("reads newline successfully", func(t *testing.T) {
		oldStdin := os.Stdin
		r, w, err := os.Pipe()
		require.NoError(t, err)
		os.Stdin = r
		defer func() {
			os.Stdin = oldStdin
			_ = r.Close()
		}()

		go func() {
			_, _ = fmt.Fprintln(w)
			_ = w.Close()
		}()

		assert.NoError(t, waitForBrowserConfirmation())
	})

	t.Run("returns read error", func(t *testing.T) {
		oldStdin := os.Stdin
		r, w, err := os.Pipe()
		require.NoError(t, err)
		os.Stdin = r
		defer func() {
			os.Stdin = oldStdin
			_ = r.Close()
		}()
		_ = w.Close()

		err = waitForBrowserConfirmation()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read confirmation input")
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

	defaultRead := defaultReadCookies
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

	_, err := defaultRead(context.Background())
	require.Error(t, err)

	cookies, err := defaultGetCookies(context.Background())
	require.NoError(t, err)
	assert.Nil(t, cookies)
}

func TestReadBrowserCookiesUsesChromedpRun(t *testing.T) {
	restoreBrowserAuthHooks(t)

	expected := browserAuthCookies()
	called := false
	chromedpRun = func(ctx context.Context, actions ...chromedp.Action) error {
		called = true
		for _, action := range actions {
			require.NoError(t, action.Do(ctx))
		}
		return nil
	}

	defaultReadCookies = func(ctx context.Context) ([]*network.Cookie, error) {
		return expected, nil
	}

	actual, err := readBrowserCookies(context.Background())
	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, expected, actual)
}

func TestCloseBrowserContext(t *testing.T) {
	restoreBrowserAuthHooks(t)
	cancelled := false
	browserContextFromContext = func(ctx context.Context) *chromedp.Context {
		return &chromedp.Context{Browser: &chromedp.Browser{}}
	}
	chromedpCancel = func(context.Context) error { return nil }
	closeBrowserContext(context.Background(), func() { cancelled = true })
	assert.True(t, cancelled)
}

func TestCloseBrowserContext_IgnoresCanceledError(t *testing.T) {
	restoreBrowserAuthHooks(t)
	browserContextFromContext = func(ctx context.Context) *chromedp.Context {
		return &chromedp.Context{Browser: &chromedp.Browser{}}
	}
	chromedpCancel = func(context.Context) error { return context.Canceled }
	assert.NotPanics(t, func() { closeBrowserContext(context.Background(), func() {}) })
}

func TestCloseBrowserContext_LogsNonCanceledError(t *testing.T) {
	restoreBrowserAuthHooks(t)
	browserContextFromContext = func(ctx context.Context) *chromedp.Context {
		return &chromedp.Context{Browser: &chromedp.Browser{}}
	}
	chromedpCancel = func(context.Context) error { return errors.New("close failed") }
	assert.NotPanics(t, func() { closeBrowserContext(context.Background(), func() {}) })
}

func TestCloseBrowserContext_WithoutBrowser(t *testing.T) {
	restoreBrowserAuthHooks(t)
	browserContextFromContext = func(ctx context.Context) *chromedp.Context { return nil }
	assert.NotPanics(t, func() { closeBrowserContext(context.Background(), func() {}) })
}

func TestReadBrowserCookies_ReturnsChromedpError(t *testing.T) {
	restoreBrowserAuthHooks(t)
	chromedpRun = func(ctx context.Context, actions ...chromedp.Action) error {
		return errors.New("chromedp failed")
	}

	actual, err := readBrowserCookies(context.Background())
	assert.Nil(t, actual)
	assert.EqualError(t, err, "chromedp failed")
}

func TestPrepareChromeProfileRemovesStaleSingletonArtifacts(t *testing.T) {
	restoreBrowserAuthHooks(t)

	profileDir := t.TempDir()
	pid := 424242
	processExists = func(got int) bool {
		return got != pid
	}

	for _, name := range []string{"SingletonLock", "SingletonCookie", "SingletonSocket"} {
		require.NoError(t, os.Symlink("mint-"+strconv.Itoa(pid), filepath.Join(profileDir, name)))
	}

	require.NoError(t, PrepareChromeProfile(profileDir))

	for _, name := range []string{"SingletonLock", "SingletonCookie", "SingletonSocket"} {
		_, err := os.Lstat(filepath.Join(profileDir, name))
		assert.True(t, os.IsNotExist(err), name)
	}
}

func TestPrepareChromeProfileKeepsLiveSingletonArtifacts(t *testing.T) {
	restoreBrowserAuthHooks(t)

	profileDir := t.TempDir()
	pid := 31337
	processExists = func(got int) bool {
		return got == pid
	}

	lockPath := filepath.Join(profileDir, "SingletonLock")
	require.NoError(t, os.Symlink("mint-"+strconv.Itoa(pid), lockPath))

	require.NoError(t, PrepareChromeProfile(profileDir))

	_, err := os.Lstat(lockPath)
	require.NoError(t, err)
}

func TestPrepareChromeProfile_EmptyPath(t *testing.T) {
	restoreBrowserAuthHooks(t)
	require.NoError(t, PrepareChromeProfile(""))
}

func TestPrepareChromeProfile_MkdirError(t *testing.T) {
	restoreBrowserAuthHooks(t)
	osMkdirAll = func(path string, perm os.FileMode) error { return errors.New("mkdir failed") }
	err := PrepareChromeProfile(t.TempDir())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create chrome profile directory")
}

func TestCleanupStaleSingletonArtifacts_ErrorPaths(t *testing.T) {
	restoreBrowserAuthHooks(t)
	profileDir := t.TempDir()
	lockPath := filepath.Join(profileDir, "SingletonLock")
	require.NoError(t, os.WriteFile(lockPath, []byte("not-symlink"), 0600))
	require.NoError(t, cleanupStaleSingletonArtifacts(profileDir))

	osLstat = func(path string) (os.FileInfo, error) { return nil, errors.New("lstat failed") }
	err := cleanupStaleSingletonArtifacts(profileDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to inspect chrome singleton lock")
}

func TestCleanupStaleSingletonArtifacts_ReadlinkError(t *testing.T) {
	restoreBrowserAuthHooks(t)
	profileDir := t.TempDir()
	lockPath := filepath.Join(profileDir, "SingletonLock")
	require.NoError(t, os.Symlink("mint-123", lockPath))
	osLstat = os.Lstat
	osReadlink = func(path string) (string, error) { return "", errors.New("readlink failed") }
	err := cleanupStaleSingletonArtifacts(profileDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read chrome singleton lock")
}

func TestCleanupStaleSingletonArtifacts_InvalidPIDReturnsNil(t *testing.T) {
	restoreBrowserAuthHooks(t)
	profileDir := t.TempDir()
	lockPath := filepath.Join(profileDir, "SingletonLock")
	require.NoError(t, os.Symlink("mint-not-a-pid", lockPath))
	osReadlink = os.Readlink
	require.NoError(t, cleanupStaleSingletonArtifacts(profileDir))
}

func TestCleanupStaleSingletonArtifacts_NoLockReturnsNil(t *testing.T) {
	restoreBrowserAuthHooks(t)
	require.NoError(t, cleanupStaleSingletonArtifacts(t.TempDir()))
}

func TestCleanupStaleSingletonArtifacts_RemoveError(t *testing.T) {
	restoreBrowserAuthHooks(t)
	profileDir := t.TempDir()
	lockPath := filepath.Join(profileDir, "SingletonLock")
	require.NoError(t, os.Symlink("mint-424242", lockPath))
	processExists = func(pid int) bool { return false }
	osRemove = func(_ string) error { return errors.New("remove failed") }
	err := cleanupStaleSingletonArtifacts(profileDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove stale chrome singleton artifact")
}

func TestRunBrowserFlow_PrepareProfileError(t *testing.T) {
	restoreBrowserAuthHooks(t)
	t.Setenv("CHROME_BIN", "/tmp/chrome")
	osMkdirAll = func(_ string, _ os.FileMode) error { return errors.New("mkdir failed") }
	_, err := NewBrowserAuth("https://example.com").WithProfileDir(t.TempDir()).runBrowserFlow(true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create chrome profile directory")
}

func TestSetHTTPCookieExpiry(t *testing.T) {
	var earliest time.Time
	setHTTPCookieExpiry(nil, &earliest)
	assert.True(t, earliest.IsZero())

	setHTTPCookieExpiry(&http.Cookie{}, &earliest)
	assert.True(t, earliest.IsZero())

	late := time.Now().Add(2 * time.Hour)
	early := time.Now().Add(time.Hour)
	setHTTPCookieExpiry(&http.Cookie{Expires: late}, &earliest)
	setHTTPCookieExpiry(&http.Cookie{Expires: early}, &earliest)
	assert.Equal(t, early, earliest)
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
