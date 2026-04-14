# Hourglass Rejections RPA

Lightweight Go automation that checks Hourglass for rejected assignments, stores the results as JSON/CSV, and delivers notifications through a Telegram bot with per-user preferences.

## What it does

- reads Hourglass through the official API
- sends Telegram notifications only when there are new changes
- lets each user choose which sections they want to follow
- persists results in `./outputs` and preferences in SQLite by default
- supports static tokens or automatic token renewal with WebAuthn

This repository is a good fit if you want a self-hosted monitor for Hourglass rejections with a bot interface. It is a bad fit if you need a generic browser scraper or a multi-tenant SaaS service.

## Quickstart

### Option 1: start the daemon with static tokens

1. Copy the environment template.

   ```bash
   cp .env.example .env
   ```

2. Fill at least these variables in `.env`:

   ```env
   TELEGRAM_BOT_TOKEN=your_bot_token
   TELEGRAM_WHITELIST=123456789
   HOURGLASS_XSRF_TOKEN=your_xsrf_token
   HOURGLASS_HGLOGIN_COOKIE=your_hglogin_cookie
   ```

3. Start the app.

   ```bash
   make run
   ```

Success means the app starts without errors and the bot is ready to accept `/start`.

### Option 2: set up automatic token renewal with Google bootstrap

If this will run on a VPS or long-lived container, generate persisted auth files first:

```bash
make setup-auth
```

That command opens a normal Chrome window so you can sign in with Google once, then it creates `auth-tokens.json` and `webauthn-credentials.json` under `~/.hourglass-rpa/`. After that first bootstrap, the daemon renews access automatically in headless mode by preferring the stored WebAuthn credential and retrying once if Hourglass invalidates the session early. `CHROME_PROFILE_DIR` remains an optional fallback when you want to reuse the persisted browser profile as well.

For VPS use, copy at least these files to the server and keep them persistent across restarts:

- `~/.hourglass-rpa/auth-tokens.json`
- `~/.hourglass-rpa/webauthn-credentials.json`
- optional fallback: `~/.hourglass-rpa/chrome-profile/`

Then point `TOKENS_PATH`, `WEBAUTHN_CREDENTIALS_PATH`, and optionally `CHROME_PROFILE_DIR` at those files before starting the daemon.

## Requirements

- Go 1.26 for local development
- Telegram bot token from [@BotFather](https://t.me/BotFather)
- Hourglass authentication, either:
  - `HOURGLASS_XSRF_TOKEN` + `HOURGLASS_HGLOGIN_COOKIE`, or
  - persisted token files plus WebAuthn credentials from the Google bootstrap flow
- Chromium/Chrome available when you use the auth setup tools

## Main workflows

### Run the daemon

```bash
make run
```

The main binary starts both the Telegram bot and the scheduler. The scheduler checks more frequently during the day and less frequently overnight.

### Start with Docker Compose

```bash
docker-compose up -d
```

The default compose file starts the main `rpa` service with persisted data and token volumes.

### Prepare authentication for servers

```bash
make setup-auth
make copy-to-vps VPS=user@your-vps.com
```

Useful helper binaries are also available through the Makefile:

- `make save-tokens` for manual token capture
- `make token-refresh` for a standalone refresh attempt

## Configuration

The full variable list lives in `.env.example`. In practice, these are the keys most people need first:

```env
TELEGRAM_BOT_TOKEN=your_bot_token
TELEGRAM_WHITELIST=123456789,987654321
HOURGLASS_XSRF_TOKEN=your_xsrf_token
HOURGLASS_HGLOGIN_COOKIE=your_hglogin_cookie

# or use persisted auth files instead of static tokens
# make setup-auth performs first Google login in visible Chrome,
# then headless renewals use stored WebAuthn credentials automatically.
TOKENS_PATH=/home/user/.hourglass-rpa/auth-tokens.json
WEBAUTHN_CREDENTIALS_PATH=/home/user/.hourglass-rpa/webauthn-credentials.json
CHROME_PROFILE_DIR=/home/user/.hourglass-rpa/chrome-profile

OUTPUT_DIR=./outputs
SQLITE_DB_PATH=data/hourglass.db
LOG_LEVEL=info
TZ=America/Sao_Paulo
```

Important notes:

- `AUTO_REFRESH_TOKENS` defaults to `true`.
- when Hourglass returns `401`, the client forces one fresh renewal before failing the run.
- preferences use SQLite by default
- results are written as both JSON and CSV
- Sentry and Grafana settings are optional

## Telegram bot commands

The bot currently registers these commands:

| Command | Purpose |
| --- | --- |
| `/start` | Show the welcome message |
| `/configure` | Choose which sections each user follows |
| `/status` | Show current preferences |
| `/stats` | Show bot statistics |
| `/whoami` | Show account details |
| `/language` | Change the bot language |
| `/help` | List available commands |
| `/checknow` | Trigger an immediate check |

## Development

Common local commands:

```bash
make fmt
make lint
make test
make ci
make build
```

After the daemon is running, use `/checknow` in Telegram for an immediate manual check.

This repository builds four binaries from `cmd/`:

- `rpa`
- `save-tokens`
- `token-refresh`
- `setup-auth`

## Project layout

```text
cmd/                    entrypoints for the daemon and auth utilities
internal/api/           Hourglass API client and analysis logic
internal/auth/webauthn/ browser-assisted auth and token renewal
internal/bot/           Telegram bot runner
internal/notifier/      Telegram notification handlers
internal/preferences/   per-user preference storage
internal/scheduler/     periodic execution and notification dispatch
internal/storage/       JSON and CSV output writers
```

## Additional project docs

- `AGENTS.md` for a high-level maintainer map
- `internal/api/AGENTS.md` for API client details
- `internal/auth/webauthn/AGENTS.md` for auth and token renewal internals
- `internal/notifier/AGENTS.md` for Telegram notifier details

## License

MIT. See [LICENSE](LICENSE).
