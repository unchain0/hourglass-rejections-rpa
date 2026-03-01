// Package api provides a client for the Hourglass REST API.
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"time"
)

const (
	defaultBaseURL = "https://app.hourglass-app.com/api/v0.2"
)

// Client is an HTTP client for the Hourglass API.
type Client struct {
	httpClient *http.Client
	baseURL    string
	xsrfToken  string
	hgLogin    string
}

// NewClient creates a new Hourglass API client.
func NewClient() *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Jar:     jar,
		},
		baseURL: defaultBaseURL,
	}
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

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setHeaders(req)
	c.setCookies(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

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

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setHeaders(req)
	c.setCookies(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

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

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setHeaders(req)
	c.setCookies(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var meetings []Meeting
	if err := json.NewDecoder(resp.Body).Decode(&meetings); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return meetings, nil
}

// GetNotifications retrieves notifications for a date range and type.
// Endpoint: GET /api/v0.2/scheduling/notifications/{start}_{end}/{type}
func (c *Client) GetNotifications(start, end, notificationType string) ([]Notification, error) {
	url := fmt.Sprintf("%s/scheduling/notifications/%s_%s/%s", c.baseURL, start, end, notificationType)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setHeaders(req)
	c.setCookies(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var notifications []Notification
	if err := json.NewDecoder(resp.Body).Decode(&notifications); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return notifications, nil
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
