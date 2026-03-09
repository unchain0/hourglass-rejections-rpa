# AGENTS.md - Notifier

**Directory**: `internal/notifier/`  
**Purpose**: Multi-channel notification system  
**Complexity**: Medium-High  
**Lines**: ~2,500

## 🎯 Overview

Sistema de notificações multi-canal. Telegram é o canal principal, com Resend (email) como fallback secundário.

## 📁 Structure

```
internal/notifier/
├── telegram.go          # Telegram bot integration
├── telegram_test.go     # Telegram tests (large file)
├── resend.go            # Email fallback (Resend API)
├── resend_test.go       # Email tests
└── types.go             # Notification interfaces
```

## 🔍 Key Components

### TelegramNotifier
Primary notification channel:
- Bot API integration (go-telegram/bot)
- Inline keyboards for configuration
- User preferences per section
- Rate limiting (30 req/min)
- i18n support (pt-BR, en-US)

### ResendNotifier
Email fallback:
- Resend API integration
- HTML email templates
- Error handling with retries

## 🏗️ Architecture

```go
// Notifier interface
type Notifier interface {
    Notify(ctx context.Context, notification Notification) error
}

// Both Telegram and Resend implement this interface
```

## ⚙️ Key Features

### User Preferences
- Per-user section filtering
- Inline keyboard configuration
- SQLite-backed storage

### Rate Limiting
- 30 requests per minute
- Token bucket algorithm
- Automatic retry with backoff

### i18n Support
- 143 translation keys
- Portuguese (pt-BR) and English (en-US)
- Dynamic language switching

## 🧪 Testing

- Uses httptest for API mocking
- Mock bot for unit tests
- TestMain for i18n initialization
- Table-driven tests

## 🚨 Anti-Patterns to Avoid

⚠️ **Comments with "ALWAYS"**: Lines ~703, 767, 824 have "Always answer callback first" comments. These should be refactored to code assertions.

## 📝 Usage Example

```go
notifier, err := NewTelegramNotifier(config, preferenceManager)
if err != nil {
    return err
}

err = notifier.Notify(ctx, Notification{
    Title:   "Rejections Detected",
    Message: "1 assignment rejected",
    Section: "Mechanical Parts",
})
```

## 🔗 Dependencies

- `go-telegram/bot` - Telegram Bot API
- `internal/i18n` - Internationalization
- `internal/preferences` - User preferences
- `internal/domain` - Models

## 📚 See Also

- Root `AGENTS.md` for project overview
- `internal/bot/` for bot runner
- `internal/i18n/` for translations
