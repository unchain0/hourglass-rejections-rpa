#!/usr/bin/env bash
# Start an isolated verification instance: build product + harness, run mock
# Telegram/Hourglass + real bot handlers, print SCRATCH=.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$ROOT"

RUN_ID="$(date +%Y%m%dT%H%M%S)-$$"
SCRATCH="${VERIFY_SCRATCH_ROOT:-/tmp/verify-hourglass-rpa}/${RUN_ID}"
EVIDENCE="${ROOT}/evidence/verify-hourglass-rpa/${RUN_ID}"

mkdir -p "$SCRATCH" "$EVIDENCE"
chmod 700 "$SCRATCH"

export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"

echo "building product binaries into $SCRATCH" >&2
go build -o "$SCRATCH/rpa" ./cmd/rpa
go build -o "$SCRATCH/token-refresh" ./cmd/token-refresh
go build -o "$SCRATCH/save-tokens" ./cmd/save-tokens
go build -o "$SCRATCH/setup-auth" ./cmd/setup-auth
go build -o "$SCRATCH/hourglass" ./cmd/hourglass

echo "building verify harness (tags=verify)" >&2
go build -tags verify -o "$SCRATCH/verify-harness" \
  ./.cursor/skills/verify-hourglass-rpa/harness

# Never inherit production Telegram/Hourglass/DB credentials into the harness.
env -i \
  PATH="$PATH" \
  HOME="$HOME" \
  GOTOOLCHAIN="$GOTOOLCHAIN" \
  TMPDIR="${TMPDIR:-/tmp}" \
  "$SCRATCH/verify-harness" serve -scratch "$SCRATCH" -evidence "$EVIDENCE" \
  >"$SCRATCH/harness.stdout" 2>"$SCRATCH/harness.stderr" &
echo $! >"$SCRATCH/launch.pid"

# state.json is written by the harness after listeners are up.
for _ in $(seq 1 100); do
  if [[ -f "$SCRATCH/state.json" ]] && grep -q '"control_url"' "$SCRATCH/state.json"; then
    # Confirm control health answers.
    CTRL="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["control_url"])' "$SCRATCH/state.json")"
    if curl -fsS "$CTRL/health" >/dev/null 2>&1; then
      echo "SCRATCH=$SCRATCH"
      echo "EVIDENCE=$EVIDENCE"
      exit 0
    fi
  fi
  # Fail fast if the process died.
  if [[ -f "$SCRATCH/launch.pid" ]] && ! kill -0 "$(cat "$SCRATCH/launch.pid")" 2>/dev/null; then
    echo "harness exited during launch" >&2
    tail -n 50 "$SCRATCH/harness.stderr" >&2 || true
    exit 1
  fi
  sleep 0.1
done

echo "timeout waiting for harness ready" >&2
tail -n 50 "$SCRATCH/harness.stderr" >&2 || true
exit 1
