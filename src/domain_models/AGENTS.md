# Domain Models Guide

## SCOPE

- Package declaration: `domain`; directory/import path: `src/domain_models`.
- Shared, transport-neutral contracts only. Keep service, persistence, HTTP, and
  notification implementation details out.

## CONTRACTS

- `AllSections` is canonical. Preserve this exact spelling and order:
  `Mechanical Parts`, `Field Ministry`, `Public Witnessing`, `Midweek Meeting`.
- Analyzer aliases, legacy/API names, and localized labels are accepted inputs, not
  additional `AllSections` values.
- Preserve `Rejection` field meaning and JSON compatibility:
  `Section`/`section`, `Who`/`who`, `What`/`what`, `When`/`when`, and
  `Timestamp`/`timestamp`.
- `JobResult.Error` records a per-section analysis failure. `AnalyzeSection` may also
  return an error; do not merge, suppress, or reinterpret either channel.
- `Scraper`, `Storage`, and `Notifier` are cross-package seams. Signature or method-set
  changes require coordinated updates to every implementation and mock.
- Sentinel error identity is part of the contract. Wrap with `%w`, inspect with
  `errors.Is`, and preserve existing sentinel values and messages.

## CHANGE DISCIPLINE

- Search all callers before changing exported structs, interfaces, `AllSections`, JSON
  tags, or sentinels; edits here have broad compile-time and compatibility impact.
- Add fields or tags only when they express a domain contract, not a transport concern.
- Keep aliases out of `AllSections`; normalize them at the analyzer boundary.
- Run `go test ./src/domain_models` after edits.
