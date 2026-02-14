# Conventions

This repo exists to be *provable*: deterministic artifacts + fixtures + goldens + a verification gate.

## Line endings
- **LF only** (enforced via `.gitattributes`).

## Determinism rules
Outputs must be deterministic:
- stable ordering (sort runs by `run_id`)
- stable formatting (pretty JSON + trailing newline)
- no timestamps, UUIDs, random IDs, or environment-specific paths

## Atomic writes
All writes are temp file → rename so partial files never appear.

## Expected-fail
Expected-fail cases emit **only**:
- `error.txt` (must end with a newline)

The proof gate recomputes outputs and compares them byte-for-byte against `fixtures/expected/**`.
