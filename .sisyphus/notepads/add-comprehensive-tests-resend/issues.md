# Issues: Adding Comprehensive Tests to resend.go

## Initial Test Failures: Real API Calls

**Issue:** Initial tests were failing because they were making real HTTP calls to the Resend API instead of the mock server.

**Root Cause:** Simply replacing `n.client = server.Client()` doesn't change the URL. The `sendEmail` function uses a constant `resendAPIURL = "https://api.resend.com/emails"` which cannot be overridden.

**Solution:** Created a custom `mockTransport` that intercepts requests and rewrites the URL scheme and host to point to the test server.

## Build Error: Unused Variable

**Issue:** `declared and not used: originalURL` when trying to temporarily override the API URL constant.

**Root Cause:** Attempted to save the constant value before modifying it, but constants cannot be modified in Go.

**Solution:** Removed the unused variable and instead used the custom transport approach.

## Coverage Limitation: sendEmail at 87.5%

**Issue:** `sendEmail` function is at 87.5% coverage instead of 100%.

**Root Cause:** Two error paths cannot be triggered in tests:
1. Line 107: JSON marshaling error - `json.Marshal` with `map[string]string` always succeeds
2. Line 112: Request creation error - `http.NewRequestWithContext` with valid parameters always succeeds

**Resolution:** These are defensive error handling paths. In production code with valid inputs, these errors will never occur. The 87.5% coverage is acceptable given the practical limitations.

## Overall Package Coverage at 44.3%

**Issue:** Coverage for `internal/notifier` package is 44.3% instead of 100%.

**Root Cause:** The package includes `telegram.go` which has 0% coverage. The task specifically requested adding tests to `resend_test.go` for `resend.go` functions.

**Clarification:** The task explicitly listed only `resend.go` functions to test:
- NewResendNotifier()
- SendJobCompletion()
- SendJobFailure()
- SendDailyReport()
- sendEmail()

All requested functions now have tests with excellent coverage:
- NewResendNotifier: 100%
- SendJobCompletion: 100%
- SendJobFailure: 100%
- SendDailyReport: 100%
- sendEmail: 87.5%
