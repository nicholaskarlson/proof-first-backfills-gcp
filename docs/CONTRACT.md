# Contract (Book 3 Anchor)

`pfbackfill` is built in lanes.

- **Render lane:** deterministic plan artifacts you can review/diff.
- **Apply lane:** deterministic batch evidence (currently **dry-run only**).
- **Local lane:** deterministic local run-folder simulation (resumable, no cloud).

## Commands

### Render (the contract)

```bash
pfbackfill render --config ./config.yaml --out ./out/render
```

On success, `--out` contains:
- `plan_manifest.json` (deterministic intent)
- `manifest.sha256` (sha256 over `plan_manifest.json`)

On failure, `--out` contains **only**:
- `error.txt` (with trailing newline)

### Apply (dry-run)

```bash
pfbackfill apply --plan ./out/render/plan_manifest.json --out ./out/apply
```

On success, `--out` contains:
- `batch_report.json` (deterministic apply-lane evidence)
- `manifest.sha256` (sha256 over `batch_report.json`)

On failure, `--out` contains **only**:
- `error.txt` (with trailing newline)

### Verify (offline)

```bash
pfbackfill verify --plan ./out/render/plan_manifest.json --apply ./out/apply --out ./out/verify
```

On success, `--out` contains:
- `verify_report.json` (deterministic verify-lane evidence)
- `manifest.sha256` (sha256 over `verify_report.json`)

Notes:
- `verify` fails fast if `apply/manifest.sha256` doesn’t match `batch_report.json` (tamper/corruption defense).

On failure, `--out` contains **only**:
- `error.txt` (with trailing newline)

### Local (execution lane, no cloud)

```bash
pfbackfill local --plan ./out/render/plan_manifest.json --out ./out/local
```

Notes:
- `local` requires a sibling `manifest.sha256` next to `plan_manifest.json` (the plan witness).
- If `runs/<run_id>/done.json` exists, it must match the plan witness (`plan_sha256`) or the local lane fails.
- `run_id` must be a safe path segment: it must match `^[A-Za-z0-9][A-Za-z0-9_-]*$`.

Safety notes:
- `run_id` and object keys (e.g., `left` / `right`) are validated as safe path segments.
- Plan consumers (`apply`, `verify`, `local`) reject missing/empty required plan metadata fields (project_id, region, input_bucket, output_bucket, service_name).
- Plan consumers (`apply`, `verify`, `local`) re-validate `left`/`right` as object keys (defense-in-depth for hand-edited plans).
- Plan consumers (`apply`, `verify`, `local`) reject duplicate `run_id` values in `runs` (defense-in-depth for hand-edited plans).
- Plan consumers (`apply`, `verify`, `local`) reject empty `runs[]` (defense-in-depth for hand-edited plans).
- Plan manifest decoding is strict: unknown `plan_manifest.json` fields are rejected.
- Config decoding is strict: unknown `config.yaml` fields are rejected.

On success, `--out` contains:
- `local_report.json` (deterministic local execution evidence)
- `local_diff.json` (deterministic per-run create/skip witness for drift explanations)
- `manifest.sha256` (sha256 over **all** files under `--out`, excluding `manifest.sha256` itself)
- `runs/<run_id>/run_meta.json` (per-run deterministic metadata)
- `runs/<run_id>/done.json` (resumability marker; if present, the run is skipped)

On failure, `--out` contains **only**:
- `error.txt` (with trailing newline)



### Pack (portable handoff kit)

```bash
pfbackfill pack --plan ./out/render/plan_manifest.json --apply ./out/apply --verify ./out/verify --local ./out/local --out ./out/pack
```

On success, `--out` contains:
- `pack_manifest.json` (deterministic index)
- `manifest.sha256` (sha256 over **all** files under `--out`, excluding `manifest.sha256` itself)
- `plan/*`, `apply/*`, `verify/*`, `local/*` (portable copies of lane receipts)

Notes:
- `pack` verifies each lane manifest entry it depends on (e.g., `apply/manifest.sha256` must match `batch_report.json`).
- `pack` is offline-only: it does not re-run lanes.

On failure, `--out` contains **only**:
- `error.txt` (with trailing newline)

### Diff (drift between two packs)

```bash
pfbackfill diff --a ./packA --b ./packB --out ./out/diff
```

On success, `--out` contains:
- `drift_report.json` (deterministic added/removed/changed evidence paths)
- `manifest.sha256` (sha256 over `drift_report.json`)

Notes:
- `diff` verifies each pack’s `manifest.sha256` against the pack bytes before computing drift.
- Drift compares evidence files only (excludes `pack_manifest.json` and all `*/manifest.sha256` receipts).

On failure, `--out` contains **only**:
- `error.txt` (with trailing newline)

### Demo (proof gate)

```bash
pfbackfill demo --out ./out/demo
```

Recomputes every fixture case and byte-compares to `fixtures/expected/**`.

Notes:
- `demo --out` is cleared at the start of the run (no dependence on prior output).
- For successful render fixtures, apply-lane artifacts are written under `out/<case>/apply/`.
- For successful verify fixtures, local-lane artifacts are written under `out/<case>/local/`.
- If a fixture provides `fixtures/input/<case>/seed/**`, demo copies it into `out/<case>/` before running the local lane.
- For apply-only fixtures (those that provide `plan_manifest.json` under `fixtures/input/<case>/`),
  demo runs `apply` against that plan to exercise apply expected-fail cases without changing render.

- For local-only fixtures (those that also provide `fixtures/input/<case>/local_only`),
  demo runs `local` against that plan to exercise local expected-fail behavior without changing apply/verify.


## Config schema (MVP)

Required:
- `project_id` (string)
- `region` (string)
- `input_bucket` (string)
- `output_bucket` (string)
- `service_name` (string)
- `runs` (list):
  - `run_id` (string; must be unique within `runs`)
  - `left` (string)
  - `right` (string)
Notes:
- Unknown fields in `config.yaml` are rejected (strict decode).
- `left` and `right` are object-key style paths: relative, forward slashes (`/`) only (no backslashes), and **no** `.` or `..` segments.


## Safety

Commands that write `--out`:
- `render`, `apply`, `verify`, and `demo` clear the directory first
- `local` is **resumable** and does not clear `--out` (it only creates missing run folders)
- If `local/manifest.sha256` already exists (resume), it must include an entry for `local_diff.json` (diff witness). Missing entry is a deterministic hard error.
- refuses unsafe paths (`.`, `..`, `/`, volume roots)
