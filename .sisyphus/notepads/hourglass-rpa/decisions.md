## Domain Layer Decisions
- Included `Cookie` struct in `types.go` to support `Storage` interface requirements.
- Used standard Go documentation comments for exported symbols.
### JSON/CSV Storage Implementation
- Decided to use a global variable for json.MarshalIndent to allow mocking in tests without changing the public API.
- Decided to split saveCSV into saveCSV (file handling) and writeCSV (logic) for better testability.

- 2026-03-03: Kept backward compatibility by preserving existing NewTokenManager signature and adding BrowserAuth through TokenManagerOption (WithBrowserAuth).
