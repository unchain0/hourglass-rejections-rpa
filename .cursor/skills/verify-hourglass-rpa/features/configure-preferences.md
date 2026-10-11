# Configure preferences

User chooses which Hourglass sections produce Telegram notifications. Choices persist in SQLite and survive later `/status` and `/checknow`.

## Sub-features

- `configure-keyboard` — `/configure` shows one inline button per `AllSections` entry plus Save/Cancel.
- `toggle-section` — callback `section_<name>` flips membership in the draft selection.
- `save-config` — callback `save_config` writes sections to the preference store.

## How to get to it (user POV)

- Send `/configure` after `/start` (or any time once authorized).
- Tap section rows on the inline keyboard, then Save.

## Driving it with verify-harness

Preconditions: launch + doctor PASS.

```sh
.cursor/skills/verify-hourglass-rpa/drive.sh "$SCRATCH" feature configure-preferences
```

Manual equivalent:

1. `message /start` then clear outbound.
2. `message /configure` → outbound contains `Choose sections to receive notifications for:` and reply_markup buttons.
3. `callback section_Public Witnessing`
4. `callback save_config`
5. `preferences` → `sections` includes `Public Witnessing`.

Canonical section strings (callback suffix):

- `Mechanical Parts`
- `Field Ministry`
- `Public Witnessing`
- `Midweek Meeting`

Evidence: `$EVIDENCE/configure-preferences/after-configure-*.json`, `after-save-*.json`.

## Gotchas

- Callback data is `section_` + exact English canonical name, including spaces (`section_Field Ministry`).
- Toggle alone does not persist; Save must run. Cancel discards the draft.
- `/status` is a useful second view after save but is not required by the automated recipe.
