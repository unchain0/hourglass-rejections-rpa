// Package api provides a client for the Hourglass REST API.
package hourglass

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"hourglass-rejections-rpa/src/integrations/auth/webauthn"
)

var newHTTPRequest = http.NewRequest

const (
	defaultSiteURL = "https://app.hourglass-app.com"
	defaultBaseURL = defaultSiteURL + "/api/v0.2"
)

// Client is an HTTP client for the Hourglass API.
type Client struct {
	httpClient         *http.Client
	baseURL            string
	xsrfToken          string
	hgLogin            string
	tokenManager       authTokenManager
	webAuthnTokensPath string
	browserProfileDir  string
	useWebAuthn        bool
}

type authTokenManager interface {
	Start(ctx context.Context) error
	Stop()
	EnsureValidTokens() (*webauthn.AuthTokens, error)
	ForceRenewal() (*webauthn.AuthTokens, error)
}

// NewClient creates a new Hourglass API client.
func NewClient() *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Jar:       jar,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
		baseURL:     defaultBaseURL,
		useWebAuthn: false,
	}
}

func normalizeAPIBaseURL(baseURL string) string {
	trimmed := strings.TrimRight(baseURL, "/")
	if trimmed == "" {
		return defaultBaseURL
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return trimmed
	}

	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		parsed.Path = "/api/v0.2"
		parsed.RawPath = ""
		return strings.TrimRight(parsed.String(), "/")
	}

	return trimmed
}

// SetBaseURL sets the base URL for API requests.
func (c *Client) SetBaseURL(baseURL string) {
	c.baseURL = normalizeAPIBaseURL(baseURL)
}

// SetWebAuthnTokensPath sets the path for storing WebAuthn tokens.
func (c *Client) SetWebAuthnTokensPath(tokensPath string) {
	c.webAuthnTokensPath = tokensPath
}

func (c *Client) SetBrowserProfileDir(profileDir string) {
	c.browserProfileDir = profileDir
}

// LoadTokensFromFile loads authentication tokens from a JSON file.
func (c *Client) LoadTokensFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read tokens file: %w", err)
	}

	var tokens webauthn.AuthTokens
	if err := json.Unmarshal(data, &tokens); err != nil {
		return fmt.Errorf("failed to parse tokens: %w", err)
	}

	if tokens.IsExpired() {
		return fmt.Errorf("tokens have expired")
	}

	c.hgLogin = tokens.HGLogin
	c.xsrfToken = tokens.XSRFToken
	return nil
}

// NewClientWithWebAuthn creates a new Hourglass API client with WebAuthn authentication.
func NewClientWithWebAuthn(credentialsPath string, capture func(error, map[string]interface{})) (*Client, error) {
	client := NewClient()

	err := client.EnableWebAuthn(credentialsPath, capture)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// EnableWebAuthn enables WebAuthn authentication with the given credentials path.
func (c *Client) EnableWebAuthn(credentialsPath string, capture func(error, map[string]interface{})) error {
	tokenManager, err := c.newTokenManager(credentialsPath, capture)
	if err != nil {
		return fmt.Errorf("failed to create token manager: %w", err)
	}

	c.tokenManager = tokenManager
	c.useWebAuthn = true
	return nil
}

func (c *Client) newTokenManager(credentialsPath string, capture func(error, map[string]interface{})) (authTokenManager, error) {
	opts := []webauthn.TokenManagerOption{
		webauthn.WithOnTokenRenewed(func(tokens *webauthn.AuthTokens) {
			c.UpdateTokensFromManager(tokens)
		}),
		webauthn.WithOnError(func(err error) {
			if capture != nil {
				capture(err, map[string]interface{}{
					"component": "token_manager",
					"action":    "token_renewal",
				})
			}
		}),
	}

	if c.webAuthnTokensPath != "" {
		opts = append(opts, webauthn.WithTokensPath(c.webAuthnTokensPath))
	}

	if c.browserProfileDir != "" {
		opts = append(opts, webauthn.WithBrowserProfileDir(c.browserProfileDir))
	}

	return webauthn.NewTokenManager(credentialsPath, c.baseURL, opts...)
}

// StartTokenManager starts the token manager for automatic token renewal.
func (c *Client) StartTokenManager(ctx context.Context) error {
	if c.tokenManager == nil {
		return nil
	}
	return c.tokenManager.Start(ctx)
}

// StopTokenManager stops the token manager.
func (c *Client) StopTokenManager() {
	if c.tokenManager != nil {
		c.tokenManager.Stop()
	}
}

// ensureAuth ensures valid authentication tokens are available.
func (c *Client) ensureAuth() error {
	if !c.useWebAuthn || c.tokenManager == nil {
		return nil
	}

	tokens, err := c.tokenManager.EnsureValidTokens()
	if err != nil {
		return fmt.Errorf("failed to ensure authentication: %w", err)
	}

	c.updateTokens(tokens)
	return nil
}

func (c *Client) updateTokens(tokens *webauthn.AuthTokens) {
	c.hgLogin = tokens.HGLogin
	c.xsrfToken = tokens.XSRFToken
}

// UpdateTokensFromManager updates the client's tokens from the token manager.
func (c *Client) UpdateTokensFromManager(tokens *webauthn.AuthTokens) {
	c.updateTokens(tokens)
}

// SetHGLogin sets the hglogin cookie for authenticated requests.
func (c *Client) SetHGLogin(cookie string) {
	c.hgLogin = cookie
}

// SetXSRFToken sets the XSRF token for authenticated requests.
func (c *Client) SetXSRFToken(token string) {
	c.xsrfToken = token
}

// GetUsers retrieves all users from the Hourglass system.
// Endpoint: GET /api/v0.2/fsreport/users
func (c *Client) GetUsers() ([]User, error) {
	url := fmt.Sprintf("%s/fsreport/users", c.baseURL)

	resp, err := c.doAuthenticatedGet(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var response UsersResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return response.Users, nil
}

// GetAVAttendants retrieves mechanical assignments for a date range.
// Endpoint: GET /api/v0.2/scheduling/av_attendant/{start}_{end}
func (c *Client) GetAVAttendants(start, end string) ([]AVAttendant, error) {
	url := fmt.Sprintf("%s/scheduling/av_attendant/%s_%s", c.baseURL, start, end)

	resp, err := c.doAuthenticatedGet(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var attendants []AVAttendant
	if err := json.NewDecoder(resp.Body).Decode(&attendants); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return attendants, nil
}

// GetMeetings retrieves meeting schedules for a date range.
// Endpoint: GET /api/v0.2/scheduling/mm/meeting/{start}_{end}?lgroup={lgroup}&no_subs=true
func (c *Client) GetMeetings(start, end string, lgroup int) ([]Meeting, error) {
	url := fmt.Sprintf("%s/scheduling/mm/meeting/%s_%s?lgroup=%d&no_subs=true", c.baseURL, start, end, lgroup)
	var meetings []Meeting
	if err := c.getJSON(url, &meetings); err != nil {
		return nil, err
	}
	return meetings, nil
}

// GetNotifications retrieves notifications for a date range and type.
// Endpoint: GET /api/v0.2/scheduling/notifications/{start}_{end}/{type}
func (c *Client) GetNotifications(start, end, notificationType string) ([]Notification, error) {
	url := fmt.Sprintf("%s/scheduling/notifications/%s_%s/%s", c.baseURL, start, end, notificationType)
	var notifications []Notification
	if err := c.getJSON(url, &notifications); err != nil {
		return nil, err
	}
	return notifications, nil
}

// getJSON performs a GET request and decodes the JSON response into the target.
func (c *Client) getJSON(url string, target interface{}) error {
	resp, err := c.doAuthenticatedGet(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

func (c *Client) doAuthenticatedGet(url string) (*http.Response, error) {
	if err := c.ensureAuth(); err != nil {
		return nil, err
	}

	req, err := newHTTPRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setHeaders(req)
	c.setCookies(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	if resp.StatusCode != http.StatusUnauthorized || !c.useWebAuthn || c.tokenManager == nil {
		return resp, nil
	}

	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if renewErr := c.forceRenewAuth(); renewErr != nil {
		return nil, fmt.Errorf("authentication rejected by Hourglass and forced renewal failed: %w (body: %s)", renewErr, string(body))
	}

	retryReq, err := newHTTPRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create retry request: %w", err)
	}

	c.setHeaders(retryReq)
	c.setCookies(retryReq)

	retryResp, err := c.httpClient.Do(retryReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request after forced token renewal: %w", err)
	}

	return retryResp, nil
}

// setHeaders sets the required headers for API requests.
func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if c.xsrfToken != "" {
		req.Header.Set("X-Hourglass-XSRF-Token", c.xsrfToken)
	}
}

// setCookies sets the authentication cookies for the request.
func (c *Client) setCookies(req *http.Request) {
	if c.hgLogin != "" {
		req.AddCookie(&http.Cookie{
			Name:  "hglogin",
			Value: c.hgLogin,
			Path:  "/",
		})
	}
}

func (c *Client) forceRenewAuth() error {
	if !c.useWebAuthn || c.tokenManager == nil {
		return errors.New("automatic token renewal is not enabled")
	}

	tokens, err := c.tokenManager.ForceRenewal()
	if err != nil {
		return err
	}

	c.updateTokens(tokens)
	return nil
}
