# Telegram Integration Research

Date: 2026-08-20

This project already uses `github.com/go-telegram/bot`, so the safest
improvements were applied at existing seams instead of migrating frameworks.

## Implemented

- All configured whitelist IDs remain authorized after the preference manager is
  installed and are included in scheduled fan-out.
- Privileged commands use the sender's numeric Telegram user ID when available,
  avoiding group-chat identity confusion.
- Rejection snapshots are sorted by stable `(section, who, what)` fields before
  cache comparison, avoiding false notifications caused by concurrent section
  completion order.
- Large rejection notifications are split into semantic messages under
  Telegram's 4096-character limit.
- Callback section and language values are validated against canonical domain
  and locale values before persistence.

## Evidence-backed constraints

- Telegram limits `sendMessage` text to 4096 characters after entity parsing:
  https://core.telegram.org/bots/api#sendmessage
- Flood control exposes machine-readable `retry_after`:
  https://core.telegram.org/bots/api#responseparameters
- Telegram advises approximately one message per second per chat and roughly
  thirty broadcast messages per second on the free tier:
  https://core.telegram.org/bots/faq#my-bot-is-hitting-limits-how-do-i-avoid-this
- Polling and webhooks are mutually exclusive and updates are retained for no
  more than 24 hours:
  https://core.telegram.org/bots/api#getupdates
- Callback queries must be answered:
  https://core.telegram.org/bots/api#callbackquery

## Comparable projects

- `go-telegram/bot`:
  https://github.com/go-telegram/bot/tree/d7672bb6b79139ab7c45bd0feaa3211a4b15f113
  — existing middleware and cancellation primitives fit this project.
- `mymmrac/telego`:
  https://github.com/mymmrac/telego/blob/87321459e011a5c39c3430e3fea9cdc7dd0e4222/telegoapi/caller.go
  — reference for bounded, context-aware 429 retry handling.
- `PaulSonOfLars/gotgbot`:
  https://github.com/PaulSonOfLars/gotgbot/tree/15b72d93695d11852240910bf8aa63b5542599fd
  — reference for bounded dispatch and graceful draining.
- `umputun/tg-spam`:
  https://github.com/umputun/tg-spam/tree/d37cd741258284c77417d6adf3b62c59f9743f20
  — durable quotas for costly/auditable actions.
- `metalmatze/alertmanager-bot`:
  https://github.com/metalmatze/alertmanager-bot/tree/fbfa20b99f9dc3f1a40625108131092dc74fb362
  — numeric sender-ID admin authorization.
- GroupButler:
  https://github.com/group-butler/GroupButler/blob/develop/lua/groupbutler/plugins/help.lua
  — localized edit-in-place UX and callback reauthorization.

## Deferred architecture work

The research identified three larger follow-ups:

1. A durable recipient-scoped outbox keyed by notification fingerprint, with
   explicit at-least-once versus at-most-once semantics.
2. A bounded outbound limiter plus typed 429 retry using
   `TooManyRequestsError.RetryAfter`; ambiguous network retries can duplicate
   messages because Telegram has no caller idempotency key.
3. Observable polling health and a durable update inbox; webhook support would
   require HTTPS, secret validation, durable acknowledgement, and deployment
   changes.

These were intentionally not mixed into this smaller verified increment.
