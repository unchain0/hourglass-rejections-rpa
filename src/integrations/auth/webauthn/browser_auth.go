package webauthn

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/chromedp"
)

const (
	authPath             = "/v2/page/app"
	maxAuthAttempts      = 3
	defaultAuthTimeout   = 2 * time.Minute
	testAuthTimeout      = 5 * time.Second
	shortTestAuthTimeout = 1 * time.Second
	defaultPollInterval  = 1 * time.Second
	testPollInterval     = 100 * time.Millisecond
	minimumChromeMajor   = 128
	browserCloseTimeout  = 2 * time.Second
	authCookieName       = "hglogin"
	xsrfCookieName       = "X-Hourglass-XSRF-Token"
)

var (
	osStat                    = os.Stat
	osMkdirAll                = os.MkdirAll
	osLstat                   = os.Lstat
	osReadlink                = os.Readlink
	osRemove                  = os.Remove
	chromeVersionOutput       = func(path string) ([]byte, error) { return exec.Command(path, "--version").Output() }
	chromedpRun               = chromedp.Run
	newExecAllocator          = chromedp.NewExecAllocator
	newBrowserContext         = chromedp.NewContext
	browserContextFromContext = chromedp.FromContext
	sleepFn                   = time.Sleep
	processExists             = func(pid int) bool {
		err := syscall.Kill(pid, 0)
		return err == nil || errors.Is(err, syscall.EPERM)
	}
	defaultReadCookies = func(ctx context.Context) ([]*network.Cookie, error) {
		return storage.GetCookies().Do(ctx)
	}
	chromedpCancel        = chromedp.Cancel
	getCookies            = readBrowserCookies
	evaluateAuthPageState = func(ctx context.Context, state *authPageState) error {
		return chromedpRun(ctx, chromedp.Evaluate(`(() => {
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
		})()`, state))
	}
	triggerWebAuthnPrompt = func(ctx context.Context, clicked *bool) error {
		return chromedpRun(ctx, chromedp.Evaluate(`(() => {
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
		})()`, clicked))
	}
	extractAuthTokens = extractTokens
)

func readBrowserCookies(ctx context.Context) ([]*network.Cookie, error) {
	var cookies []*network.Cookie

	err := chromedpRun(ctx, chromedp.ActionFunc(func(actionCtx context.Context) error {
		var err error
		cookies, err = defaultReadCookies(actionCtx)
		return err
	}))
	if err != nil {
		return nil, err
	}

	return cookies, nil
}

func getPollInterval() time.Duration {
	if os.Getenv("CI") == "true" || os.Getenv("GITHUB_ACTIONS") == "true" || os.Getenv("TEST_TIMEOUT_SHORT") == "1" {
		return testPollInterval
	}
	return defaultPollInterval
}

func getAuthAttemptTimeout() time.Duration {
	if os.Getenv("CI") == "true" || os.Getenv("GITHUB_ACTIONS") == "true" {
		return shortTestAuthTimeout
	}
	if os.Getenv("TEST_TIMEOUT_SHORT") == "1" {
		return testAuthTimeout
	}
	return defaultAuthTimeout
}

func getRetryDelay(attempt int) time.Duration {
	if os.Getenv("CI") == "true" || os.Getenv("GITHUB_ACTIONS") == "true" || os.Getenv("TEST_TIMEOUT_SHORT") == "1" {
		return 100 * time.Millisecond
	}
	return time.Duration(attempt) * time.Second
}

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
		if _, err := osStat(path); err == nil {
			return path
		}
	}
	return ""
}

type BrowserAuth struct {
	baseURL    string
	headless   bool
	profileDir string
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

func (ba *BrowserAuth) WithProfileDir(profileDir string) *BrowserAuth {
	ba.profileDir = profileDir
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
		sleepFn(getRetryDelay(attempt))
	}

	slog.Error("browser authentication failed", "error", lastErr)
	return nil, fmt.Errorf("browser authentication failed after %d attempts: %w", maxAuthAttempts, lastErr)
}

func (ba *BrowserAuth) ExtractTokensFromProfile() (*AuthTokens, error) {
	slog.Info("extracting authentication tokens from persisted browser profile")
	var lastErr error

	for attempt := 1; attempt <= maxAuthAttempts; attempt++ {
		tokens, err := ba.extractTokensFromProfileAttempt()
		if err == nil {
			slog.Info("browser profile token extraction successful")
			return tokens, nil
		}

		lastErr = err
		if attempt == maxAuthAttempts || !isTransientAuthError(err) {
			break
		}

		slog.Info("retrying browser profile token extraction after transient error", "attempt", attempt, "error", err)
		sleepFn(getRetryDelay(attempt))
	}

	return nil, fmt.Errorf("browser profile token extraction failed after %d attempts: %w", maxAuthAttempts, lastErr)
}

func (ba *BrowserAuth) authenticateAttempt() (*AuthTokens, error) {
	return ba.runBrowserFlow(true)
}

func (ba *BrowserAuth) extractTokensFromProfileAttempt() (*AuthTokens, error) {
	return ba.runBrowserFlow(false)
}

func (ba *BrowserAuth) runBrowserFlow(allowAuthTrigger bool) (*AuthTokens, error) {
	chromePath := getChromePath()
	if chromePath == "" {
		return nil, fmt.Errorf("chrome/chromium not found: set CHROME_BIN environment variable or install Chrome")
	}

	chromeVersion, err := validateChromeVersion(chromePath)
	if err != nil {
		return nil, err
	}

	slog.Info("using chrome binary", "path", chromePath, "version", chromeVersion)

	if ba.profileDir != "" {
		if err := PrepareChromeProfile(ba.profileDir); err != nil {
			return nil, err
		}
	}

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

	if ba.profileDir != "" {
		opts = append(opts, chromedp.UserDataDir(ba.profileDir))
	}

	allocCtx, cancel := newExecAllocator(context.Background(), opts...)
	defer cancel()

	browserCtx, cancelBrowser := newBrowserContext(allocCtx)
	defer closeBrowserContext(browserCtx, cancelBrowser)

	timeoutCtx, cancelTimeout := context.WithTimeout(browserCtx, getAuthAttemptTimeout())
	defer cancelTimeout()

	var cookies []*network.Cookie
	loginURL := strings.TrimSuffix(ba.baseURL, "/") + authPath

	slog.Debug("navigating to login page", "url", loginURL)

	if err := chromedpRun(timeoutCtx,
		network.Enable(),
		chromedp.Navigate(loginURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
	); err != nil {
		return nil, fmt.Errorf("failed to navigate to login page: %w", err)
	}

	slog.Debug("navigated to login page successfully")

	if err := ba.waitForAuthentication(timeoutCtx, &cookies, allowAuthTrigger); err != nil {
		return nil, fmt.Errorf("failed to complete browser auth flow: %w", err)
	}

	tokens, err := extractAuthTokens(cookies)
	if err != nil {
		return nil, err
	}

	return tokens, nil
}

func validateChromeVersion(path string) (string, error) {
	output, err := chromeVersionOutput(path)
	if err != nil {
		return "", fmt.Errorf("failed to read browser version from %q: %w", path, err)
	}

	version := strings.TrimSpace(string(output))
	major, err := chromeMajorVersion(version)
	if err != nil {
		return "", err
	}
	if major < minimumChromeMajor {
		return "", fmt.Errorf("unsupported browser %q: browser authentication requires Chromium %d or newer", version, minimumChromeMajor)
	}

	return version, nil
}

func chromeMajorVersion(version string) (int, error) {
	start := -1
	for i, char := range version {
		if char >= '0' && char <= '9' {
			start = i
			break
		}
	}
	if start == -1 {
		return 0, fmt.Errorf("could not parse browser version %q", version)
	}

	end := start
	for end < len(version) && version[end] >= '0' && version[end] <= '9' {
		end++
	}

	major, err := strconv.Atoi(version[start:end])
	if err != nil {
		return 0, fmt.Errorf("could not parse browser version %q: %w", version, err)
	}

	return major, nil
}

func (ba *BrowserAuth) waitForAuthentication(ctx context.Context, cookies *[]*network.Cookie, allowAuthTrigger bool) error {
	clickedAuthButton := false

	for {
		isAuthenticated, err := ba.processAuthenticationStep(ctx, cookies, allowAuthTrigger, &clickedAuthButton)
		if err != nil {
			return err
		}
		if isAuthenticated {
			return nil
		}
	}
}

func (ba *BrowserAuth) processAuthenticationStep(ctx context.Context, cookies *[]*network.Cookie, allowAuthTrigger bool, clickedAuthButton *bool) (bool, error) {
	if err := ba.verifyAuthContext(ctx, "authentication timeout reached"); err != nil {
		return false, err
	}

	state, err := ba.getPageState(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to inspect authentication page: %w", err)
	}

	slog.Debug("detected auth state", "hasAuthButton", state.HasAuthButton, "hasWebAuthnPrompt", state.HasWebAuthnPrompt)
	if err := ba.handleAuthTrigger(ctx, state, allowAuthTrigger, clickedAuthButton); err != nil {
		return false, err
	}

	if err := ba.waitForAuthCookies(ctx, cookies); err != nil {
		return false, err
	}

	if hasAuthCookies(*cookies) {
		return true, nil
	}

	sleepFn(getPollInterval())
	return false, nil
}

func (ba *BrowserAuth) verifyAuthContext(ctx context.Context, messagePrefix string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s: %w", messagePrefix, err)
	}

	return nil
}

func (ba *BrowserAuth) handleAuthTrigger(ctx context.Context, state *authPageState, allowAuthTrigger bool, clickedAuthButton *bool) error {
	if !allowAuthTrigger || *clickedAuthButton || !state.HasAuthButton {
		return nil
	}

	clicked, clickErr := ba.tryTriggerWebAuthn(ctx)
	if clickErr != nil {
		return fmt.Errorf("failed to trigger webauthn prompt: %w", clickErr)
	}

	*clickedAuthButton = clicked
	return nil
}

func (ba *BrowserAuth) waitForAuthCookies(ctx context.Context, cookies *[]*network.Cookie) error {
	if err := ba.verifyAuthContext(ctx, "context cancelled before reading cookies"); err != nil {
		return err
	}

	currentCookies, err := getCookies(ctx)
	if err != nil {
		return fmt.Errorf("failed to read browser cookies: %w", err)
	}

	if hasAuthCookies(currentCookies) {
		*cookies = currentCookies
		return nil
	}

	return nil
}

type authPageState struct {
	URL                string `json:"url"`
	HasAuthButton      bool   `json:"hasAuthButton"`
	HasWebAuthnPrompt  bool   `json:"hasWebAuthnPrompt"`
	IsAuthenticatedURL bool   `json:"isAuthenticatedUrl"`
}

func (ba *BrowserAuth) getPageState(ctx context.Context) (*authPageState, error) {
	var state authPageState

	err := evaluateAuthPageState(ctx, &state)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate auth page state: %w", err)
	}

	return &state, nil
}

func (ba *BrowserAuth) tryTriggerWebAuthn(ctx context.Context) (bool, error) {
	var clicked bool

	err := triggerWebAuthnPrompt(ctx, &clicked)
	if err != nil {
		return false, fmt.Errorf("failed to execute auth trigger script: %w", err)
	}

	return clicked, nil
}

func closeBrowserContext(browserCtx context.Context, cancelBrowser context.CancelFunc) {
	browserContext := browserContextFromContext(browserCtx)
	if browserContext != nil && browserContext.Browser != nil {
		closeCtx, cancelClose := context.WithTimeout(browserCtx, browserCloseTimeout)
		defer cancelClose()
		if err := chromedpCancel(closeCtx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			slog.Warn("failed to close browser context gracefully", "error", err)
		}
	}
	cancelBrowser()
}

func waitForBrowserConfirmation() error {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("🔐 O Chrome normal será aberto com o perfil persistente.")
	fmt.Println("👉 Faça o login manualmente no Google/Hourglass, confirme que chegou no app e feche essa janela do Chrome.")
	fmt.Print("⏎ Depois disso, pressione Enter para extrair os tokens do perfil salvo... ")
	_, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read confirmation input: %w", err)
	}
	return nil
}

func PrepareChromeProfile(profileDir string) error {
	if profileDir == "" {
		return nil
	}

	if err := osMkdirAll(profileDir, 0o700); err != nil {
		return fmt.Errorf("failed to create chrome profile directory: %w", err)
	}

	return cleanupStaleSingletonArtifacts(profileDir)
}

func cleanupStaleSingletonArtifacts(profileDir string) error {
	lockPath := filepath.Join(profileDir, "SingletonLock")
	info, err := osLstat(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to inspect chrome singleton lock: %w", err)
	}

	if info.Mode()&os.ModeSymlink == 0 {
		return nil
	}

	target, err := osReadlink(lockPath)
	if err != nil {
		return fmt.Errorf("failed to read chrome singleton lock: %w", err)
	}

	parts := strings.Split(target, "-")
	pid, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return nil
	}

	if processExists(pid) {
		return nil
	}

	for _, name := range []string{"SingletonLock", "SingletonCookie", "SingletonSocket"} {
		path := filepath.Join(profileDir, name)
		if err := osRemove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove stale chrome singleton artifact %s: %w", name, err)
		}
	}

	return nil
}

func extractTokens(cookies []*network.Cookie) (*AuthTokens, error) {
	tokens := &AuthTokens{
		ExpiresAt: time.Now().Add(8 * time.Hour),
	}
	var earliestExpiry time.Time

	for _, cookie := range cookies {
		switch cookie.Name {
		case authCookieName:
			tokens.HGLogin = cookie.Value
			setCookieExpiry(cookie, &earliestExpiry)
		case xsrfCookieName:
			tokens.XSRFToken = cookie.Value
			setCookieExpiry(cookie, &earliestExpiry)
		}
	}

	if !earliestExpiry.IsZero() {
		tokens.ExpiresAt = earliestExpiry
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
		case authCookieName:
			hasHGLogin = cookie.Value != ""
		case xsrfCookieName:
			hasXSRF = cookie.Value != ""
		}
	}

	return hasHGLogin && hasXSRF
}

func setCookieExpiry(cookie *network.Cookie, earliestExpiry *time.Time) {
	if cookie == nil || cookie.Session || cookie.Expires <= 0 {
		return
	}

	expiry := time.Unix(int64(cookie.Expires), 0)
	if earliestExpiry.IsZero() || expiry.Before(*earliestExpiry) {
		*earliestExpiry = expiry
	}
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
