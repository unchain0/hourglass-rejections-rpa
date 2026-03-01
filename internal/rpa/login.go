package rpa

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"

	"hourglass-rejeicoes-rpa/internal/domain"
)

// HourglassBaseURL is the base URL for the Hourglass application
const HourglassBaseURL = "https://app.hourglass-app.com/v2/page/app"

// LoginManager handles authentication and cookie persistence for Hourglass.
type LoginManager struct {
	browser  *Browser
	storage  domain.Storage
	email    string
	password string
}

// NewLoginManager creates a new LoginManager instance.
func NewLoginManager(browser *Browser, storage domain.Storage) *LoginManager {
	return &LoginManager{
		browser:  browser,
		storage:  storage,
		email:    os.Getenv("HOURGLASS_EMAIL"),
		password: os.Getenv("HOURGLASS_PASSWORD"),
	}
}

// SetupMode runs the browser in non-headless mode for manual login.
func (lm *LoginManager) SetupMode(ctx context.Context) error {
	return lm.browser.Setup()
}

// PerformLogin performs automatic login with email/password or Google OAuth.
func (lm *LoginManager) PerformLogin(ctx context.Context) error {
	// Check if we should use Google OAuth
	if os.Getenv("HOURGLASS_USE_GOOGLE") == "true" {
		return lm.performGoogleLogin(ctx)
	}
	
	// Use email/password login
	return lm.performEmailLogin(ctx)
}

// performEmailLogin logs in using email and password fields.
func (lm *LoginManager) performEmailLogin(ctx context.Context) error {
	if lm.email == "" || lm.password == "" {
		return fmt.Errorf("HOURGLASS_EMAIL and HOURGLASS_PASSWORD must be set for automatic login")
	}

	err := chromedp.Run(lm.browser.Context(),
		chromedp.Navigate(HourglassBaseURL),
		chromedp.WaitVisible("#email", chromedp.ByID),
		chromedp.SendKeys("#email", lm.email, chromedp.ByID),
		chromedp.SendKeys("#password", lm.password, chromedp.ByID),
		chromedp.Click("button[type='submit']", chromedp.ByQuery),
		chromedp.Sleep(3*time.Second), // Wait for login to complete
	)
	if err != nil {
		return fmt.Errorf("failed to perform email login: %w", err)
	}

	return nil
}

// performGoogleLogin initiates Google OAuth login.
func (lm *LoginManager) performGoogleLogin(ctx context.Context) error {
	// Click on Google Sign-In button
	err := chromedp.Run(lm.browser.Context(),
		chromedp.Navigate(HourglassBaseURL),
		chromedp.WaitVisible(".gsi-material-button", chromedp.ByQuery),
		chromedp.Click(".gsi-material-button", chromedp.ByQuery),
	)
	if err != nil {
		return fmt.Errorf("failed to initiate Google login: %w", err)
	}

	return nil
}

// WaitForManualLogin waits for user to complete login manually (for setup mode).
func (lm *LoginManager) WaitForManualLogin(ctx context.Context) error {
	fmt.Println("\n📝 Complete o login manualmente no navegador...")
	fmt.Println("   - Use email/senha, Google ou Apple")
	fmt.Println("   - Após fazer login, pressione ENTER aqui para salvar os cookies")
	fmt.Print("\n➤ Pressione ENTER quando o login estiver completo...")
	
	fmt.Scanln()
	
	return nil
}

// SaveCookies extracts cookies from the browser and persists them.
func (lm *LoginManager) SaveCookies(ctx context.Context) error {
	var cookies []*network.Cookie

	err := chromedp.Run(lm.browser.Context(),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			cookies, err = network.GetCookies().Do(ctx)
			return err
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to get cookies: %w", err)
	}

	// Convert network.Cookie to domain.Cookie
	domainCookies := make([]domain.Cookie, len(cookies))
	for i, c := range cookies {
		domainCookies[i] = domain.Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Expires:  time.Unix(int64(c.Expires), 0),
			Secure:   c.Secure,
			HttpOnly: c.HTTPOnly,
		}
	}

	if err := lm.storage.SaveCookies(domainCookies); err != nil {
		return fmt.Errorf("failed to save cookies: %w", err)
	}

	fmt.Printf("✅ %d cookies salvos com sucesso!\n", len(domainCookies))
	return nil
}

// LoadCookies loads cookies from storage and sets them in the browser.
func (lm *LoginManager) LoadCookies(ctx context.Context) error {
	cookies, err := lm.storage.LoadCookies()
	if err != nil {
		return fmt.Errorf("failed to load cookies: %w", err)
	}

	if len(cookies) == 0 {
		return nil
	}

	// Navigate to domain first (required before setting cookies)
	err = chromedp.Run(lm.browser.Context(),
		chromedp.Navigate(HourglassBaseURL),
	)
	if err != nil {
		return fmt.Errorf("failed to navigate before setting cookies: %w", err)
	}

	// Convert domain.Cookie to network.CookieParam
	cookieParams := make([]*network.CookieParam, len(cookies))
	for i, c := range cookies {
		expires := cdp.TimeSinceEpoch(c.Expires)
		cookieParams[i] = &network.CookieParam{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Expires:  &expires,
			Secure:   c.Secure,
			HTTPOnly: c.HttpOnly,
		}
	}

	// Set cookies in browser
	err = chromedp.Run(lm.browser.Context(),
		network.SetCookies(cookieParams),
	)
	if err != nil {
		return fmt.Errorf("failed to set cookies: %w", err)
	}

	return nil
}

// IsAuthenticated checks if the current session is valid by looking for dashboard elements.
func (lm *LoginManager) IsAuthenticated(ctx context.Context) (bool, error) {
	var isLoggedIn bool

	err := chromedp.Run(lm.browser.Context(),
		chromedp.Navigate(HourglassBaseURL),
		chromedp.Sleep(2*time.Second),
		chromedp.Evaluate(`
			// Check if we're on the login page or already logged in
			const loginForm = document.querySelector('#email');
			const dashboard = document.querySelector('.dashboard, .app-container, nav, [data-testid="dashboard"]');
			!loginForm && !!dashboard
		`, &isLoggedIn),
	)
	if err != nil {
		return false, fmt.Errorf("failed to check authentication: %w", err)
	}

	return isLoggedIn, nil
}
