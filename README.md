# Hourglass Rejections RPA 🤖

[![CI](https://github.com/unchain0/hourglass-rejections-rpa/actions/workflows/ci.yml/badge.svg)](https://github.com/unchain0/hourglass-rejections-rpa/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/go-1.24-blue.svg)](https://golang.org)
[![Coverage](https://img.shields.io/badge/coverage-86.2%25-brightgreen.svg)](https://github.com/unchain0/hourglass-rejections-rpa/actions)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Docker](https://img.shields.io/badge/docker-ready-blue.svg)](docker-compose.yml)

Automated system to monitor assignment rejections in Hourglass. Runs 2x daily (9 AM and 5 PM) and sends Telegram notifications when rejections are detected in: Mechanical Parts, Field Ministry, and Public Witnessing sections.

- **Official REST API**: Uses Hourglass API directly (no browser)
- **Secure Authentication**: XSRF Token + Session Cookie
- **Interactive Telegram Bot**: Configure preferences via bot commands
- **Per-User Filtering**: Each user receives only their selected sections
- **Smart Scheduling**: Cron jobs for 9 AM and 5 PM daily
- **Multiple Formats**: Exports data in JSON and CSV
- **Maximum Range**: Searches for rejections up to 2 years (730 days)
- **86.2% Coverage**: Comprehensive unit tests

## 📋 Prerequisites

- Go 1.24+ (for development)
- Docker (for deployment)
- Telegram account (for notifications)

## 🔧 Installation

### Via Docker (Recommended)

```bash
docker-compose up -d
```

### Via Go (Development)

```bash
go install github.com/unchain0/hourglass-rejections-rpa/cmd/rpa@latest
```

### Local Build (Manual)

Build the binary manually from source:

```bash
# Clone the repository
git clone https://github.com/unchain0/hourglass-rejections-rpa.git
cd hourglass-rejections-rpa

# Download dependencies
go mod download

# Build the binary
go build -o rpa ./cmd/rpa

# Make it executable (Linux/macOS)
chmod +x rpa

# Run it
./rpa -once
```

**Requirements:**
- Go 1.24 or higher
- Git

**Build for different platforms:**
```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o rpa-linux ./cmd/rpa

# macOS
GOOS=darwin GOARCH=amd64 go build -o rpa-macos ./cmd/rpa

# Windows
GOOS=windows GOARCH=amd64 go build -o rpa.exe ./cmd/rpa
```

## ⚙️ Configuration

Copy the `.env.example` file to `.env` and configure:

```bash
cp .env.example .env
```

### Required Variables

```env
# Hourglass API (required)
HOURGLASS_XSRF_TOKEN=your_token_here
HOURGLASS_HGLOGIN_COOKIE=your_cookie_here

# Telegram Bot (required for notifications)
TELEGRAM_BOT_TOKEN=your_bot_token_here

# Authorized users whitelist (optional)
TELEGRAM_WHITELIST=123456789,987654321
```

### Optional Variables

```env
# User Preferences (for per-user filtering)
USER_PREFS_FILE=data/preferences.json

# Sentry (error tracking)
SENTRY_DSN=https://xxxxxx@xxx.ingest.sentry.io/xxxxx
SENTRY_ENVIRONMENT=production

# Grafana (metrics)
GRAFANA_API_KEY=glc_xxxxxxxx

# General settings
LOG_LEVEL=info
TZ=America/Sao_Paulo
OUTPUT_DIR=./outputs
```

## 🔑 Getting Credentials

### XSRF Token and Cookie

1. Log in to [Hourglass](https://app.hourglass-app.com) via browser
2. Open DevTools (F12) → Network
3. Perform any action in the system
4. Look for a request starting with `api/v0.2/`
5. In the request headers, copy:
   - `X-Hourglass-XSRF-Token`
   - Cookie `hglogin`

### Telegram Bot

1. Send `/newbot` to [@BotFather](https://t.me/BotFather)
2. Follow instructions to create the bot
3. Copy the provided token (format: `123456789:ABCdef...`)
4. Send a message to your bot
5. Access: `https://api.telegram.org/bot<YOUR_TOKEN>/getUpdates`
6. Look for your `chat.id` in the response

## 🎮 Usage

### Single Run Mode

To run once immediately:

```bash
./rpa -once
```

### Scheduled Mode (Production)

To start the scheduler (9 AM and 5 PM):

```bash
./rpa
```

### Docker

```bash
# Build and run
docker build -t hourglass-rejections-rpa .
docker run --env-file .env hourglass-rejections-rpa -once
```

## 🤖 Telegram Bot Commands

After sending a message to your bot, you can use these commands:

| Command | Description |
|---------|-------------|
| `/start` | Welcome message and instructions |
| `/configurar` | Configure which sections to receive notifications for |
| `/status` | Show your current preferences |
| `/ajuda` | List all available commands |
| `/checknow` | Trigger immediate check (admin only) |

### User Preferences

Users can customize which sections they receive notifications for:

1. Send `/configurar` to the bot
2. Click on sections to toggle them on/off (✅/❌)
3. Click "Salvar" to save your preferences
4. You'll only receive notifications for selected sections

Available sections:
- Partes Mecânicas (Mechanical Parts)
- Campo (Field Ministry)
- Testemunho Público (Public Witnessing)
- Reunião Meio de Semana (Midweek Meeting)

## 🧪 Testing

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Detailed coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## 📊 Project Structure

```
.
├── cmd/rpa/              # Entry point
├── internal/
│   ├── api/              # Hourglass API client
│   ├── config/           # Configuration
│   ├── domain/           # Models and interfaces
│   ├── notifier/         # Telegram notifications
│   ├── preferences/      # User preferences storage
│   ├── scheduler/        # Cron scheduling
│   ├── storage/          # JSON/CSV persistence
│   ├── logger/           # Structured logging
│   ├── sentry/           # Error tracking
│   └── metrics/          # Grafana metrics
├── .github/workflows/    # CI/CD
├── Dockerfile            # Container (34.8MB)
├── docker-compose.yml    # Orchestration
├── go.mod               # Dependencies
└── README.md            # Documentation
```

## 📝 Output Format

### JSON

```json
[
  {
    "section": "Mechanical Parts",
    "who": "John Smith",
    "what": "Audio/Video & Indicators",
    "when": "09/03/2026",
    "timestamp": "2026-03-01T19:30:00Z"
  }
]
```

### CSV

```csv
section,who,what,when,timestamp
Mechanical Parts,John Smith,Audio/Video & Indicators,09/03/2026,2026-03-01T19:30:00Z
```

### Telegram Notification

```
❌ Rejections Detected in Hourglass

1 assignment(s) rejected:

Rejection #1:
👤 Who: John Smith
📋 Section: Mechanical Parts
📝 Assignment: Audio/Video & Indicators
📅 Date: 09/03/2026
```

## 🔒 Security

- ✅ Tokens in environment variables (never committed)
- ✅ `.env` file in `.gitignore`
- ✅ Non-root container execution
- ✅ Whitelist for access control
- ✅ No credential storage in code

## 🐳 Deploy with Coolify

This project is compatible with [Coolify](https://coolify.io):

1. Connect your Git repository
2. Select "Docker Compose" type
3. Configure environment variables in the panel
4. Automatic deployment!

Coolify will automatically detect the `docker-compose.yml` and build.

## 🛠️ Technologies

- **Go 1.24** - Main language
- **go-telegram/bot** - Telegram integration
- **robfig/cron** - Scheduling
- **charmbracelet/log** - Logging
- **Docker** - Containerization

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

