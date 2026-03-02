# Decisions: Adding Comprehensive Tests to resend.go

## Use Custom Transport Instead of Modifying Source

**Decision:** Do not modify `resend.go` source code to make the API URL configurable.

**Rationale:**
- Task explicitly states "Do NOT modify source code, only add tests"
- Source code modification could introduce bugs
- Production code should remain as-is
- Custom transport approach is a common testing pattern

**Alternative Considered:** Refactor `resend.go` to inject the API URL as a dependency.

**Why Rejected:** Violates the constraint "Do NOT modify source code".

## Directly Test Private Function sendEmail

**Decision:** Test the private `sendEmail` function directly in addition to testing it indirectly through public methods.

**Rationale:**
- Provides more granular test control
- Can verify request headers and method
- Tests error paths that may not be easily triggered through public methods
- Clearer test intent

**Alternative Considered:** Only test `sendEmail` indirectly through `SendJobCompletion`, `SendJobFailure`, and `SendDailyReport`.

**Why Rejected:** Would limit ability to verify implementation details and error handling.

## Use httptest for Mock Server

**Decision:** Use Go's `net/http/httptest` package for mock HTTP server.

**Rationale:**
- Built-in Go standard library
- No external dependencies
- Fast and reliable
- Simulates real HTTP behavior

**Alternative Considered:** Use third-party HTTP mocking libraries.

**Why Rejected:** Unnecessary dependency, httptest is sufficient.

## Accept 87.5% Coverage for sendEmail

**Decision:** Accept 87.5% coverage for `sendEmail` instead of striving for 100%.

**Rationale:**
- The uncovered paths are defensive error handling that are practically unreachable
- `json.Marshal` with valid `map[string]string` always succeeds
- `http.NewRequestWithContext` with valid parameters always succeeds
- Attempting to trigger these errors would require complex, fragile test setups
- The test effort would not provide meaningful protection against real failures

**Alternative Considered:** Use reflection or other techniques to force error conditions.

**Why Rejected:** Would make tests complex, fragile, and hard to understand. Defensive error paths are acceptable to leave uncovered when they're realistically unreachable.

## Separate Test Scenarios Instead of Table-Driven Tests

**Decision:** Use individual test functions instead of table-driven tests.

**Rationale:**
- Clearer test names that describe the scenario
- Easier to debug failures (specific test name)
- More verbose but easier to understand
- Aligns with task requirements for specific scenarios

**Alternative Considered:** Use table-driven tests for status code variations.

**Why Rejected:** Individual functions provide better test names for the task's specific requirements.
