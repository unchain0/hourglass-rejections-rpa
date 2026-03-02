// Package scraper provides browser automation for Hourglass using Playwright.
package scraper

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"hourglass-rejections-rpa/internal/domain"

	"github.com/playwright-community/playwright-go"
)

const (
	loginURL           = "https://app.hourglass-app.com/v2/page/app"
	selectorEmail      = "input#email"
	selectorPassword   = "input#password"
	selectorSubmit     = "button[type='submit']"
	pageLoadTimeout    = 30000 // 30 seconds in milliseconds
	navigationTimeout  = 15000 // 15 seconds in milliseconds
	maxLoginRetries    = 3
	authSuccessPattern = "/page/app"
	dashboardPattern   = "/dashboard"
)

// pwRun abstracts playwright.Run for testing.
var pwRun = playwright.Run

// PlaywrightScraper implements domain.Scraper using browser automation.
type PlaywrightScraper struct {
	browser  playwright.Browser
	context  playwright.BrowserContext
	page     playwright.Page
	pw       *playwright.Playwright
	email    string
	password string
	storage  domain.Storage
	headless bool
	logger   *slog.Logger
}

// NewPlaywrightScraper creates a new PlaywrightScraper instance.
func NewPlaywrightScraper(email, password string, storage domain.Storage, headless bool) *PlaywrightScraper {
	return &PlaywrightScraper{
		email:    email,
		password: password,
		storage:  storage,
		headless: headless,
		logger:   slog.Default(),
	}
}

// Setup initializes the Playwright browser and context.
func (s *PlaywrightScraper) Setup(ctx context.Context) error {
	pw, err := pwRun()
	if err != nil {
		return fmt.Errorf("failed to start playwright: %w", err)
	}
	s.pw = pw

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(s.headless),
	})
	if err != nil {
		pw.Stop()
		return fmt.Errorf("failed to launch browser: %w", err)
	}
	s.browser = browser

	// Load existing cookies if available
	cookies, err := s.storage.LoadCookies()
	if err != nil {
		s.logger.Warn("failed to load cookies, starting fresh", "error", err)
	}

	browserContext, err := browser.NewContext()
	if err != nil {
		browser.Close()
		pw.Stop()
		return fmt.Errorf("failed to create browser context: %w", err)
	}
	s.context = browserContext

	// Inject stored cookies into the browser context
	if len(cookies) > 0 {
		pwCookies := domainCookiesToPlaywright(cookies)
		if err := browserContext.AddCookies(pwCookies); err != nil {
			s.logger.Warn("failed to add stored cookies", "error", err)
		}
	}

	page, err := browserContext.NewPage()
	if err != nil {
		browserContext.Close()
		browser.Close()
		pw.Stop()
		return fmt.Errorf("failed to create page: %w", err)
	}
	s.page = page

	s.logger.Info("playwright setup complete", "headless", s.headless)
	return nil
}

// Login performs authentication against the Hourglass app.
// It retries up to maxLoginRetries times on failure.
func (s *PlaywrightScraper) Login(ctx context.Context) error {
	var lastErr error

	for attempt := 1; attempt <= maxLoginRetries; attempt++ {
		s.logger.Info("login attempt", "attempt", attempt, "max", maxLoginRetries)

		if err := s.attemptLogin(ctx); err != nil {
			lastErr = err
			s.logger.Warn("login attempt failed", "attempt", attempt, "error", err)

			if attempt < maxLoginRetries {
				time.Sleep(time.Duration(attempt) * time.Second)
			}
			continue
		}

		// Login succeeded — extract and save cookies
		if err := s.extractAndSaveCookies(); err != nil {
			s.logger.Warn("failed to save cookies after login", "error", err)
		}

		s.logger.Info("login successful")
		return nil
	}

	return fmt.Errorf("%w: after %d attempts: %v", domain.ErrAuthenticationFailed, maxLoginRetries, lastErr)
}

// attemptLogin performs a single login attempt.
func (s *PlaywrightScraper) attemptLogin(ctx context.Context) error {
	if s.page == nil {
		return fmt.Errorf("page not initialized: call Setup() first")
	}
	// Navigate to the login page
	timeout := float64(pageLoadTimeout)
	if _, err := s.page.Goto(loginURL, playwright.PageGotoOptions{
		Timeout:   &timeout,
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	}); err != nil {
		return fmt.Errorf("failed to navigate to login page: %w", err)
	}

	// Wait for the email input to be visible
	waitTimeout := float64(pageLoadTimeout)
	if err := s.page.Locator(selectorEmail).WaitFor(playwright.LocatorWaitForOptions{
		Timeout: &waitTimeout,
		State:   playwright.WaitForSelectorStateVisible,
	}); err != nil {
		return fmt.Errorf("login form not found: %w", err)
	}

	// Fill email
	if err := s.page.Locator(selectorEmail).Fill(s.email); err != nil {
		return fmt.Errorf("failed to fill email: %w", err)
	}

	// Fill password
	if err := s.page.Locator(selectorPassword).Fill(s.password); err != nil {
		return fmt.Errorf("failed to fill password: %w", err)
	}

	// Click submit
	if err := s.page.Locator(selectorSubmit).Click(); err != nil {
		return fmt.Errorf("failed to click submit: %w", err)
	}

	// Wait for redirect indicating successful login
	navTimeout := float64(navigationTimeout)
	if err := s.page.WaitForURL("**"+authSuccessPattern+"**", playwright.PageWaitForURLOptions{
		Timeout: &navTimeout,
	}); err != nil {
		// Try dashboard pattern as fallback
		if err2 := s.page.WaitForURL("**"+dashboardPattern+"**", playwright.PageWaitForURLOptions{
			Timeout: &navTimeout,
		}); err2 != nil {
			return fmt.Errorf("login redirect not detected: %w", err)
		}
	}

	return nil
}

// IsAuthenticated checks if the current session has valid authentication.
func (s *PlaywrightScraper) IsAuthenticated(ctx context.Context) (bool, error) {
	if s.page == nil {
		return false, fmt.Errorf("%w: page not initialized", domain.ErrNotAuthenticated)
	}

	cookies, err := s.context.Cookies()
	if err != nil {
		return false, fmt.Errorf("failed to get cookies: %w", err)
	}

	for _, c := range cookies {
		if c.Name == "hglogin" && c.Value != "" {
			return true, nil
		}
	}

	return false, nil
}

// AnalyzeSection scrapes rejection data for a given section.
// This is a placeholder that will be fully implemented when section selectors are mapped.
func (s *PlaywrightScraper) AnalyzeSection(ctx context.Context, secao string) (*domain.JobResult, error) {
	start := time.Now()

	if s.page == nil {
		return nil, fmt.Errorf("%w: page not initialized", domain.ErrNotAuthenticated)
	}

	authenticated, err := s.IsAuthenticated(ctx)
	if err != nil {
		return nil, err
	}
	if !authenticated {
		return nil, domain.ErrNotAuthenticated
	}

	result := &domain.JobResult{
		Secao:     secao,
		Total:     0,
		Rejeicoes: []domain.Rejeicao{},
		Duration:  time.Since(start),
	}

	s.logger.Info("section analysis complete", "section", secao, "duration", result.Duration)
	return result, nil
}

// Close releases all browser resources.
func (s *PlaywrightScraper) Close() error {
	var errs []string

	if s.page != nil {
		if err := s.page.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("page close: %v", err))
		}
	}

	if s.context != nil {
		if err := s.context.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("context close: %v", err))
		}
	}

	if s.browser != nil {
		if err := s.browser.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("browser close: %v", err))
		}
	}

	if s.pw != nil {
		if err := s.pw.Stop(); err != nil {
			errs = append(errs, fmt.Sprintf("playwright stop: %v", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors during close: %s", strings.Join(errs, "; "))
	}

	s.logger.Info("playwright resources released")
	return nil
}

// extractAndSaveCookies extracts all cookies from the browser context and persists them.
func (s *PlaywrightScraper) extractAndSaveCookies() error {
	pwCookies, err := s.context.Cookies()
	if err != nil {
		return fmt.Errorf("failed to extract cookies: %w", err)
	}

	domainCookies := playwrightCookiesToDomain(pwCookies)

	s.logger.Info("cookies extracted", "count", len(domainCookies))
	for _, c := range domainCookies {
		s.logger.Debug("cookie", "name", c.Name, "domain", c.Domain)
	}

	return s.storage.SaveCookies(domainCookies)
}

// playwrightCookiesToDomain converts Playwright cookies to domain cookies.
func playwrightCookiesToDomain(pwCookies []playwright.Cookie) []domain.Cookie {
	cookies := make([]domain.Cookie, 0, len(pwCookies))
	for _, c := range pwCookies {
		cookies = append(cookies, domain.Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Expires:  time.Unix(int64(c.Expires), 0),
			Secure:   c.Secure,
			HttpOnly: c.HttpOnly,
		})
	}
	return cookies
}

// domainCookiesToPlaywright converts domain cookies to Playwright's OptionalCookie format.
func domainCookiesToPlaywright(cookies []domain.Cookie) []playwright.OptionalCookie {
	pwCookies := make([]playwright.OptionalCookie, 0, len(cookies))
	for _, c := range cookies {
		expires := float64(c.Expires.Unix())
		pwCookies = append(pwCookies, playwright.OptionalCookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   &c.Domain,
			Path:     &c.Path,
			Expires:  &expires,
			Secure:   &c.Secure,
			HttpOnly: &c.HttpOnly,
		})
	}
	return pwCookies
}
