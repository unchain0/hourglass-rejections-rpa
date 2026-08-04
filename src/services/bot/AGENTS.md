# Bot services guide

## SCOPE

- `BotRunner` owns Telegram orchestration and dependency resolution.
- `manualCheckService` owns one chat's on-demand analysis.

## RUNNER CONTRACT

- Initialize i18n before resolving dependencies; return init failures and capture phase `init_i18n`.
- `WithPreferenceStore` takes precedence. Otherwise require `cfg.DatabaseURL` and open Postgres from it; do not fall back to `DB_TYPE` or SQLite.
- Close only a store created by the runner, and only when it implements `Close`; injected stores remain caller-owned.
- `WithNotifier` takes precedence. Otherwise resolve token and whitelist from config, then `TELEGRAM_BOT_TOKEN` and `TELEGRAM_WHITELIST`.
- Parse the comma-separated whitelist as int64 IDs, skipping invalid entries; use the first valid ID as constructor chat ID.
- Transient manual-send notifiers append the target chat to the resolved whitelist.
- Register the check-now callback before `StartBot`; pass its context and chat ID to a fresh `manualCheckService`.
- `StartBot` owns its listener goroutine. After `ctx.Done()`, call `StopBot`; log and capture stop errors without replacing the nil shutdown result.
- Keep `newTelegramNotifier`, `newPreferenceStoreFromDatabaseURL`, and `i18nInit` injectable; restore overrides after tests.

## MANUAL CHECK CONTRACT

- Load preferences by target chat ID through `PreferenceManager`; missing or failed preference reads return errors.
- Use the user's saved section list, never every canonical section, and default language through `PreferenceManager.GetLanguage`.
- Empty sections send localized `no_sections_selected`; no aggregate rejections send `no_rejections_found`.
- Analyze selected sections sequentially and aggregate successful results.
- Analyzer errors and `JobResult.Error` are recorded and skipped so remaining sections run.
- Preference, context, and notification errors remain actionable and return to the caller.
- Preserve caller context and check cancellation before each section.

## OBSERVABILITY AND SECURITY

- Spans must end and duration metrics must record on every exit path.
- Telemetry capture is best effort; reporting failures must not become operation failures.
- Treat chat IDs as sensitive. Never log Telegram tokens, database URLs or passwords, auth material, or full preference records.

## VERIFICATION

- Run `go test ./src/services/bot`; use `-race` for lifecycle or concurrency changes.
- Synchronize lifecycle tests with injected callbacks/channels instead of sleeps; assert cancellation reaches `StopBot` and runner completion.
