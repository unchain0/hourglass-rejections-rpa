# WebAuthn authentication

## OVERVIEW

- Package path: `src/integrations/auth/webauthn/`; do not use the former `internal/...` path.
- Owns native WebAuthn, Chrome/chromedp fallback, and persisted-token renewal.
- Auth cookies are `hglogin` and `X-Hourglass-XSRF-Token`; both are required.

## AUTH AND CONFIGURATION

- Credential path precedence: constructor argument, `WEBAUTHN_CREDENTIALS_PATH`, then
  `~/.hourglass-rpa/webauthn-credentials.json`.
- Token path precedence: `WithTokensPath`, `WEBAUTHN_TOKENS_PATH`, then `auth-tokens.json` beside
  the credential file. `NewTokenManager` also normalizes the base URL.
- Headless + stored credentials: native WebAuthn is attempted first; configured browser profile may
  be the fallback. Otherwise, browser auth is first when enabled, followed by native WebAuthn.
- `WithBrowserProfileDir` overrides `CHROME_PROFILE_DIR`; either keeps browser auth available in a
  headless environment. `BrowserAuth` defaults to headless; `WithHeadless`/`WithProfileDir` tune it.
- Browser auth retries transient failures up to three times. Profile extraction reads cookies without
  triggering the login control.
- `Start` validates loaded tokens before starting its five-minute renewal loop. Successful native
  authentication updates the credential sign count before saving.

## STORAGE AND SECURITY

- Credential, token, and Chrome-profile directories are `0700`; credential and token files are
  `0600`. Credentials write directly, while tokens use a `0600` temporary file plus atomic rename;
  remove the temporary file when rename fails.
- Cookies, private keys, browser profiles, and complete auth files are secret. Log only presence,
  paths, and expiry metadata, never secret values or full file contents.
- `CHROME_BIN` then `CHROME_PATH` override lookup; otherwise common system paths are probed.

## TESTING

- Run `go test ./src/integrations/auth/webauthn`; add `-race` for synchronization or renewal changes.
- Restore every injected package hook with `t.Cleanup`. Most browser tests mock chromedp, so the
  package suite does not generally require a real Chrome installation.
- Timing branches: CI/GitHub Actions use a 1s auth timeout; `TEST_TIMEOUT_SHORT=1` uses 5s;
  both shorten polling and retry delays.

## ANTI-PATTERNS

- Do not reorder browser/native fallback; it is environment- and credential-sensitive.
- Do not bypass atomic token writes, weaken permissions, or persist/log secrets elsewhere.
- Do not share a live Chrome profile, remove singleton locks without stale-PID checks, or suppress
  cleanup errors that protect profile integrity.
- Do not leave injected global hooks modified across tests.
