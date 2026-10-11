#!/usr/bin/env bash
# Read-only health check for a launched verification instance.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$ROOT"

SCRATCH="${1:-}"
if [[ -z "$SCRATCH" ]]; then
  echo "usage: doctor.sh SCRATCH" >&2
  exit 2
fi

if [[ ! -x "$SCRATCH/verify-harness" ]]; then
  echo "missing $SCRATCH/verify-harness — run launch.sh first" >&2
  exit 1
fi

exec "$SCRATCH/verify-harness" doctor -scratch "$SCRATCH"
