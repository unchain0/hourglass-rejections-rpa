One-time `rpa -once` execution.

## Sub-features
- `once-run`: CLI completes and exits.

## How to get to it (user POV)
- Build binary; invoke `./rpa -once`.

## Driving it with CLI
Precondition: `make build` produced `rpa`.

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

## Expected result
- `once-proof.txt` contains `exit_status=1`.
- It contains `run failed: runOnce not implemented`.

## Gotchas
- `-once` currently delegates to a stub. Any configuration, database, or authentication error is a failed verification, not proof of the stub.
