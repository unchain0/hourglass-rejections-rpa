# Hourglass RPA verification map

Maintained source for proving user-facing Telegram bot behavior. Read this index, then the feature file for the path you changed.

## Baseline preconditions

- Launch via `.cursor/skills/verify-hourglass-rpa/launch.sh` and keep the printed `SCRATCH`.
- Run `doctor.sh "$SCRATCH"` and require all PASS lines.
- Admin chat id is `9001` (harness whitelist/admin). Do not use a production bot token.
- Scratch SQLite starts empty; `/start` creates the preference row.
- Mock Hourglass starts in `empty` mode (no declined assignments).
- Never drive an instance this skill did not start.

## Driving conventions

- Prefer the feature recipes: `drive.sh "$SCRATCH" feature <name>`.
- Command text and callback_data are literal (`/configure`, `section_Public Witnessing`, `lang_pt-BR`).
- Clear outbound before a step when the recipe says so (`drive.sh "$SCRATCH" clear`).
- After mutations, read preferences (`drive.sh "$SCRATCH" preferences`) as the side-effect check.

## Proof and skip reporting

- Outbound Telegram method + text is the user-visible proof.
- Preference JSON is the durable side-effect proof.
- Report an unreachable path with the command attempted and the doctor/harness error.
- Do not claim a feature verified only because unit tests passed.

## Features

- [Start and help](./start-help.md) — welcome for authorized admin, command list.
- [Configure preferences](./configure-preferences.md) — section keyboard, toggle, save, SQLite sections.
- [Check now](./checknow.md) — manual analysis empty and declined paths.
- [Language](./language.md) — language keyboard and persisted `pt-BR` (or other) choice.
