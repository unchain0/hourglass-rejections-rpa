---
name: verify-hourglass-rejections-rpa
description: "Verification skill for hourglass-rejections-rpa (Go daemon + Telegram bot + scheduler). Drives CLI and service behavior; no hardcoded secrets; evidence kept outside git at /tmp/verify-hourglass-rejections-rpa-evidence."
---

# Launch
`make build` then `./rpa -once` (short-lived CLI drive). Full mode starts bot + scheduler; not driven here to avoid shared-state conflicts.

# Doctor
`ls cmd/rpa/main.go` and check `go version`; require build exits 0.

# Drive
One feature: once-mode execution (`cmd/rpa/main.go` `-once`). Run `./rpa -once`, capture stdout/exit to `/tmp/verify-hourglass-rejections-rpa-evidence/once-proof.txt`.

# Evidence
Directory `/tmp/verify-hourglass-rejections-rpa-evidence`; proof files named by feature.

# Cleanup
Kill nothing (short-lived); remove scratch dirs only; leave evidence.

# Helpers
`make build`, `./rpa -once`
