# Command Binaries Guide

## OVERVIEW

- Keep the four `package main` entrypoints thin and orchestration-focused.
- `rpa`: configures telemetry, persistence, Hourglass, bot, and scheduler. `-once` still delegates to the unimplemented `runOnceFn` stub; do not describe it as functional.
- `setup-auth`: interactive visible-Chrome bootstrap, WebAuthn registration, token persistence, and optional confirmed SCP upload.
- `save-tokens`: interactive visible-Chrome cookie extraction and masked token summary.
- `token-refresh`: unattended token validation or renewal, controlled by refresh and auth-path environment settings.

## WHERE TO LOOK

- `rpa/main.go` owns run modes and lifecycle; `rpa/bootstrap.go` owns dependency and database wiring; `rpa/helpers.go` owns ID-list parsing.
- Each authentication utility keeps its orchestration in `main.go`; command tests live beside it.
- Shared auth, persistence, telemetry, and service behavior belongs under `src/`, not in command entrypoints.

## CONVENTIONS

- Preserve narrow interfaces and function variables around filesystem, browser, subprocess, input, SCP, token-manager, scheduler, and `runOnceFn` side effects.
- Exercise fatal paths through `runOptions.exit` (`rpa`), `setupRunner.osExit` (`setup-auth`), or package `osExit` (`save-tokens` and `token-refresh`), never real `os.Exit` in tests.
- Restore replaced package globals with `t.Cleanup`; tests mutating them must not run in parallel.
- Preserve path precedence: `rpa` config fields before environment fallbacks; token fallback is `TOKENS_PATH`, then `WEBAUTHN_TOKENS_PATH`; refresh prefers `WEBAUTHN_TOKENS_PATH`.
- Keep config/profile directories `0700` and token/credential files `0600`. Output may show paths, expiry, and masked fragments, never full secrets or auth files.
- Interactive auth uses visible Chrome with a dedicated persistent profile. In QA, set `CHROME_BIN`/`CHROME_PATH`, isolate the profile, and close competing Chrome processes.
- SCP remains opt-in through `SCPClient`; require confirmation and pass arguments directly to `exec.Command`, never through a shell.

## QA

- Run `make fmt-check` and `go test -race ./cmd/...`.
- Cover each command's success and fatal boundary through injected dependencies and exits; assert secret-safe output and no leaked global replacements.
- For auth flows, verify valid-token reuse, renewal or extraction, owner-only files, isolated-profile browser failures, and declined SCP without live services.
- Use disposable credentials, profile, and remote host for any real browser or SCP check.

## AVOID

- Do not bypass injected seams with direct process, filesystem, browser, or network side effects in code under test.
- Do not use a developer's default Chrome profile or copy auth state to shared locations; profile locks and unrelated cookies make extraction unsafe and nondeterministic.
- Do not automate SCP destinations or expose tokens, cookies, credentials, or auth-file contents in logs, errors, snapshots, or output.
