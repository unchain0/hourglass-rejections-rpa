# Work Plan: Automatic Token Renewal for Hourglass RPA

**Created:** 2026-03-03  
**Priority:** High  
**Status:** In Progress  

## Objective
Implement automatic token renewal for the Hourglass RPA project using Go only, ensuring the system can automatically authenticate and renew XSRF tokens and hglogin cookies without manual intervention, working headlessly on a VPS.

## Background

### Current State
- **Phase 1 (WebAuthn Native):** FAILED - Server rejects software-based credentials
- **Phase 2 (Browser Automation):** IN PROGRESS - chromedp build errors need fixing

### Files Being Modified
- `internal/auth/webauthn/browser_auth.go` - Build errors need fixing
- `internal/auth/webauthn/token_manager.go` - Needs BrowserAuth integration
- `internal/auth/webauthn/authentication.go` - Native implementation (complete but not working)

### Constraints
- **Go only** - No other programming languages
- **VPS compatible** - Must work without GUI (headless Chrome)
- **Automatic renewal** - No manual intervention required

---

## Task Breakdown

### Phase 1: Fix chromedp Build Errors

- [ ] **Task 1.1:** Fix incorrect chromedp API usage in `browser_auth.go`
  - Replace `chromedp.NetworkCookie` with `network.Cookie`
  - Replace `chromedp.Cookies()` with `storage.GetCookies()`
  - Add correct imports: `github.com/chromedp/cdproto/network` and `github.com/chromedp/cdproto/storage`
  - **Expected Outcome:** File compiles without errors
  - **Verification:** `go build ./...` passes

- [ ] **Task 1.2:** Complete browser automation logic
  - Navigate to login page (`/v2/page/app`)
  - Handle WebAuthn authentication flow
  - Wait for successful login and page redirect
  - Extract cookies after authentication
  - **Expected Outcome:** BrowserAuth.Authenticate() returns valid tokens
  - **Verification:** Manual test with real credentials

### Phase 2: Integrate with TokenManager

- [ ] **Task 2.1:** Update TokenManager to use BrowserAuth as fallback
  - Add BrowserAuth field to TokenManager struct
  - Implement fallback logic: Try API auth first, then BrowserAuth
  - Add retry mechanism with exponential backoff
  - **Expected Outcome:** TokenManager seamlessly uses BrowserAuth when native auth fails
  - **Verification:** Unit tests pass, integration test works

- [ ] **Task 2.2:** Add token persistence and caching
  - Store tokens in file with 0600 permissions
  - Load cached tokens on startup
  - Refresh tokens before expiry (5-minute buffer)
  - **Expected Outcome:** Tokens persist across restarts
  - **Verification:** Tokens saved/loaded correctly

### Phase 3: VPS Deployment Preparation

- [ ] **Task 3.1:** Configure headless Chrome for VPS
  - Add Chrome/Chromium installation to Dockerfile
  - Configure chromedp with headless flags (already partially done)
  - Handle Chrome path detection for different environments
  - **Expected Outcome:** Works on VPS without GUI
  - **Verification:** Test in Docker container

- [ ] **Task 3.2:** Add error handling and monitoring
  - Implement comprehensive logging for auth flow
  - Add Sentry integration for authentication errors
  - Create health check endpoint for auth status
  - **Expected Outcome:** Full observability of auth process
  - **Verification:** Logs show auth flow, Sentry captures errors

### Phase 4: Testing & Validation

- [ ] **Task 4.1:** Unit tests for BrowserAuth
  - Mock chromedp for testing
  - Test cookie extraction logic
  - Test error scenarios
  - **Expected Outcome:** 80%+ test coverage for BrowserAuth
  - **Verification:** `go test -cover ./...` passes

- [ ] **Task 4.2:** Integration test
  - End-to-end test with real Hourglass login
  - Test token refresh flow
  - Test fallback mechanism
  - **Expected Outcome:** Full auth flow works end-to-end
  - **Verification:** Manual test on staging environment

- [ ] **Task 4.3:** VPS deployment test
  - Deploy to VPS with Docker
  - Verify headless Chrome works
  - Test automatic token renewal
  - **Expected Outcome:** Production-ready deployment
  - **Verification:** System runs 24/7 without manual intervention

---

## Parallelization Strategy

### Parallel Tasks (Group 1)
- Task 1.1: Fix chromedp API usage
- Task 3.1: Configure headless Chrome for VPS

### Parallel Tasks (Group 2 - depends on Group 1)
- Task 1.2: Complete browser automation logic
- Task 2.1: Update TokenManager

### Sequential Tasks (depends on Group 2)
- Task 2.2: Add token persistence
- Task 3.2: Add error handling
- Task 4.1: Unit tests
- Task 4.2: Integration test
- Task 4.3: VPS deployment test

---

## Key Code Patterns to Follow

### 1. chromedp Cookie Extraction
```go
import (
    "github.com/chromedp/cdproto/network"
    "github.com/chromedp/cdproto/storage"
    "github.com/chromedp/chromedp"
)

// Correct way to get cookies
var cookies []*network.Cookie
cookies, err = storage.GetCookies().Do(ctx)
```

### 2. Error Wrapping (existing convention)
```go
return fmt.Errorf("browser automation failed: %w", err)
```

### 3. File Permissions (existing convention)
```go
os.WriteFile(path, data, 0600) // Owner-only permissions
```

### 4. Sentry Integration (existing convention)
```go
if s.sentryClient != nil {
    s.sentryClient.CaptureError(err, map[string]interface{}{
        "section": "auth",
        "phase": "browser",
    })
}
```

---

## Acceptance Criteria

1. **Build Success**: `go build ./...` completes without errors
2. **Test Coverage**: Overall test coverage remains above 80%
3. **VPS Compatible**: Runs without GUI (headless Chrome)
4. **Automatic Auth**: No manual token extraction needed
5. **Token Persistence**: Tokens survive restarts
6. **Error Handling**: All errors logged and reported to Sentry
7. **Documentation**: Code comments explain auth flow

---

## Risk Mitigation

| Risk | Mitigation |
|------|-----------|
| chromedp API changes | Pin to specific version in go.mod |
| Chrome not installed on VPS | Add Chrome to Dockerfile |
| WebAuthn still fails | Implement retry with screenshots for debugging |
| Token expiry edge cases | 5-minute buffer before expiry |
| Rate limiting | Add delays between retry attempts |

---

## Notes

- Reference: [chromedp cookie example](https://github.com/chromedp/examples/blob/master/cookie/main.go)
- Chrome flags already configured: headless, disable-gpu, no-sandbox, disable-dev-shm-usage
- Hourglass login URL: `https://app.hourglass-app.com/v2/page/app`
- Target cookies: `hglogin` and `X-Hourglass-XSRF-Token`
