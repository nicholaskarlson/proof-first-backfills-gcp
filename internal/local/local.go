package local

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nicholaskarlson/proof-first-backfills-gcp/internal/model"
	"github.com/nicholaskarlson/proof-first-backfills-gcp/internal/pfutil"
)

func verifyPlanManifest(planPath, planSha string) error {
	planDir := filepath.Dir(planPath)
	shaPath := filepath.Join(planDir, "manifest.sha256")

	b, err := os.ReadFile(shaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("missing plan manifest.sha256")
		}
		return err
	}

	wantFile := filepath.Base(planPath)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	found := false
	for _, line := range lines {
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 {
			continue
		}
		if parts[1] != wantFile {
			continue
		}
		found = true
		if parts[0] != planSha {
			return fmt.Errorf("plan manifest sha mismatch")
		}
	}
	if !found {
		return fmt.Errorf("plan manifest.sha256 missing entry: %s", wantFile)
	}

	return nil
}

type runMeta struct {
	RunID      string `json:"run_id"`
	Left       string `json:"left"`
	Right      string `json:"right"`
	PlanSHA256 string `json:"plan_sha256"`
}

type doneMarker struct {
	RunID      string `json:"run_id"`
	PlanSHA256 string `json:"plan_sha256"`
	Status     string `json:"status"`
}

type runReport struct {
	RunID  string `json:"run_id"`
	Action string `json:"action"`
}

type localReport struct {
	Mode       string      `json:"mode"`
	PlanSHA256 string      `json:"plan_sha256"`
	RunCount   int         `json:"run_count"`
	Created    int         `json:"created"`
	Skipped    int         `json:"skipped"`
	Runs       []runReport `json:"runs"`
}

// Exec simulates deterministic run-folder production locally (no cloud).
// It is resumable: if a run already has runs/<run_id>/done.json, it is skipped and left untouched.
func Exec(planPath, outDir string) error {
	if pfutil.IsUnsafeOutDir(outDir) {
		return fmt.Errorf("refusing unsafe --out: %q", outDir)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	planBytes, err := os.ReadFile(planPath)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(planBytes)
	planSha := hex.EncodeToString(sum[:])
	if err := verifyPlanManifest(planPath, planSha); err != nil {
		return err
	}

	var plan model.PlanManifest
	if err := json.Unmarshal(planBytes, &plan); err != nil {
		return fmt.Errorf("invalid plan_manifest.json: %w", err)
	}

	// Deterministic: process runs in sorted order by run_id.
	runs := make([]model.PlanRun, 0, len(plan.Runs))
	runs = append(runs, plan.Runs...)
	sort.Slice(runs, func(i, j int) bool { return runs[i].RunID < runs[j].RunID })

	created := 0
	skipped := 0
	rr := make([]runReport, 0, len(runs))

	for _, r := range runs {
		runDir := filepath.Join(outDir, "runs", r.RunID)
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			return err
		}

		donePath := filepath.Join(runDir, "done.json")
		if _, err := os.Stat(donePath); err == nil {
			// Resume: ensure done marker matches this plan.
			b, err := os.ReadFile(donePath)
			if err != nil {
				return err
			}
			var dm doneMarker
			if err := json.Unmarshal(b, &dm); err != nil {
				return fmt.Errorf("invalid %s: %w", filepath.ToSlash(filepath.Join("runs", r.RunID, "done.json")), err)
			}
			if dm.PlanSHA256 != planSha {
				return fmt.Errorf("runs/%s/done.json plan_sha256 mismatch", r.RunID)
			}

			skipped++
			rr = append(rr, runReport{RunID: r.RunID, Action: "SKIPPED"})
			continue
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}

		// Create minimal deterministic run-folder artifacts.
		meta := runMeta{
			RunID:      r.RunID,
			Left:       r.Left,
			Right:      r.Right,
			PlanSHA256: planSha,
		}
		mb, err := json.MarshalIndent(meta, "", "  ")
		if err != nil {
			return err
		}
		if err := pfutil.WriteText(filepath.Join(runDir, "run_meta.json"), string(mb)+"\n"); err != nil {
			return err
		}

		dm := doneMarker{
			RunID:      r.RunID,
			PlanSHA256: planSha,
			Status:     "DONE",
		}
		db, err := json.MarshalIndent(dm, "", "  ")
		if err != nil {
			return err
		}
		if err := pfutil.WriteText(donePath, string(db)+"\n"); err != nil {
			return err
		}

		created++
		rr = append(rr, runReport{RunID: r.RunID, Action: "CREATED"})
	}

	report := localReport{
		Mode:       "local",
		PlanSHA256: planSha,
		RunCount:   len(runs),
		Created:    created,
		Skipped:    skipped,
		Runs:       rr,
	}
	rb, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := pfutil.WriteText(filepath.Join(outDir, "local_report.json"), string(rb)+"\n"); err != nil {
		return err
	}

	// Deterministic lane manifest over everything under outDir (excluding manifest itself).
	files, err := pfutil.ListFiles(outDir)
	if err != nil {
		return err
	}
	filtered := make([]string, 0, len(files))
	for _, f := range files {
		if f == "manifest.sha256" {
			continue
		}
		filtered = append(filtered, f)
	}
	if err := pfutil.WriteShaManifest(outDir, filtered); err != nil {
		return err
	}

	return nil
}
