# Learnings: Adding Comprehensive Tests to resend.go

## Testing HTTP Clients with Hardcoded URLs

When testing HTTP client code with hardcoded API URLs (like `resendAPIURL` constant), simply replacing the client's transport isn't enough because the URL is still pointing to the real API.

**Solution: Custom Transport with URL Rewriting**

Create a custom `http.RoundTripper` that intercepts requests and rewrites the URL to point to the test server:

```go
type mockTransport struct {
    targetURL string
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
    newURL, err := url.Parse(m.targetURL)
    if err != nil {
        return nil, err
    }
    req.URL.Scheme = newURL.Scheme
    req.URL.Host = newURL.Host
    return http.DefaultTransport.RoundTrip(req)
}
```

This allows testing HTTP client code without modifying the source code to make the URL configurable.

## Helper Function for Test Setup

Avoid repeating test setup code by creating a helper function:

```go
func setupTestServer(handler http.HandlerFunc) (*httptest.Server, *ResendNotifier) {
    server := httptest.NewServer(handler)
    transport := &mockTransport{targetURL: server.URL}
    client := &http.Client{
        Transport: transport,
        Timeout:  30 * time.Second,
    }
    n := &ResendNotifier{
        apiKey: "test-api-key",
        from:   "from@example.com",
        to:     "to@example.com",
        client:  client,
    }
    return server, n
}
```

## Testing Status Code Variations

Test multiple HTTP status codes to ensure proper error handling:
- Success codes: 200 OK, 201 Created
- Client errors: 400 Bad Request, 401 Unauthorized
- Server errors: 500 Internal Server Error
- Network errors: Use custom failing transport

## Coverage Limitations

Some error paths in defensive code are difficult or impossible to trigger in tests:
- `json.Marshal` with valid data always succeeds
- `http.NewRequestWithContext` with valid parameters always succeeds

These are defensive error handling paths that are unlikely to occur in production. Focus test coverage on the realistic code paths.
