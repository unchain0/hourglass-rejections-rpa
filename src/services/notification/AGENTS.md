# Notification package guide

## Scope

- Package import name: `notifier`.
- `TelegramNotifier` handles interactive Telegram delivery and preferences.
- `ResendNotifier` sends completion, failure, and daily-report email through `domain.Notifier`.
- Public Telegram send methods return wrapped delivery errors; handler side effects are best effort.

## Telegram contracts

- An empty whitelist authorizes every chat. Otherwise, only listed chat IDs are authorized.
- Chat discovery is separate from authorization; `/start` may record a chat without whitelisting it.
- Interactive handlers use the per-chat rolling limiter before work: 30 attempts per minute.
- `StartBot` requires a non-nil `PreferenceManager`.
- Preference mutations go through the manager; do not bypass it with direct storage access.
- Empty rejection sets are a no-op. Escape every dynamic value included in Telegram HTML.
- Handler send, preference-store, callback-answer, and message-edit failures are intentionally non-fatal unless the surrounding handler defines otherwise.

### Callback ordering

- Unauthorized callbacks answer with an alert and return.
- Section-toggle, save, and cancel callbacks answer before preference or message work.
- Language selection is the deliberate exception: best-effort language update, callback answer, then message edit.
- Preserve these orders when changing callback handlers; delayed answers degrade Telegram UX.

## Resend contract

- There is no retry or backoff.
- HTTP 200 and 201 are the only success statuses.
- Marshal, request-construction, transport, and other HTTP status failures return to the caller.

## Test seams

- Run `go test ./src/services/notification`.
- Telegram tests use injected bot factories/functions and HTTP fakes; restore package-level seams with `t.Cleanup`.
- Preference-backed cases use temporary stores; keep authorization, limiter, callback ordering, and best-effort failures independently testable.
- Resend tests inject marshal, request, and transport failures and assert the 200/201 status boundary.
