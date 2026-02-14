# Quick Start (PR1)

This is the smallest “reader path” through the repo.

PR1 is still **render-only**: given a `config.yaml`, we emit deterministic plan artifacts (or `error.txt`).

## Contract

### Success

`pfbackfill render` writes:

- `plan_manifest.json` — deterministic intent (runs sorted, stable JSON, trailing newline)
- `manifest.sha256` — sha256 over `plan_manifest.json`

### Expected-fail

If the config violates the contract, the command writes **only**:

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
go run ./cmd/pfbackfill render \
  --config ./fixtures/input/demo/config.yaml \
  --out ./out/render
```

Expected outputs:

```bash
ls -1 ./out/render
cat ./out/render/plan_manifest.json
cat ./out/render/manifest.sha256
```

Run an expected-fail fixture:

```bash
go run ./cmd/pfbackfill render \
  --config ./fixtures/input/bad_missing_run_field/config.yaml \
  --out ./out/bad

cat ./out/bad/error.txt
```

## Artifact review

- `plan_manifest.json` is the “plan you can diff” before any apply work exists.
- `manifest.sha256` is the minimal integrity witness for the plan.
- `error.txt` is the expected-fail lane (contract violation), and it is the **only** artifact in that lane.

## Recap

- `make verify` runs `gofmt` checks, `go test`, and the deterministic `demo`.
- `demo` recomputes every fixture and diffs byte-for-byte against `fixtures/expected/**`.
- The apply lane is **not** wired yet in PR1 (we start the skeleton later without changing the public contract).
