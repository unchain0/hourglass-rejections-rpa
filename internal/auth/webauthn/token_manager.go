package webauthn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TokenManager handles automatic token renewal using WebAuthn.
type TokenManager struct {
	authenticator *Authenticator
	browserAuth   *BrowserAuth
	storagePath   string
	tokensPath    string
	baseURL       string

	currentTokens *AuthTokens
	mu            sync.RWMutex

	// Callbacks
	onTokenRenewed func(tokens *AuthTokens)
	onError        func(err error)

	// Configuration
	renewalThreshold time.Duration
	stopChan         chan struct{}
}

// TokenManagerOption configures the TokenManager.
type TokenManagerOption func(*TokenManager)

// WithOnTokenRenewed sets a callback for when tokens are renewed.
func WithOnTokenRenewed(callback func(tokens *AuthTokens)) TokenManagerOption {
	return func(tm *TokenManager) {
		tm.onTokenRenewed = callback
	}
}

// WithOnError sets a callback for errors.
func WithOnError(callback func(err error)) TokenManagerOption {
	return func(tm *TokenManager) {
		tm.onError = callback
	}
}

// WithRenewalThreshold sets how close to expiry before renewing (default: 1 hour).
func WithRenewalThreshold(threshold time.Duration) TokenManagerOption {
	return func(tm *TokenManager) {
		tm.renewalThreshold = threshold
	}
}

func WithBrowserAuth(browserAuth *BrowserAuth) TokenManagerOption {
	return func(tm *TokenManager) {
		tm.browserAuth = browserAuth
	}
}

// WithTokensPath sets the file path used to persist auth tokens.
func WithTokensPath(path string) TokenManagerOption {
	return func(tm *TokenManager) {
		if path != "" {
			tm.tokensPath = path
		}
	}
}

// NewTokenManager creates a new token manager for automatic renewal.
func NewTokenManager(storagePath, baseURL string, opts ...TokenManagerOption) (*TokenManager, error) {
	if storagePath == "" {
		storagePath = os.Getenv("WEBAUTHN_CREDENTIALS_PATH")
		if storagePath == "" {
			homeDir, err := os.UserHomeDir()
			if err == nil {
				storagePath = filepath.Join(homeDir, ".hourglass-rpa", "webauthn-credentials.json")
			}
		}
	}

	tokensPath := os.Getenv("WEBAUTHN_TOKENS_PATH")
	if tokensPath == "" {
		tokensPath = filepath.Join(filepath.Dir(storagePath), "auth-tokens.json")
	}

	authenticator, err := NewAuthenticator(storagePath, baseURL)
	if err != nil {
		return nil, err
	}

	tm := &TokenManager{
		authenticator:    authenticator,
		browserAuth:      NewBrowserAuth(baseURL),
		storagePath:      storagePath,
		tokensPath:       tokensPath,
		baseURL:          baseURL,
		renewalThreshold: 1 * time.Hour,
		stopChan:         make(chan struct{}),
	}

	for _, opt := range opts {
		opt(tm)
	}

	return tm, nil
}

// Start begins the automatic token renewal loop.
func (tm *TokenManager) Start(ctx context.Context) error {
	loadedTokens, err := tm.LoadTokens()
	if err != nil {
		return fmt.Errorf("failed to load persisted tokens: %w", err)
	}

	if loadedTokens != nil {
		slog.Info("loaded persisted authentication tokens", "path", tm.tokensPath, "expires_at", loadedTokens.ExpiresAt)
		tm.setTokens(loadedTokens)
	}

	tokens, err := tm.EnsureValidTokens()
	if err != nil {
		return err
	}

	if loadedTokens != nil && !loadedTokens.IsNearExpiry(tm.renewalThreshold) && tm.onTokenRenewed != nil {
		tm.onTokenRenewed(tokens)
	}

	// Start renewal loop
	go tm.renewalLoop(ctx)

	return nil
}

// Stop stops the automatic renewal loop.
func (tm *TokenManager) Stop() {
	close(tm.stopChan)
}

// GetTokens returns the current authentication tokens.
func (tm *TokenManager) GetTokens() *AuthTokens {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if tm.currentTokens == nil {
		return nil
	}

	// Return copy
	return &AuthTokens{
		HGLogin:   tm.currentTokens.HGLogin,
		XSRFToken: tm.currentTokens.XSRFToken,
		ExpiresAt: tm.currentTokens.ExpiresAt,
	}
}

// IsAuthenticated returns true if we have valid tokens.
func (tm *TokenManager) IsAuthenticated() bool {
	tokens := tm.GetTokens()
	return tokens != nil && !tokens.IsExpired()
}

// EnsureValidTokens ensures tokens are valid, renewing if necessary.
func (tm *TokenManager) EnsureValidTokens() (*AuthTokens, error) {
	tokens := tm.GetTokens()

	if tokens == nil || tokens.IsNearExpiry(tm.renewalThreshold) {
		slog.Info("renewing authentication tokens")
		newTokens, err := tm.authenticateWithFallback()
		if err != nil {
			if tm.onError != nil {
				tm.onError(err)
			}
			return nil, err
		}

		tm.setTokens(newTokens)
		if err := tm.SaveTokens(newTokens); err != nil {
			slog.Warn("failed to persist authentication tokens", "path", tm.tokensPath, "error", err)
		}

		if tm.onTokenRenewed != nil {
			tm.onTokenRenewed(newTokens)
		}

		return newTokens, nil
	}

	return tokens, nil
}

// SaveTokens persists authentication tokens to disk with owner-only permissions.
func (tm *TokenManager) SaveTokens(tokens *AuthTokens) error {
	if tokens == nil {
		return errors.New("tokens cannot be nil")
	}

	if tm.tokensPath == "" {
		return errors.New("tokens path is not configured")
	}

	data, err := json.Marshal(tokens)
	if err != nil {
		return fmt.Errorf("failed to marshal tokens: %w", err)
	}

	dir := filepath.Dir(tm.tokensPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create tokens directory: %w", err)
	}

	if err := os.WriteFile(tm.tokensPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write tokens file: %w", err)
	}

	slog.Info("persisted authentication tokens", "path", tm.tokensPath, "expires_at", tokens.ExpiresAt)

	return nil
}

// LoadTokens loads persisted authentication tokens from disk.
func (tm *TokenManager) LoadTokens() (*AuthTokens, error) {
	if tm.tokensPath == "" {
		return nil, errors.New("tokens path is not configured")
	}

	data, err := os.ReadFile(tm.tokensPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			slog.Info("persisted tokens file not found", "path", tm.tokensPath)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read tokens file: %w", err)
	}

	var tokens AuthTokens
	if err := json.Unmarshal(data, &tokens); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tokens file: %w", err)
	}

	return &tokens, nil
}

func (tm *TokenManager) authenticateWithFallback() (*AuthTokens, error) {
	if tm.browserAuth != nil {
		slog.Info("using browser authentication")
		tokens, err := tm.browserAuth.Authenticate()
		if err == nil {
			return tokens, nil
		}
		slog.Error("browser authentication failed", "error", err)
	}

	tokens, err := tm.authenticator.Authenticate()
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	return tokens, nil
}

func (tm *TokenManager) setTokens(tokens *AuthTokens) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.currentTokens = tokens
}

func (tm *TokenManager) renewalLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tm.stopChan:
			return
		case <-ticker.C:
			_, err := tm.EnsureValidTokens()
			if err != nil {
				slog.Error("failed to renew tokens", "error", err)
			}
		}
	}
}

// GetHGLogin returns the current hglogin cookie value.
func (tm *TokenManager) GetHGLogin() string {
	tokens := tm.GetTokens()
	if tokens == nil {
		return ""
	}
	return tokens.HGLogin
}

// GetXSRFToken returns the current XSRF token.
func (tm *TokenManager) GetXSRFToken() string {
	tokens := tm.GetTokens()
	if tokens == nil {
		return ""
	}
	return tokens.XSRFToken
}
