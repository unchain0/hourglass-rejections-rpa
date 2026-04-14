# AGENTS.md - WebAuthn Authentication

**Directory**: `internal/auth/webauthn/`  
**Purpose**: WebAuthn authentication with browser automation fallback  
**Complexity**: High  
**Lines**: ~3,000

## 🎯 Overview

Sistema de autenticação WebAuthn com fallback para browser automation. Gerencia credenciais, tokens e autenticação automática na VPS.

## 📁 Structure

```
internal/auth/webauthn/
├── authenticator.go          # Core WebAuthn authentication
├── authentication.go         # Authentication flow
├── browser_auth.go           # ChromeDP browser automation
├── environment.go            # VPS/headless detection
├── token_manager.go          # Token lifecycle management
├── storage.go (types.go)     # Credential storage
├── types.go                  # Data structures
└── priority_coverage_test.go # Complex test scenarios
```

## 🔍 Key Components

### TokenManager
Central orchestrator for authentication:
- Checks token expiration
- Attempts browser auth first (if available)
- Falls back to WebAuthn credentials
- Handles token renewal

### Authenticator
WebAuthn protocol implementation:
- Credential generation
- Attestation handling
- Assertion creation
- Token extraction from cookies

### BrowserAuth
ChromeDP-based browser automation:
- Opens Chrome/Chromium
- Navigates to login page
- Extracts tokens from cookies
- Headless mode support

## 🏗️ Authentication Flow

```
authenticateWithFallback()
├── Try browser auth (if not headless)
│   └── ChromeDP automation
├── Check WebAuthn credentials
├── Try WebAuthn authentication
│   ├── Load stored credential
│   ├── Begin authentication
│   ├── Create assertion
│   └── Finish authentication
└── Return tokens or error
```

## ⚙️ Environment Detection

VPS/headless environments are detected via:
- `CI` environment variable
- `GITHUB_ACTIONS` environment variable
- `TEST_TIMEOUT_SHORT` environment variable

## 🧪 Testing Notes

- Tests require Chrome/Chromium
- Set `CHROME_BIN` environment variable
- CI uses shorter timeouts (1s vs 2min)
- Excluded from pre-commit hooks (slow)

## 🔐 Security

- Credentials stored with 0600 permissions
- Separate token and credential files
- Atomic token writes (temp + rename)
- No credentials in logs

## 🚨 Common Issues

1. **"chrome not found"**: Set `CHROME_BIN` or install Chrome
2. **Timeout on VPS**: Browser auth disabled in headless mode
3. **"no credentials stored"**: Run `make setup-auth` first

## 📚 See Also

- Root `AGENTS.md` for project overview
- `cmd/setup-auth/` for authentication setup
- `cmd/save-tokens/` for token extraction
