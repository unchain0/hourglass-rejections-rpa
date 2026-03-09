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

	isHeadless := IsHeadlessEnvironment()
	if isHeadless {
		slog.Info("detected headless environment, disabling browser authentication", "storage_path", storagePath)
	}

	tm := &TokenManager{
		authenticator:    authenticator,
		storagePath:      storagePath,
		tokensPath:       tokensPath,
		baseURL:          baseURL,
		renewalThreshold: 1 * time.Hour,
		stopChan:         make(chan struct{}),
	}

	if !isHeadless {
		tm.browserAuth = NewBrowserAuth(baseURL)
	}

	for _, opt := range opts {
		opt(tm)
	}

	slog.Info("token manager initialized", "tokens_path", tm.tokensPath, "storage_path", tm.storagePath, "headless", isHeadless, "has_browser_auth", tm.browserAuth != nil)

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

	if tokens != nil {
		timeUntilExpiry := time.Until(tokens.ExpiresAt)
		slog.Debug("checking token validity", "expires_at", tokens.ExpiresAt, "time_until_expiry", timeUntilExpiry, "threshold", tm.renewalThreshold)
	}

	if tokens == nil || tokens.IsNearExpiry(tm.renewalThreshold) {
		if tokens == nil {
			slog.Info("no tokens available, attempting authentication")
		} else {
			slog.Info("tokens near expiry or expired, renewing", "expires_at", tokens.ExpiresAt, "threshold", tm.renewalThreshold)
		}

		newTokens, err := tm.authenticateWithFallback()
		if err != nil {
			slog.Error("authentication failed", "error", err)
			if tm.onError != nil {
				tm.onError(err)
			}
			return nil, err
		}

		slog.Info("authentication successful, updating tokens", "expires_at", newTokens.ExpiresAt)
		tm.setTokens(newTokens)

		if err := tm.SaveTokens(newTokens); err != nil {
			slog.Error("CRITICAL: failed to persist authentication tokens", "path", tm.tokensPath, "error", err)
		} else {
			slog.Info("tokens persisted successfully", "path", tm.tokensPath)
		}

		if tm.onTokenRenewed != nil {
			slog.Info("calling onTokenRenewed callback")
			tm.onTokenRenewed(newTokens)
		}

		return newTokens, nil
	}

	slog.Debug("tokens still valid, no renewal needed", "expires_at", tokens.ExpiresAt)
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

	slog.Info("saving tokens to disk", "path", tm.tokensPath, "expires_at", tokens.ExpiresAt, "hglogin_present", tokens.HGLogin != "", "xsrf_present", tokens.XSRFToken != "")

	data, err := json.Marshal(tokens)
	if err != nil {
		return fmt.Errorf("failed to marshal tokens: %w", err)
	}

	dir := filepath.Dir(tm.tokensPath)
	slog.Debug("ensuring directory exists", "dir", dir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create tokens directory: %w", err)
	}

	tempPath := tm.tokensPath + ".tmp"
	slog.Debug("writing to temp file", "temp_path", tempPath)
	if err := os.WriteFile(tempPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write temp tokens file: %w", err)
	}

	slog.Debug("renaming temp file to final path", "from", tempPath, "to", tm.tokensPath)
	if err := os.Rename(tempPath, tm.tokensPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename tokens file: %w", err)
	}

	info, err := os.Stat(tm.tokensPath)
	if err != nil {
		slog.Warn("could not stat tokens file after save", "error", err)
	} else {
		slog.Info("tokens saved successfully", "path", tm.tokensPath, "size", info.Size(), "expires_at", tokens.ExpiresAt)
	}

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
	slog.Info("attempting authentication fallback", "has_browser_auth", tm.browserAuth != nil, "has_credentials", tm.HasWebAuthnCredentials())

	if tm.browserAuth != nil {
		slog.Info("trying browser authentication first")
		tokens, err := tm.browserAuth.Authenticate()
		if err == nil {
			slog.Info("browser authentication succeeded")
			return tokens, nil
		}
		slog.Error("browser authentication failed, falling back to WebAuthn", "error", err)
	} else {
		slog.Info("browser auth disabled (headless environment), using WebAuthn only")
	}

	if !tm.HasWebAuthnCredentials() {
		slog.Error("no WebAuthn credentials found", "storage_path", tm.storagePath)
		return nil, fmt.Errorf("no WebAuthn credentials available - run 'make setup-auth' to configure")
	}

	slog.Info("attempting WebAuthn authentication", "storage_path", tm.storagePath)
	tokens, err := tm.authenticator.Authenticate()
	if err != nil {
		slog.Error("WebAuthn authentication failed", "error", err)
		return nil, fmt.Errorf("browser auth fallback failed: %w", err)
	}

	slog.Info("WebAuthn authentication succeeded")
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
