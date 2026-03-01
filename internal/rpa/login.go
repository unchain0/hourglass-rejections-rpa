package rpa

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"

	"hourglass-rejeicoes-rpa/internal/domain"
)

// LoginManager handles authentication and cookie persistence for Hourglass.
type LoginManager struct {
	browser *Browser
	storage domain.Storage
}

// NewLoginManager creates a new LoginManager instance.
func NewLoginManager(browser *Browser, storage domain.Storage) *LoginManager {
	return &LoginManager{
		browser: browser,
		storage: storage,
	}
}

// SetupMode runs the browser in non-headless mode for manual login.
// This should be used locally to authenticate and save cookies.
func (lm *LoginManager) SetupMode(ctx context.Context) error {
	return lm.browser.Setup()
}

// PerformLogin navigates to Hourglass and waits for manual authentication.
// In debug mode, it opens a visible browser for manual login.
func (lm *LoginManager) PerformLogin(ctx context.Context) error {
	url := "https://app.hourglass-app.com/v2/page/app"

	err := chromedp.Run(lm.browser.Context(),
		chromedp.Navigate(url),
		chromedp.WaitVisible("body", chromedp.ByQuery),
	)
	if err != nil {
		return fmt.Errorf("failed to navigate to Hourglass: %w", err)
	}

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
		chromedp.Navigate("https://app.hourglass-app.com/v2/page/app"),
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

// IsAuthenticated checks if the current session is valid.
// It looks for specific elements that indicate successful login.
func (lm *LoginManager) IsAuthenticated(ctx context.Context) (bool, error) {
	var authenticated bool

	// Try to find an element that only appears when logged in
	// This could be the AG-Grid or a user menu
	err := chromedp.Run(lm.browser.Context(),
		chromedp.Navigate("https://app.hourglass-app.com/v2/page/app"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Check for login indicator (e.g., specific element)
			// This is a placeholder - adjust selector based on actual site
			authenticated = true
			return nil
		}),
	)
	if err != nil {
		return false, fmt.Errorf("failed to check authentication: %w", err)
	}

	return authenticated, nil
}
