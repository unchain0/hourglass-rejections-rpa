# AGENTS.md - Hourglass Rejections RPA

**Generated:** 2026-03-03
**Project:** Hourglass Rejections RPA
**Language:** Go 1.24
**Lines of Code:** ~10,600
**Test Coverage:** 98.3%

---

## OVERVIEW

Automated monitoring system for Hourglass assignment rejections. Fetches data via REST API, analyzes for declined assignments across 4 sections (Mechanical Parts, Field Ministry, Public Witnessing, Midweek Meetings), and notifies users via Telegram bot.

**Core Stack:**
- Go 1.24 with standard library patterns
- SQLite (GORM) for user preferences
- Telegram Bot API (go-telegram/bot)
- Sentry for error tracking
- Charmbracelet/log for structured logging

---

## STRUCTURE

```
.
├── cmd/                    # Entry points
│   ├── rpa/               # Main application
│   └── generate-key/      # Utility for generating API keys
├── internal/              # Private application code
│   ├── api/              # Hourglass REST API client
│   ├── bot/              # Telegram bot runner
│   ├── cache/            # Rejection deduplication cache
│   ├── config/           # Environment configuration
│   ├── domain/           # Core models/interfaces
│   ├── logger/           # Structured logging setup
│   ├── metrics/          # Grafana metrics (optional)
│   ├── notifier/         # Telegram notifications
│   ├── preferences/      # User preference storage
│   ├── scheduler/        # Cron-like job scheduler
│   ├── sentry/           # Error tracking integration
│   └── storage/          # JSON/CSV file persistence
├── docker-compose.yml    # Container orchestration
├── Dockerfile            # Multi-stage build (34MB)
└── .golangci.yml         # Linting rules
```

---

## WHERE TO LOOK

| Task | Location | Notes |
|------|----------|-------|
| **Add new API endpoint** | `internal/api/client.go` | Follow existing pattern with error wrapping |
| **Modify Telegram commands** | `internal/notifier/telegram.go` | Handlers registered in `StartBot()` |
| **Add new analysis section** | `internal/api/analyzer.go` | Add case to `AnalyzeSection()` switch |
| **Change scheduling logic** | `internal/scheduler/scheduler.go` | `runWithTicker()` for intervals |
| **Update user preferences** | `internal/preferences/store.go` | GORM models with JSON fields |
| **Add error tracking** | `internal/sentry/sentry.go` | Already injected throughout |
| **Build commands** | `Dockerfile` | Multi-stage Alpine build |
| **Test patterns** | `*_test.go` | Table-driven tests with testify |

---

## CONVENTIONS (Non-Standard)

### 1. Package-Level Variables for Test Injection
```go
// In production code, vars allow test mocking:
var newTelegramNotifier = func(token string, ...) (Notifier, error) {
    return notifier.NewTelegramNotifier(token, ...)
}
```
**Used in:** `cmd/rpa/main.go`, `internal/notifier/resend.go`, `internal/storage/json.go`

### 2. Error Wrapping Pattern
Always wrap errors with context:
```go
return fmt.Errorf("failed to X: %w", err)
```
**Not:** `return err` or `errors.New("failed")`

### 3. Sentry Integration
Error capture throughout, nil-safe:
```go
if s.sentryClient != nil {
    s.sentryClient.CaptureError(err, map[string]interface{}{
        "section": section,
        "phase": "analysis",
    })
}
```

### 4. HTML Escaping for Telegram
All user data escaped before HTML messages:
```go
msg.WriteString(fmt.Sprintf("👤 <b>Who:</b> %s\n", html.EscapeString(r.Quem)))
```

### 5. Parallel Analysis with Mutex
Section analysis runs in goroutines with `sync.Mutex` for result collection.

---

## ANTI-PATTERNS (Forbidden Here)

| Pattern | Why Forbidden | Enforced By |
|---------|---------------|-------------|
| **Direct `os.Exit()`** | Use `osExit` var for testability | Custom var injection |
| **0644 file permissions** | Sensitive data must use 0600 | Code review + gosec |
| **Unescaped HTML in Telegram** | XSS vulnerability | `html.EscapeString()` required |
| **Sequential API calls** | Use goroutines for parallel sections | Performance requirements |
| **Ignored errors in tests** | Must check all errors | `errcheck` linter |
| **Complex functions (>15)** | Cognitive complexity limit | `gocognit` linter (min: 15) |
| **Naked returns** | Explicit returns required | `nakedret` linter |

---

## COMMANDS

```bash
# Development
go run ./cmd/rpa              # Run with scheduler
go run ./cmd/rpa -once        # Single execution
go test ./...                 # Run all tests
go test -race ./...           # Race detector

# Build
docker-compose up -d          # Production deploy
go build -o rpa ./cmd/rpa     # Binary build

# Linting
lefthook run                  # Pre-commit hooks
golangci-lint run             # Manual lint

# Coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

---

## CONFIGURATION

**Required env vars:**
```bash
HOURGLASS_XSRF_TOKEN=        # From browser devtools
HOURGLASS_HGLOGIN_COOKIE=    # Session cookie
TELEGRAM_BOT_TOKEN=          # From @BotFather
TELEGRAM_WHITELIST=          # Comma-separated chat IDs
```

**Optional:**
```bash
SENTRY_DSN=                  # Error tracking
SQLITE_DB_PATH=              # Default: data/hourglass.db
SCHEDULE_MORNING=            # Cron: "0 9 * * *"
SCHEDULE_EVENING=            # Cron: "0 17 * * *"
```

---

## TESTING

- **Coverage Target:** 100% (currently 98.3%)
- **Pattern:** Table-driven tests with `testify/assert`
- **Mocking:** Package-level vars + interface injection
- **Race Detection:** Enabled in CI (`go test -race`)
- **Test Files:** 17 test files for 21 source files

---

## NOTES

1. **Parallel Analysis:** 4 sections analyzed concurrently → ~4x speedup
2. **Rate Limiting:** 10 commands/minute per Telegram user
3. **File Permissions:** All sensitive files use 0600 (owner-only)
4. **Context Cancellation:** All long operations check `ctx.Done()`
5. **Race Safety:** `sync.Mutex` on all shared state
6. **CI/CD:** GitHub Actions with lefthook pre-commit hooks

---

## RELATED FILES

- `.golangci.yml` - Linter configuration (18 rules)
- `docker-compose.yml` - Production deployment
- `.env.example` - Environment template
- `docs/hourglass-mapping.md` - API field mappings
