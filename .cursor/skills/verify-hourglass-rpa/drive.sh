#!/usr/bin/env bash
# Drive the bot the way a user would: inject Telegram commands/callbacks
# into the live harness and assert outbound messages + SQLite side effects.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$ROOT"

SCRATCH="${1:-}"
shift || true
if [[ -z "$SCRATCH" ]]; then
  echo "usage: drive.sh SCRATCH <subcommand> [args...]" >&2
  echo "  feature start-help|configure-preferences|checknow|language" >&2
  echo "  message TEXT | callback DATA | outbound | preferences | clear" >&2
  exit 2
fi

if [[ ! -x "$SCRATCH/verify-harness" ]]; then
  echo "missing $SCRATCH/verify-harness — run launch.sh first" >&2
  exit 1
fi

exec "$SCRATCH/verify-harness" drive -scratch "$SCRATCH" "$@"
