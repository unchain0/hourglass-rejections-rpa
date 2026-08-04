# Scheduler Package Guide

## Scope

- `Scheduler` drives periodic Hourglass analysis and changed-rejection notifications.
- `New` owns an in-memory `RejectionCache`; callers provide the analyzer, storage, telemetry, and optional notifier.
- Keep `domain.AllSections` as the canonical fan-out source.

## Timing and execution

- `Run` polls with a one-minute ticker. Due runs set `nextRun` to 30 minutes later from 06:00 through 21:59, and 2 hours later otherwise.
- Ticks before `nextRun` are skipped; cancellation stops the ticker loop cleanly.
- Each due run gets a child context with a 10-minute timeout and a `scheduled_analysis` span.
- `runAnalysis` starts one goroutine per `domain.AllSections` entry and waits for all of them. Keep the aggregate rejection slice synchronized.
- `Storage.SaveRejections` may be called concurrently by section goroutines, so implementations and test doubles must be concurrency-safe.
- Analyzer errors and `JobResult.Error` are logged/captured per section and do not fail the whole run.
- Non-empty results are persisted per section; save errors are also logged/captured and swallowed.
- After each due run, a non-nil store records `scheduled_analysis` through `RecordJobExecution`, with success and error text derived from the run result. Recording failures are warnings and do not replace that result; counters update afterward.

## Notification invariants

- Empty results never notify. Non-empty results notify only when `RejectionCache.HasChanges` detects a change and a notifier is configured.
- Cache comparison is positional and uses `Section`, `Who`, and `What`; a `When`-only difference does not open the notification gate.
- Preserve first-seen section order in `buildNotificationSummary`.
- The notifier receives the changed rejection snapshot so the delivery layer can filter it per recipient.
- `cmd/rpa/runFullMode` wires the shared `BotRunner` as the notifier. Notifier errors propagate as the scheduled run error; a nil notifier remains a deliberate no-op for isolated tests and alternate callers.
- A missing notifier or failed delivery resets the candidate cache entry so an identical snapshot remains eligible for the next run.

## Operational boundaries

- Logging, tracing, metrics, and `telemetry.Client.CaptureError` are best-effort observability; they must not turn section or persistence failures into scheduler failures.
- `Analyzer.AnalyzeSection` has no context parameter. The 10-minute context bounds context-aware work but cannot forcibly stop an in-flight analyzer call.
- Telegram long polling still requires a single daemon instance per bot token; scheduled sends reuse that instance's notifier and do not start another poller.
