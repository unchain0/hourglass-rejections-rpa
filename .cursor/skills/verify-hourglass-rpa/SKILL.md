---
name: verify-hourglass-rpa
description: Launch and drive Hourglass Rejections RPA — the Telegram bot plus Hourglass rejection check — through an isolated mock Telegram/Hourglass harness. Use when proving bot commands, preference persistence, /checknow analysis, or language changes without touching production tokens.
---

# Verify Hourglass Rejections RPA

Primary surface: the **Telegram bot** (`TelegramNotifier` + `BotRunner` manual-check path). Users talk to the bot with `/start`, `/configure`, `/checknow`, `/language`, and related commands; the daemon also schedules the same analysis.

Other surfaces this skill does **not** drive end-to-end:

- Interactive Chrome bootstrap (`setup-auth`, `save-tokens`) — needs a visible browser and real Hourglass login.
- Live `api.telegram.org` long-polling with a production bot token — Telegram allows only one `getUpdates` consumer; never point this skill at the production token.
- `rpa -once` — still returns `runOnce not implemented`.
- Docker Compose / Coolify deploy paths.

A proof that drives one entry point is incomplete when `features/` lists others. Drive the feature you changed.

## Why a harness (not the raw `rpa` binary)

`NewTelegramNotifier` hard-codes the public Bot API host. Verification starts the **same** notifier type and handlers against a mock Bot API (`-tags verify` constructor), with mock Hourglass HTTP and scratch SQLite. Commands are injected as Telegram updates; outbound `sendMessage` / `editMessageText` bodies are the proof. That is the real user path without stealing the production getUpdates session.

Two instances can run side by side. Each launch owns `/tmp/verify-hourglass-rpa/<run-id>/`, its own ports, and its own SQLite file. Do not point a second drive at another run's scratch directory.

## Launch

From the repository root (Go toolchain from `go.mod`, currently 1.26+):

```sh
.cursor/skills/verify-hourglass-rpa/launch.sh
```

The script:

1. Creates `SCRATCH=/tmp/verify-hourglass-rpa/<run-id>/` (mode 0700) and `evidence/verify-hourglass-rpa/<run-id>/`.
2. Builds `rpa`, `token-refresh`, `save-tokens`, `setup-auth` into the scratch dir (doctor checks these).
3. Builds `verify-harness` with `-tags verify` (pulls in `NewTelegramNotifierForVerify`).
4. Starts the harness with a clean environment (no inherited `TELEGRAM_*`, `HOURGLASS_*`, `DATABASE_URL`).
5. Waits until `state.json` exists and `GET $CONTROL_URL/health` returns ok.

Stdout ends with `SCRATCH=...` and `EVIDENCE=...`. Pass that scratch path to doctor, drive, and cleanup.

Admin chat id inside the harness is **9001** (always authorized). Mock Hourglass defaults to **no** declined assignments until drive sets `hourglass-mode declined`.

## Doctor

Read-only. Run before driving, and again when something looks wrong:

```sh
.cursor/skills/verify-hourglass-rpa/doctor.sh "$SCRATCH"
```

Passes only when all hold:

- The harness pid from `state.json` is alive.
- `GET $CONTROL_URL/health` returns `"ok": true`.
- Scratch SQLite `preferences.db` exists and is non-empty.
- `$SCRATCH/rpa` exists and is executable.

Writes `evidence/verify-hourglass-rpa/<run-id>/doctor.txt`. Does not send Telegram updates or mutate preferences.

## Drive

Harness: control HTTP API on the instance (`state.json` → `control_url`). Prefer the wrapper:

```sh
DRIVE=.cursor/skills/verify-hourglass-rpa/drive.sh

# Full feature recipes (write evidence under $EVIDENCE/<feature>/)
$DRIVE "$SCRATCH" feature start-help
$DRIVE "$SCRATCH" feature configure-preferences
$DRIVE "$SCRATCH" feature checknow
$DRIVE "$SCRATCH" feature language

# Manual steps when debugging
$DRIVE "$SCRATCH" clear
$DRIVE "$SCRATCH" message /start
$DRIVE "$SCRATCH" wait-outbound 1
$DRIVE "$SCRATCH" outbound
$DRIVE "$SCRATCH" callback 'section_Public Witnessing'
$DRIVE "$SCRATCH" callback save_config
$DRIVE "$SCRATCH" hourglass-mode declined
$DRIVE "$SCRATCH" preferences
```

Stable handles (Telegram text / callback_data, not coordinates):

| User action | Drive handle |
| --- | --- |
| Open bot | `message /start` |
| Help | `message /help` |
| Configure sections | `message /configure` |
| Toggle a section | `callback section_<AllSections name>` e.g. `section_Public Witnessing` |
| Save / cancel config | `callback save_config` / `callback cancel_config` |
| Status / stats / whoami | `message /status` etc. |
| Manual check | `message /checknow` |
| Language menu | `message /language` |
| Pick language | `callback lang_en` \| `lang_pt-BR` \| `lang_es` \| `lang_fr` |

Canonical section names are `domain_models.AllSections`: `Mechanical Parts`, `Field Ministry`, `Public Witnessing`, `Midweek Meeting`.

Per-feature sequences and expected proof live under `features/`.

## Evidence

Proof lands in `evidence/verify-hourglass-rpa/<run-id>/` (gitignored). Cleanup never deletes it.

Standards:

- Exercise the real handlers (`StartBot` registration + long-poll `ProcessUpdate` path). Do not call `PreferenceManager.UpdateSections` or `AnalyzeSection` from the drive script to fake the outcome.
- Capture the action and the resulting state: outbound JSON (`*-outbound.json`) plus SQLite preferences (`*-preferences.json` via control `/preferences`).
- Side effects: after configure+save, preferences must list the toggled section; after language callback, `language` must be `pt-BR` (or the chosen code); after `/checknow` with `hourglass-mode declined` and Public Witnessing enabled, outbound must include a rejection notification (not only "Immediate check requested").
- Mock boundaries: Telegram and Hourglass are local HTTP. Confirm by `state.json` URLs on `127.0.0.1` and by the absence of production env vars in the harness process. Do not set `TELEGRAM_BOT_TOKEN` to a real token for this skill.
- Auth CLIs that open Chrome are out of scope; do not treat a green unit test as a substitute for a mapped bot feature.

## Cleanup

```sh
.cursor/skills/verify-hourglass-rpa/cleanup.sh "$SCRATCH"
```

Stops only the recorded harness pid (cmdline must contain `verify-harness` or the scratch path). Deletes the scratch directory. Leaves `evidence/verify-hourglass-rpa/<run-id>/` alone. Never `pkill rpa` or kill by binary name alone.

Run cleanup after a failed launch or drive too, once you have copied any useful stderr into evidence.

## Helpers

| Script | Invocation |
| --- | --- |
| `launch.sh` | `.cursor/skills/verify-hourglass-rpa/launch.sh` |
| `doctor.sh` | `.cursor/skills/verify-hourglass-rpa/doctor.sh "$SCRATCH"` |
| `drive.sh` | `.cursor/skills/verify-hourglass-rpa/drive.sh "$SCRATCH" feature start-help` |
| `cleanup.sh` | `.cursor/skills/verify-hourglass-rpa/cleanup.sh "$SCRATCH"` |

Harness binary (built into scratch): `verify-harness serve|doctor|drive`.

Verify-only product hook: `src/services/notification/verify_export.go` (`//go:build verify`) — not linked into `make build` / release images.

## Feature map

- [Start and help](features/start-help.md)
- [Configure preferences](features/configure-preferences.md)
- [Check now](features/checknow.md)
- [Language](features/language.md)

Index: [features/README.md](features/README.md).
