#!/usr/bin/env bash
# Tear down only the harness this launch started. Evidence is kept.
set -euo pipefail

SCRATCH="${1:-}"
if [[ -z "$SCRATCH" ]]; then
  echo "usage: cleanup.sh SCRATCH" >&2
  exit 2
fi

if [[ ! -d "$SCRATCH" ]]; then
  echo "scratch not found: $SCRATCH" >&2
  exit 1
fi

STATE="$SCRATCH/state.json"
PID_FILE="$SCRATCH/launch.pid"

kill_pid_if_ours() {
  local pid="$1"
  local reason="$2"
  if [[ -z "$pid" ]] || ! [[ "$pid" =~ ^[0-9]+$ ]]; then
    return 0
  fi
  if ! kill -0 "$pid" 2>/dev/null; then
    return 0
  fi
  # Refuse to signal a recycled pid that is not our harness binary.
  local cmd
  cmd="$(ps -p "$pid" -o args= 2>/dev/null || true)"
  if [[ "$cmd" != *verify-harness* && "$cmd" != *"$SCRATCH"* ]]; then
    echo "refusing to kill pid $pid ($reason): cmdline not harness ($cmd)" >&2
    return 1
  fi
  kill -TERM "$pid" 2>/dev/null || true
  for _ in $(seq 1 30); do
    if ! kill -0 "$pid" 2>/dev/null; then
      return 0
    fi
    sleep 0.1
  done
  # Re-check identity before SIGKILL.
  cmd="$(ps -p "$pid" -o args= 2>/dev/null || true)"
  if [[ "$cmd" == *verify-harness* || "$cmd" == *"$SCRATCH"* ]]; then
    kill -KILL "$pid" 2>/dev/null || true
  fi
}

if [[ -f "$STATE" ]]; then
  HPID="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("harness_pid",""))' "$STATE" 2>/dev/null || true)"
  kill_pid_if_ours "$HPID" "state.harness_pid" || true
fi

if [[ -f "$PID_FILE" ]]; then
  LPID="$(cat "$PID_FILE")"
  kill_pid_if_ours "$LPID" "launch.pid" || true
fi

# Scratch holds the disposable DB and binaries; evidence lives under the repo.
rm -rf "$SCRATCH"
echo "cleaned $SCRATCH (evidence retained under evidence/verify-hourglass-rpa/)"
