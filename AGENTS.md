# AGENTS.md - Hourglass Rejections RPA

**Project**: Hourglass Rejections RPA  
**Language**: Go 1.26  
**Type**: Automation/Bot  
**Lines of Code**: ~20,000  
**Test Coverage**: 98.1%

## 🎯 Project Overview

Automated system to monitor assignment rejections in Hourglass. Runs 2x daily and sends Telegram notifications for rejections in Mechanical Parts, Field Ministry, and Public Witnessing sections.

**Core Stack**:
- Go 1.26 with standard library patterns
- Telegram Bot API (go-telegram/bot)
- ChromeDP for browser automation
- WebAuthn for secure authentication
- SQLite for preferences storage
- Cron scheduling (robfig/cron)

## 📁 Structure

```
.
├── cmd/                    # Entry points (4 binaries)
│   ├── rpa/               # Main daemon
│   ├── save-tokens/       # Token extraction tool
│   ├── token-refresh/     # Token renewal utility
│   └── setup-auth/        # Authentication setup
├── internal/              # Private application code
│   ├── api/               # Hourglass API client
│   ├── auth/webauthn/     # WebAuthn authentication
│   ├── bot/               # Telegram bot runner
│   ├── config/            # Configuration management
│   ├── notifier/          # Telegram notifications
│   ├── scheduler/         # Cron scheduling
│   └── ...                # Other utilities
├── .github/workflows/     # CI/CD
└── docker-compose.yml     # Local orchestration
```

## 🔍 Where to Look

| Task | Location | Notes |
|------|----------|-------|
| Add new command | `cmd/<name>/main.go` | Follow existing 4 cmd patterns |
| API client changes | `internal/api/` | Client, analyzer, models |
| Auth/WebAuthn | `internal/auth/webauthn/` | Complex: browser + token management |
| Bot logic | `internal/bot/` | Runner, commands, handlers |
| Notifications | `internal/notifier/` | Telegram, resend (email) |
| Scheduling | `internal/scheduler/` | Cron, ticker logic |
| User preferences | `internal/preferences/` | SQLite-backed |
| i18n/locales | `internal/i18n/` | pt-BR, en-US |

## 🏗️ Architecture Patterns

### Entry Points (cmd/)
Each binary follows standard Go pattern:
- `main.go` with `main()` function
- Dependency injection via constructors
- Graceful shutdown with context cancellation

### Internal Packages
- **api**: REST client with retry logic, cookie management
- **auth/webauthn**: Complex browser automation + credential storage
- **bot**: Telegram bot with command handlers, inline keyboards
- **notifier**: Multi-channel notifications (Telegram primary, Resend email secondary)
- **scheduler**: Cron-based with smart business hours logic

### Key Interfaces
```go
// Notifier - for sending notifications
type Notifier interface {
    Notify(ctx context.Context, notification Notification) error
}

// Storage - for data persistence
type Storage interface {
    Save(data interface{}) error
    Load(dest interface{}) error
}
```

## ⚙️ Conventions

### Code Style
- Standard Go formatting (`gofmt`)
- Linting with `golangci-lint` (see `.golangci.yml`)
- Pre-commit hooks via `lefthook`
- Line length: no strict limit, but keep readable

### Testing
- **Framework**: testify (assert/require)
- **Coverage**: 95% threshold (pre-push), 70% (CI)
- **Patterns**: 
  - `TestMain` for global setup (i18n init)
  - Table-driven tests with `t.Run`
  - Mocks in `internal/testutil/mocks.go`
  - httptest for HTTP mocking
  - `t.TempDir()` for temp files
- **Exclusions**: `internal/auth/webauthn` from pre-commit (slow)

### Error Handling
```go
// Wrap errors with context
return fmt.Errorf("failed to create telegram bot: %w", err)

// Never ignore errors
if err != nil {
    return err
}
```

### Logging
- Use `charmbracelet/log` (structured)
- Levels: Debug, Info, Warn, Error
- Always include context fields

## 🚫 Anti-Patterns (Explicitly Avoided)

1. **No bare `panic()` in production code** - only in `TestMain` for init failures
2. **No ignored errors** - always handle or explicitly comment why ignored
3. **No hardcoded credentials** - use env vars or token files
4. **No direct `os.Exit()` outside `main()`** - use error propagation
5. **No global state** - prefer dependency injection
6. **Comment directives** - avoid "ALWAYS", "NEVER" in comments (use code instead)

## 🧪 Testing Quick Reference

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package
go test ./internal/api/...

# Run short tests (excludes webauthn)
go test -short ./...

# Generate coverage HTML
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## 🏃 Development Workflow

```bash
# Install hooks
lefthook install

# Format code
make fmt

# Run linting
make lint

# Run tests
make test

# Build all binaries
make all

# Run main binary
make run
# or
./rpa -once
```

## 🔐 Security Notes

- Tokens stored in `~/.hourglass-rpa/` with 0600 permissions
- WebAuthn credentials in separate file
- Sentry for error tracking (optional)
- No credentials in code or logs
- Whitelist for Telegram users

## 🐳 Docker

```bash
# Build and run
docker-compose up -d

# View logs
docker-compose logs -f rpa

# Token refresh service
docker-compose up -d token-refresh
```

## 📝 Key Files

| File | Purpose |
|------|---------|
| `cmd/rpa/main.go` | Main application entry |
| `internal/bot/runner.go` | Bot orchestration |
| `internal/api/client.go` | Hourglass API client |
| `internal/auth/webauthn/` | Authentication system |
| `internal/notifier/telegram.go` | Telegram integration |
| `internal/scheduler/scheduler.go` | Cron scheduling |
| `lefthook.yml` | Pre-commit hooks config |
| `.golangci.yml` | Linter configuration |

## 🆘 Troubleshooting

**Tests failing with Chrome errors?**
- Set `CHROME_BIN` environment variable
- Or install Chrome/Chromium

**Coverage below threshold?**
- Pre-push requires 95%
- CI requires 70%
- Check `internal/auth/webauthn` tests (excluded from short runs)

**Linting failures?**
- Run `make fmt` first
- Check `.golangci.yml` for exclusions

## 📚 Additional Documentation

- See `README.md` for user-facing docs
- See package comments in source files
- Check `internal/auth/webauthn/AGENTS.md` for auth details
- Check `internal/notifier/AGENTS.md` for notification details

---

**Last Updated**: Generated by init-deep  
**Maintainers**: See GitHub contributors
