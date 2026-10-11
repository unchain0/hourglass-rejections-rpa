# Check now

User asks for an immediate rejection scan of their configured sections. The bot acknowledges, runs analysis (same path as the manual-check service), then sends either a clean bill of health or rejection details.

## Sub-features

- `check-empty` — configured sections, Hourglass returns no declined rows → "No rejections found…".
- `check-declined` — mock Hourglass `declined` mode on Public Witnessing → rejection notification outbound.
- `check-no-sections` — zero sections → "no sections" style message (covered indirectly; recipe always configures one section first).

## How to get to it (user POV)

- Send `/checknow` after configuring at least one section.
- Scheduled delivery is a separate daemon path (`Scheduler`); this feature is the bot button/command only.

## Driving it with verify-harness

Preconditions: launch + doctor PASS.

```sh
.cursor/skills/verify-hourglass-rpa/drive.sh "$SCRATCH" feature checknow
```

Recipe outline:

1. `/start`, `/configure`, toggle `section_Public Witnessing`, `save_config`.
2. `hourglass-mode empty` → `message /checknow` → expect processing + no-rejections text.
3. `hourglass-mode declined` → `message /checknow` → expect a second outbound payload beyond the processing line (rejection body references the section/user).

Evidence: `$EVIDENCE/checknow/empty-check-*.json`, `declined-check-*.json`.

## Gotchas

- `/checknow` runs the callback in a **goroutine**; wait for ≥2 outbound events and a short settle sleep before snapshotting.
- Without `SetCheckNowCallback` the bot would reply `check_now_unavailable`. The harness wires the real manual-check path; if you see unavailable, the harness failed to start correctly.
- Declined fixtures are only injected for notification paths containing `pubwit`. Enable **Public Witnessing** or the declined mode will still look empty.
- Do not hit production Hourglass; mode switches only the mock.
