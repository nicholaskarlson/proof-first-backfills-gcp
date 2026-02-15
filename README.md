# proof-first-backfills-gcp

Book 3 anchor: **batch/backfill plans you can prove**.

**Pitch:** Book 2 proves a single run. Book 3 proves many runs: a deterministic plan you can review/diff, and an apply lane you can verify (PR2 is dry-run only).

## Quick start

Run the proof gate:

```bash
make verify
```

Then follow the reader path:

- `docs/QUICKSTART.md`

## Contract

Commands:

- `pfbackfill render` — render deterministic plan evidence
- `pfbackfill apply` — apply lane (PR2 dry-run) deterministic batch evidence
- `pfbackfill verify` — verify lane (PR4 offline) deterministic verification evidence
- `pfbackfill demo` — recompute fixtures and diff goldens

Docs:

- `docs/CONTRACT.md`
- `docs/CONVENTIONS.md`
- `docs/HANDOFF.md`

Fixtures + goldens live in:

- `fixtures/input/**`
- `fixtures/expected/**`
