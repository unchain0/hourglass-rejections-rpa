# Preferences Persistence Guide

## BOUNDARY

- `PreferenceStore` is the narrow persistence contract used by callers.
- `PreferenceManager` owns get-or-create and preference mutations without exposing
  GORM. `RecordDiscoveredChat` intentionally uses a concrete `*Store` assertion.
- `Store` is the GORM implementation for SQLite and Postgres. Keep this layering and
  update interface mocks and callers together when the contract changes.

## WHERE TO LOOK

- `store.go`: models, CRUD, audit writes, retention cleanup, and execution/rejection
  persistence.
- `manager.go`: caller-facing preference behavior.
- `factory.go`: environment parsing, database opening, SQLite setup, and migrations.
- `cmd/rpa/bootstrap.go`: application-level `DATABASE_URL` mapping.

## INVARIANTS

- Every store construction path must migrate all five models: `UserPreference`,
  `JobExecution`, `AuditLog`, `DiscoveredChat`, and `RejectionLog`. Preserve their
  indexes, uniqueness, nullability, and column tags.
- Before migration, SQLite enables `foreign_keys`, WAL journaling,
  `synchronous = NORMAL`, and `busy_timeout = 5000`. PRAGMA failures must identify
  the failing setting.
- File-backed SQLite uses owner-only directories (`0700`) and database files (`0600`).
  In-memory databases remain exempt from file permission changes.
- The factory reads `DB_TYPE` (default `sqlite`) and `DB_PATH`, or Postgres `DB_HOST`,
  `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, and `DB_SSLMODE`; an explicit DSN
  takes precedence. Bootstrap separately maps `DATABASE_URL` to a Postgres DSN.
- Saving a preference sets a 90-day `DataRetention` deadline only when it is unset.
  `CleanupExpiredData` deletes expired `UserPreference` rows only; it does not purge
  audit, execution, discovered-chat, or rejection history.
- Preference reads, writes, deletes, and lists emit best-effort `AuditLog` rows with
  explicit actions and purposes. Do not make audit-write failure fatal without
  reviewing caller-visible error behavior and auditability requirements.
- Telegram chat IDs and usernames are PII. Never log them, database passwords, or
  complete DSNs; do not weaken storage permissions or ignore migration failures.

## TESTING

- Run `go test ./src/integrations/database/preferences`; add `-race` for shared-store
  or concurrency changes.
- Use `PreferenceStore` mocks for manager behavior and temporary or in-memory SQLite
  for persistence behavior.
- Preserve the focused factory seams for database opening, migration, PRAGMAs, and
  permissions so failures can be tested without a live Postgres instance.
- Assert persisted data plus relevant retention, audit, PRAGMA, and permission effects.
