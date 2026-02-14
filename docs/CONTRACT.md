# Contract (Book 3 Anchor)

`pfbackfill` is built in lanes.

- **Render lane:** deterministic plan artifacts you can review/diff.
- **Apply lane:** deterministic batch evidence (PR2 is **dry-run only**).

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

### Apply (PR2: dry-run)

```bash
pfbackfill apply --plan ./out/render/plan_manifest.json --out ./out/apply
```

On success, `--out` contains:
- `batch_report.json` (deterministic apply-lane evidence)
- `manifest.sha256` (sha256 over `batch_report.json`)

On failure, `--out` contains **only**:
- `error.txt` (with trailing newline)

### Demo (proof gate)

```bash
pfbackfill demo --out ./out/demo
```

Recomputes every fixture case and byte-compares to `fixtures/expected/**`.

## Config schema (MVP)

Required:
- `project_id` (string)
- `region` (string)
- `input_bucket` (string)
- `output_bucket` (string)
- `service_name` (string)
- `runs` (list):
  - `run_id` (string)
  - `left` (string)
  - `right` (string)

## Safety

Any command that writes `--out`:
- clears the directory first
- refuses unsafe paths (`.`, `..`, `/`, volume roots)
