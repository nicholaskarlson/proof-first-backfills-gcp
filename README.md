# proof-first-backfills-gcp

Book 3 anchor: **batch/backfill plans you can prove**.

**Pitch:** Book 2 proves a single run. Book 3 proves many runs: a deterministic plan you can review/diff, and (later) apply safely with batch evidence.

## Quick start

Run the proof gate:

```bash
make verify
```

Then follow the reader path:

- `docs/QUICKSTART.md`

Or just run the deterministic demo:

```bash
go run ./cmd/pfbackfill demo --out ./out/demo
```

## Contract

See:

- `docs/CONTRACT.md`
- `docs/CONVENTIONS.md`
- `docs/HANDOFF.md`

Fixtures + goldens live in:

- `fixtures/input/**`
- `fixtures/expected/**`
