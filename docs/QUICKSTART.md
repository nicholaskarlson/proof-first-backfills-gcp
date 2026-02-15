# Quick Start (PR6)

This is the smallest “reader path” through the repo.

PR5 adds the **local lane** (run-folder simulation). PR6 tightens safety + requires a plan witness. The system is still deterministic: no network calls, no timestamps, no non-determinism.

## Contract

### Render (plan)

`pfbackfill render` writes:

- `plan_manifest.json` — deterministic intent (runs sorted, stable JSON, trailing newline)
- `manifest.sha256` — sha256 over `plan_manifest.json`

### Apply (dry-run)

`pfbackfill apply` writes:

- `batch_report.json` — deterministic apply-lane evidence
- `manifest.sha256` — sha256 over `batch_report.json`

### Verify (offline)

`pfbackfill verify` writes:

- `verify_report.json` — deterministic verify-lane evidence
- `manifest.sha256` — sha256 over `verify_report.json`

### Local (no cloud, resumable)

`pfbackfill local` writes:

- `local_report.json` — deterministic local execution evidence
- `manifest.sha256` — sha256 over all files under the local lane
- `runs/<run_id>/run_meta.json` — per-run metadata
- `runs/<run_id>/done.json` — resumability marker

Notes:
- `local` does **not** clear `--out`; existing `done.json` files are left untouched.
- `local` requires a sibling `manifest.sha256` next to `plan_manifest.json` (the plan witness).

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

Then verify the plan + apply artifacts:

```bash
go run ./cmd/pfbackfill verify   --plan ./out/render/plan_manifest.json   --apply ./out/apply   --out ./out/verify
```

Then run the local execution lane (no cloud):

```bash
go run ./cmd/pfbackfill local   --plan ./out/render/plan_manifest.json   --out ./out/local
```

Expected outputs:

```bash
ls -1 ./out/render
ls -1 ./out/apply
ls -1 ./out/verify
ls -1 ./out/local
```

Run an expected-fail fixture:

```bash
go run ./cmd/pfbackfill render   --config ./fixtures/input/bad_missing_run_field/config.yaml   --out ./out/bad

cat ./out/bad/error.txt
```

## Artifact review

- `plan_manifest.json` is the “plan you can diff” before any real apply work exists.
- `batch_report.json` is the “apply lane witness” (PR2 dry-run), proving that apply can be deterministic too.
- `local_report.json` + `runs/**` are the “run-folder simulation” artifacts (resumable and deterministic).
- If `local/manifest.sha256` already exists, it must include `local_diff.json` (diff witness).
- `manifest.sha256` is the minimal integrity witness for each lane’s primary artifact.
- `error.txt` is the expected-fail lane (contract violation), and it is the **only** artifact in that lane.

## Recap

- `make verify` runs `gofmt` checks, `go test`, and the deterministic `demo`.
- `demo` recomputes every fixture and diffs byte-for-byte against `fixtures/expected/**`.
