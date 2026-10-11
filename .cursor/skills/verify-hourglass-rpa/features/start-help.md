# Start and help

Authorized admin opens the bot and reads the command list. Unauthorized users are out of scope for the default harness (admin chat 9001 is always allowed).

## Sub-features

- `start-welcome` — `/start` creates a preference row and replies with the English welcome string.
- `help-list` — `/help` lists slash commands including `/checknow` and admin commands.

## How to get to it (user POV)

- Telegram: send `/start` to the bot after it is running.
- Telegram: send `/help` (or `/help@botname` prefix path exists in code; harness uses exact `/help`).

## Driving it with verify-harness

Preconditions: launch + doctor PASS. Outbound cleared optional.

- User sends `/start` → `drive.sh "$SCRATCH" feature start-help` (or `message /start` then `wait-outbound 1`).
  - Expect outbound text containing `Welcome to Hourglass RPA Bot!`.
  - Expect preferences `exists: true` for chat `9001`.
- User sends `/help` → included in the same feature recipe after clear.
  - Expect outbound text containing `/checknow` and `/configure`.

Evidence: `$EVIDENCE/start-help/after-start-*.json`, `after-help-*.json`.

## Gotchas

- `handleStart` does not send `welcome_instructions`; only the short `welcome` string. Do not fail the proof looking for configure instructions on `/start`.
- Rate limit is 30 messages/minute/chat — fine for recipes, not for tight spam loops.
