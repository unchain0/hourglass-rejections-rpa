---
name: verify-hourglass-rejections-rpa
description: "Verify the Go RPA build, tests, and intentionally unimplemented once-mode CLI with isolated runtime state and external evidence."
---

# Launch
Run `make build`. Full mode starts the bot and scheduler, so do not drive it here.

# Doctor
Run `go version`, confirm `cmd/rpa/main.go` exists, then require `make build` to exit zero.

# Drive
Verify once-mode execution from `cmd/rpa/main.go` without using real credentials or persistent application state. The known result is exit status `1` and the error marker `run failed: runOnce not implemented`.

```sh
set -euo pipefail
EVIDENCE=/tmp/verify-hourglass-rejections-rpa-evidence
mkdir -p "$EVIDENCE"
SANDBOX=$(mktemp -d)
trap 'rm -rf "$SANDBOX"' EXIT
XSRF=$(od -An -N 32 -tx1 /dev/urandom | tr -d ' \n')
HGLOGIN=$(od -An -N 32 -tx1 /dev/urandom | tr -d ' \n')

set +e
HOME="$SANDBOX" AUTO_REFRESH_TOKENS=false HOURGLASS_XSRF_TOKEN="$XSRF" HOURGLASS_HGLOGIN_COOKIE="$HGLOGIN" \
  ./rpa -once >"$EVIDENCE/once-proof.txt" 2>&1
status=$?
set -e
printf 'exit_status=%d\n' "$status" >>"$EVIDENCE/once-proof.txt"
test "$status" -eq 1
grep -Fq 'run failed: runOnce not implemented' "$EVIDENCE/once-proof.txt"
```

Do not accept a configuration, database, or authentication startup error as proof of once-mode behavior.

# Evidence
Keep proof files in `/tmp/verify-hourglass-rejections-rpa-evidence`; do not add them to git.

# Cleanup
The command removes its scratch directory and leaves evidence. It starts no long-lived process.

# Helpers
`make build`, `make test-short`, and `./rpa -once`
