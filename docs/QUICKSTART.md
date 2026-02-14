# Quick Start (PR2)

This is the smallest “reader path” through the repo.

PR2 adds the **apply lane** (dry-run only). The system is still deterministic: no network calls, no timestamps, no non-determinism.

## Contract

### Render (plan)

`pfbackfill render` writes:

- `plan_manifest.json` — deterministic intent (runs sorted, stable JSON, trailing newline)
- `manifest.sha256` — sha256 over `plan_manifest.json`

### Apply (dry-run)

`pfbackfill apply` writes:

- `batch_report.json` — deterministic apply-lane evidence
- `manifest.sha256` — sha256 over `batch_report.json`

### Expected-fail

If inputs violate the contract, the command writes **only**:

- `error.txt` — stable error message + trailing newline

## Command block

Clone and run the proof gate:

```bash
git clone https://github.com/nicholaskarlson/proof-first-backfills-gcp
cd proof-first-backfills-gcp
make verify
```

Render the demo fixture:

```bash
go run ./cmd/pfbackfill render   --config ./fixtures/input/demo/config.yaml   --out ./out/render
```

Then apply the rendered plan (dry-run):

```bash
go run ./cmd/pfbackfill apply   --plan ./out/render/plan_manifest.json   --out ./out/apply
```

Expected outputs:

```bash
ls -1 ./out/render
ls -1 ./out/apply
```

Run an expected-fail fixture:

```bash
go run ./cmd/pfbackfill render   --config ./fixtures/input/bad_missing_run_field/config.yaml   --out ./out/bad

cat ./out/bad/error.txt
```

## Artifact review

- `plan_manifest.json` is the “plan you can diff” before any real apply work exists.
- `batch_report.json` is the “apply lane witness” (PR2 dry-run), proving that apply can be deterministic too.
- `manifest.sha256` is the minimal integrity witness for each lane’s primary artifact.
- `error.txt` is the expected-fail lane (contract violation), and it is the **only** artifact in that lane.

## Recap

- `make verify` runs `gofmt` checks, `go test`, and the deterministic `demo`.
- `demo` recomputes every fixture and diffs byte-for-byte against `fixtures/expected/**`.
