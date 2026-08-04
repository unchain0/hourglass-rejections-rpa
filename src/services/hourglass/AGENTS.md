# Hourglass Service Guide

## Boundaries

- `Client` owns authenticated Hourglass HTTP/JSON access. Keep API models within this
  package; callers receive domain results, not transport structs.
- `APIAnalyzer` owns section dispatch and API-to-domain analysis. Keep rejection
  filtering, mapping, and identity-based deduplication in `rejection_mapper.go`,
  separate from HTTP transport.
- `client.go` defines request and authentication behavior; `analyzer.go` defines the
  user cache and section-specific analysis; `models.go` defines Hourglass payloads.

## Client contracts

- The default API base is the Hourglass site plus `/api/v0.2`. `SetBaseURL` appends
  that path to a site root and preserves an explicitly supplied API path.
- Endpoint methods must use `doAuthenticatedGet`. It calls `ensureAuth` before
  creating the request, then applies the shared `setHeaders` and `setCookies` policy.
- A `401` with WebAuthn enabled forces one token renewal, updates the client tokens,
  and retries once. Preserve the original body context when renewal fails; never add
  an unbounded authentication retry.
- `EnableWebAuthn` configures the manager but does not start it. Call
  `StartTokenManager(ctx)` before authenticated requests and `StopTokenManager()` at
  shutdown; propagate startup errors.
- Never log or expose `hglogin`, XSRF tokens, WebAuthn credentials, token files, or
  complete authentication responses.

## Analyzer contracts

- `AnalyzeSection` loads users through `sync.Once`. The first load result, including
  its error, is cached for the analyzer's lifetime; do not introduce per-section
  reloads or reset one side of that state.
- `domain.AllSections` is canonical: Mechanical Parts, Field Ministry, Public
  Witnessing, and Midweek Meeting. Legacy/API and localized names remain analyzer
  input aliases only and must not enter `domain.AllSections`.
- Preserve the existing aliases: `avattendant`/`Partes Mecânicas`,
  `fsMeeting`/`Campo`, `publicWitnessing`/`Testemunho Público`, and
  `midweekMeeting`/`Reunião Meio de Semana`.
- Section failures, including unknown names and user-load failures, are returned in
  `JobResult.Error` while the Go error is nil. Do not collapse or reinterpret this
  per-section result contract.
- Only declined notifications become rejections. Deduplicate mapped results by
  section, person, title, and formatted date.

## Test seams

- Use `newHTTPTestServer` for request tests instead of real ports. Assert the shared
  auth path and the single forced-renewal retry without duplicating endpoint catalogs.
- Stub `authTokenManager` for ensure/renew/start/stop behavior. Restore any replacement
  of `newHTTPRequest` with `t.Cleanup` to avoid cross-test contamination.
