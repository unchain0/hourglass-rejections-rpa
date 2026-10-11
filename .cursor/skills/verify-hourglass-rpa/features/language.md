# Language

User switches bot locale among en, pt-BR, es, fr. Choice is stored on the preference row and changes subsequent copy.

## Sub-features

- `language-menu` — `/language` shows four inline language buttons.
- `language-select` — callback `lang_pt-BR` (or en/es/fr) persists language and edits the menu message to the confirmation string.

## How to get to it (user POV)

- Send `/language`, tap a flag row.

## Driving it with verify-harness

Preconditions: launch + doctor PASS.

```sh
.cursor/skills/verify-hourglass-rpa/drive.sh "$SCRATCH" feature language
```

Manual equivalent:

1. `message /start`, clear.
2. `message /language` → text `Select your language:` (en).
3. `callback lang_pt-BR`.
4. `preferences` → `"language": "pt-BR"`.
5. Outbound/edit text contains `Idioma alterado` or the en confirmation if edit landed differently.

Evidence: `$EVIDENCE/language/language-menu-*.json`, `after-lang-*.json`.

## Gotchas

- Callback codes are `lang_en`, `lang_pt-BR`, `lang_es`, `lang_fr` — note the hyphen in `pt-BR`.
- Confirmation is an `editMessageText` on the menu message, not always a new `sendMessage`.
- Default language before any preference row is `en`.
