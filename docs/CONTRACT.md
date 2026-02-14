# Contract (Book 3 Anchor)

`pfbackfill` starts Book 3 with one rock-solid contract:
**render a batch/backfill plan into deterministic evidence artifacts.**

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
