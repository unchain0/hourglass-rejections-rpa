# Hourglass Rejections RPA

Lightweight Go automation that checks Hourglass for rejected assignments, stores operational history in PostgreSQL, and delivers notifications through a Telegram bot with per-user preferences.

## What it does

- reads Hourglass through the official API
- sends Telegram notifications only when there are new changes
- lets each user choose which sections they want to follow
- persists preferences and rejection history in the configured database
- supports static tokens or automatic token renewal with WebAuthn

This repository is a good fit if you want a self-hosted monitor for Hourglass rejections with a bot interface. It is a bad fit if you need a generic browser scraper or a multi-tenant SaaS service.

## Quickstart

### Option 1: bootstrap automatic renewal from token and cookie

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

With `AUTO_REFRESH_TOKENS=true` (the default), a valid token/cookie pair is used only to register a WebAuthn credential on the first start. The daemon then obtains fresh cookies without Chrome and stores the credential and renewed tokens under `~/.hourglass-rpa/`. Success means the logs contain `registered WebAuthn credential` followed by `WebAuthn authentication succeeded`, and the bot is ready to accept `/start`.

Keep that directory on persistent storage. If the directory is discarded on every deploy, Hourglass receives a new passkey on every start.

### Option 2: bootstrap from a visible browser

If this will run on a VPS or long-lived container, generate persisted auth files first:

```bash
make setup-auth
```

That command opens a normal Chrome window so you can sign in with Google once, then it creates `auth-tokens.json` and `webauthn-credentials.json` under `~/.hourglass-rpa/`. After that first bootstrap, the daemon renews access automatically in headless mode by preferring the stored WebAuthn credential and retrying once if Hourglass invalidates the session early. `CHROME_PROFILE_DIR` remains an optional fallback when you want to reuse the persisted browser profile as well.

Use this alternative when the token/cookie pair is already expired or unavailable. For VPS use, copy at least these files to the server and keep them persistent across restarts:

- `~/.hourglass-rpa/auth-tokens.json`
- `~/.hourglass-rpa/webauthn-credentials.json`
- optional fallback: `~/.hourglass-rpa/chrome-profile/`

Then point `TOKENS_PATH`, `WEBAUTHN_CREDENTIALS_PATH`, and optionally `CHROME_PROFILE_DIR` at those files before starting the daemon.

## Requirements

- Go 1.26 for local development
- Telegram bot token from [@BotFather](https://t.me/BotFather)
- Hourglass authentication, either:
  - one valid `HOURGLASS_XSRF_TOKEN` + `HOURGLASS_HGLOGIN_COOKIE` pair for automatic first-start bootstrap, or
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
# On a workstation with a visible Chrome/Chromium session:
make setup-auth
make copy-to-vps VPS=user@your-seedbox

# On the seedbox, import both files into the persistent Docker volume:
AUTH_SOURCE_DIR="$HOME/.hourglass-rpa" docker compose --profile bootstrap run --rm auth-bootstrap

# Rebuild from the current runtime base and start the services.
export APP_VERSION="$(git describe --always --dirty)"
export GIT_COMMIT="$(git rev-parse --short=12 HEAD)"
docker compose build --pull --no-cache rpa
docker compose up -d rpa
```

The default compose file starts the main `rpa` service with a persisted authentication volume. When valid Hourglass token/cookie variables are present, a fresh deployment creates its WebAuthn credential directly from that session; importing files is optional. Leave `CHROME_PROFILE_DIR` unset unless you also imported a previously authenticated browser profile.

The main service already renews tokens automatically. The optional `autorefresh` profile should only be used when `AUTO_REFRESH_TOKENS=false`, so two processes do not renew the same files concurrently:

```bash
AUTO_REFRESH_TOKENS=false docker compose --profile autorefresh up -d
```

### Prepare authentication for servers

```bash
make setup-auth
make copy-to-vps VPS=user@your-vps.com
```

Useful helper binaries are also available through the Makefile:

- `make save-tokens` for manual token capture
- `make token-refresh` for a standalone refresh attempt

### Deploy with Coolify

Configure the required Telegram and Hourglass variables in Coolify, keep `AUTO_REFRESH_TOKENS=true`, and add persistent storage mounted at:

```text
/home/rpa/.hourglass-rpa
```

Do not set `CHROME_PROFILE_DIR` for the normal passkey flow. The first deployment consumes the valid Hourglass token/cookie pair to register the RPA credential; subsequent deployments reuse `webauthn-credentials.json` and `auth-tokens.json` from the volume. After the first successful start, the original environment token and cookie may expire without interrupting renewal.

The container health check reports whether the `rpa` daemon is alive. Authentication readiness is intentionally separate: token and cookie may come from Coolify variables before the persistent token file is created, while renewal failures remain visible in the application logs.

Keep exactly one running `rpa` instance for each Telegram bot token. Telegram long polling permits only one `getUpdates` consumer, so a second Coolify replica or a local daemon using the production token will cause HTTP 409 conflicts. Use the automated test suite or a separate development bot token for local validation, and keep the Coolify replica count at one.

## Configuration

The full variable list lives in `.env.example`. In practice, these are the keys most people need first:

```env
TELEGRAM_BOT_TOKEN=your_bot_token
TELEGRAM_WHITELIST=123456789,987654321
HOURGLASS_XSRF_TOKEN=your_xsrf_token
HOURGLASS_HGLOGIN_COOKIE=your_hglogin_cookie

# Files created automatically from the initial token/cookie pair.
# They must live on persistent storage in a VPS/container.
TOKENS_PATH=/home/user/.hourglass-rpa/auth-tokens.json
WEBAUTHN_CREDENTIALS_PATH=/home/user/.hourglass-rpa/webauthn-credentials.json
# CHROME_PROFILE_DIR is only an optional browser-profile fallback.

DB_TYPE=postgres
DATABASE_URL=postgres://user:password@host:5432/postgres

OTEL_EXPORTER_OTLP_ENDPOINT=https://otel.example.com
OTEL_EXPORTER_OTLP_HEADERS=Authorization=Bearer token,stream-name=default
OTEL_SERVICE_NAME=hourglass-rejections-rpa
OTEL_SERVICE_VERSION=1.0.0
LOG_LEVEL=info
TZ=America/Sao_Paulo
```

Important notes:

- `AUTO_REFRESH_TOKENS` defaults to `true`.
- when Hourglass returns `401`, the client forces one fresh renewal before failing the run.
- database schema is created automatically on startup when missing
- OpenTelemetry export is optional but recommended for logs, traces, and metrics

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
internal/preferences/   per-user preference storage and operational history
internal/scheduler/     periodic execution and notification dispatch
```

## Additional project docs

- `AGENTS.md` for a high-level maintainer map
- `internal/api/AGENTS.md` for API client details
- `internal/auth/webauthn/AGENTS.md` for auth and token renewal internals
- `internal/notifier/AGENTS.md` for Telegram notifier details

## License

MIT. See [LICENSE](LICENSE).
