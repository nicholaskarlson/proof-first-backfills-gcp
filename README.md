# proof-first-backfills-gcp

Book 3 anchor: **batch/backfill plans you can prove**.

**Pitch:** Book 2 proves a single run. Book 3 proves many runs: a deterministic plan you can review/diff, and an apply lane you can verify (currently dry-run only).

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
- `pfbackfill apply` — apply lane (dry-run) deterministic batch evidence
- `pfbackfill verify` — verify lane (PR4 offline) deterministic verification evidence
- `pfbackfill local` — local execution lane (PR5 no cloud) deterministic run-folder simulation (requires plan witness)
- `pfbackfill cloud-plan` — offline cloud planning artifact (name mappings + ordering + marker keys)
- `pfbackfill demo` — recompute fixtures and diff goldens

Docs:

- `docs/CONTRACT.md`
- `docs/CONVENTIONS.md`
- `docs/HANDOFF.md`

Fixtures + goldens live in:

- `fixtures/input/**`
- `fixtures/expected/**`

Book 4 fixtures:

- (Ch03) `case01_plan_manifest_smoke` — render plan_manifest.json + manifest.sha256 (smoke)
- (Ch07) `case02_cloud_plan_consulting_smoke` — emit cloud/cloud_plan.json + manifest.sha256 (offline cloud planning; names-only review)

- (Ch08) `case03_apply_smoke_consulting` — apply dry-run and emit apply/batch_report.json + manifest.sha256 (receipt); demo continues with verify/local lanes

- (Ch09) `case04_verify_smoke_consulting` — seed apply outputs and emit verify/verify_report.json + manifest.sha256 (receipt); demo continues with local lane
- (Ch09) `case05_verify_apply_manifest_mismatch_expected_fail` — expected-fail verify lane (apply/manifest.sha256 mismatch) emits verify/error.txt

- (Ch10) `case06_pack_smoke_consulting` — build a portable pack/ folder (pack_manifest.json + manifest.sha256 + lane receipts)

