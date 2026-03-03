package webauthn

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/chromedp"
)

const (
	authPath           = "/v2/page/app"
	maxAuthAttempts    = 3
	authAttemptTimeout = 2 * time.Minute
	authPollInterval   = 1 * time.Second
)

func getChromePath() string {
	if path := os.Getenv("CHROME_BIN"); path != "" {
		return path
	}
	if path := os.Getenv("CHROME_PATH"); path != "" {
		return path
	}
	// Common paths
	paths := []string{
		"/usr/bin/chromium-browser",
		"/usr/bin/chromium",
		"/usr/bin/google-chrome",
		"/usr/bin/google-chrome-stable",
		"/usr/bin/chrome",
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

type BrowserAuth struct {
	baseURL  string
	headless bool
}

func NewBrowserAuth(baseURL string) *BrowserAuth {
	return &BrowserAuth{
		baseURL:  baseURL,
		headless: true,
	}
}

func (ba *BrowserAuth) WithHeadless(headless bool) *BrowserAuth {
	ba.headless = headless
	return ba
}

func (ba *BrowserAuth) Authenticate() (*AuthTokens, error) {
	slog.Info("starting browser authentication")
	var lastErr error

	for attempt := 1; attempt <= maxAuthAttempts; attempt++ {
		slog.Debug("authentication attempt", "attempt", attempt)
		tokens, err := ba.authenticateAttempt()
		if err == nil {
			slog.Info("browser authentication successful")
			return tokens, nil
		}

		lastErr = err
		if attempt == maxAuthAttempts || !isTransientAuthError(err) {
			break
		}

		slog.Info("retrying authentication after transient error", "attempt", attempt, "error", err)
		time.Sleep(time.Duration(attempt) * time.Second)
	}

	slog.Error("browser authentication failed", "error", lastErr)
	return nil, fmt.Errorf("browser authentication failed after %d attempts: %w", maxAuthAttempts, lastErr)
}

func (ba *BrowserAuth) authenticateAttempt() (*AuthTokens, error) {
	chromePath := getChromePath()
	if chromePath == "" {
		return nil, fmt.Errorf("chrome/chromium not found: set CHROME_BIN environment variable or install Chrome")
	}

	slog.Info("using chrome binary", "path", chromePath)

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", ba.headless),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.ExecPath(chromePath),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-software-rasterizer", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-translate", true),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.WindowSize(1920, 1080),
		chromedp.UserAgent("Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()

	timeoutCtx, cancelTimeout := context.WithTimeout(browserCtx, authAttemptTimeout)
	defer cancelTimeout()

	var cookies []*network.Cookie
	loginURL := strings.TrimSuffix(ba.baseURL, "/") + authPath

	slog.Debug("navigating to login page", "url", loginURL)

	if err := chromedp.Run(timeoutCtx,
		network.Enable(),
		chromedp.Navigate(loginURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
	); err != nil {
		return nil, fmt.Errorf("failed to navigate to login page: %w", err)
	}

	slog.Debug("navigated to login page successfully")

	if err := ba.waitForAuthentication(timeoutCtx, &cookies); err != nil {
		return nil, fmt.Errorf("failed to complete webauthn flow: %w", err)
	}

	tokens, err := extractTokens(cookies)
	if err != nil {
		return nil, err
	}

	return tokens, nil
}

func (ba *BrowserAuth) waitForAuthentication(ctx context.Context, cookies *[]*network.Cookie) error {
	clickedAuthButton := false

	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("authentication timeout reached: %w", err)
		}

		state, err := ba.getPageState(ctx)
		if err != nil {
			return fmt.Errorf("failed to inspect authentication page: %w", err)
		}

		slog.Debug("detected auth state", "hasAuthButton", state.HasAuthButton, "hasWebAuthnPrompt", state.HasWebAuthnPrompt)

		if !clickedAuthButton && state.HasAuthButton {
			clicked, clickErr := ba.tryTriggerWebAuthn(ctx)
			if clickErr != nil {
				return fmt.Errorf("failed to trigger webauthn prompt: %w", clickErr)
			}
			clickedAuthButton = clicked
		}

		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context cancelled before reading cookies: %w", err)
		}

		var currentCookies []*network.Cookie
		if err := chromedp.Run(ctx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				var err error
				currentCookies, err = storage.GetCookies().Do(ctx)
				return err
			}),
		); err != nil {
			return fmt.Errorf("failed to read browser cookies: %w", err)
		}

		if hasAuthCookies(currentCookies) {
			*cookies = currentCookies
			return nil
		}

		if state.IsAuthenticatedURL && !state.HasWebAuthnPrompt {
			time.Sleep(authPollInterval)
			continue
		}

		time.Sleep(authPollInterval)
	}
}

type authPageState struct {
	URL                string `json:"url"`
	HasAuthButton      bool   `json:"hasAuthButton"`
	HasWebAuthnPrompt  bool   `json:"hasWebAuthnPrompt"`
	IsAuthenticatedURL bool   `json:"isAuthenticatedUrl"`
}

func (ba *BrowserAuth) getPageState(ctx context.Context) (*authPageState, error) {
	var state authPageState

	err := chromedp.Run(ctx, chromedp.Evaluate(`(() => {
		const bodyText = document.body ? document.body.innerText : "";
		const lowerBodyText = bodyText.toLowerCase();
		const authButtonSelectors = [
			"button[type='submit']",
			"button[data-testid*='login']",
			"button[id*='login']",
			"button[class*='login']",
			"[role='button'][data-testid*='login']"
		];
		const hasAuthButton = authButtonSelectors.some((selector) => {
			const el = document.querySelector(selector);
			if (!el) {
				return false;
			}
			const text = (el.textContent || "").toLowerCase();
			return text.includes("login") || text.includes("log in") || text.includes("entrar") || text.includes("passkey") || text.includes("security");
		});

		const hasWebAuthnPrompt =
			document.querySelector("input[autocomplete='webauthn']") !== null ||
			document.querySelector("[data-webauthn]") !== null ||
			document.querySelector("[id*='webauthn']") !== null ||
			document.querySelector("[class*='webauthn']") !== null ||
			lowerBodyText.includes("passkey") ||
			lowerBodyText.includes("security key") ||
			lowerBodyText.includes("biometric") ||
			lowerBodyText.includes("webauthn") ||
			lowerBodyText.includes("touch your");

		const path = window.location.pathname || "";
		const isAuthenticatedUrl = path === "/v2/page/app";

		return {
			url: window.location.href,
			hasAuthButton,
			hasWebAuthnPrompt,
			isAuthenticatedUrl,
		};
	})()`, &state))
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate auth page state: %w", err)
	}

	return &state, nil
}

func (ba *BrowserAuth) tryTriggerWebAuthn(ctx context.Context) (bool, error) {
	var clicked bool

	err := chromedp.Run(ctx, chromedp.Evaluate(`(() => {
		const selectors = [
			"button[type='submit']",
			"button[data-testid*='login']",
			"button[id*='login']",
			"button[class*='login']",
			"[role='button'][data-testid*='login']"
		];

		for (const selector of selectors) {
			const el = document.querySelector(selector);
			if (!el) {
				continue;
			}

			const text = (el.textContent || "").toLowerCase();
			if (text.includes("login") || text.includes("log in") || text.includes("entrar") || text.includes("passkey") || text.includes("security")) {
				el.click();
				return true;
			}
		}

		return false;
	})()`, &clicked))
	if err != nil {
		return false, fmt.Errorf("failed to execute auth trigger script: %w", err)
	}

	return clicked, nil
}

func extractTokens(cookies []*network.Cookie) (*AuthTokens, error) {
	tokens := &AuthTokens{
		ExpiresAt: time.Now().Add(8 * time.Hour),
	}

	for _, cookie := range cookies {
		switch cookie.Name {
		case "hglogin":
			tokens.HGLogin = cookie.Value
		case "X-Hourglass-XSRF-Token":
			tokens.XSRFToken = cookie.Value
		}
	}

	if tokens.HGLogin == "" || tokens.XSRFToken == "" {
		return nil, fmt.Errorf("failed to extract authentication cookies from browser")
	}

	return tokens, nil
}

func hasAuthCookies(cookies []*network.Cookie) bool {
	hasHGLogin := false
	hasXSRF := false

	for _, cookie := range cookies {
		switch cookie.Name {
		case "hglogin":
			hasHGLogin = cookie.Value != ""
		case "X-Hourglass-XSRF-Token":
			hasXSRF = cookie.Value != ""
		}
	}

	return hasHGLogin && hasXSRF
}

func isTransientAuthError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}

	errMsg := strings.ToLower(err.Error())
	transientSignals := []string{
		"net::err_",
		"target closed",
		"navigation timeout",
		"authentication timeout",
		"connection reset",
	}

	for _, signal := range transientSignals {
		if strings.Contains(errMsg, signal) {
			return true
		}
	}

	return false
}
