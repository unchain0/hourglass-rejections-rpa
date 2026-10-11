One-time `rpa -once` execution.

## Sub-features
- `once-run`: CLI completes and exits.

## How to get to it (user POV)
- Build binary; invoke `./rpa -once`.

## Driving it with CLI
Preconditions: `make build` produced `rpa` binary.
- Run `./rpa -once`
- Observable: exit code and stdout captured.

## Gotchas
- `-once` currently delegates to a stub; proof is exit code/stdout, not full feature.
