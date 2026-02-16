package apply

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nicholaskarlson/proof-first-backfills-gcp/internal/model"
	"github.com/nicholaskarlson/proof-first-backfills-gcp/internal/pfutil"
)

type RunReport struct {
	RunID  string `json:"run_id"`
	Left   string `json:"left"`
	Right  string `json:"right"`
	Status string `json:"status"`
}

type BatchReport struct {
	Mode         string      `json:"mode"`
	PlanSHA256   string      `json:"plan_sha256"`
	ProjectID    string      `json:"project_id"`
	Region       string      `json:"region"`
	ServiceName  string      `json:"service_name"`
	InputBucket  string      `json:"input_bucket"`
	OutputBucket string      `json:"output_bucket"`
	Runs         []RunReport `json:"runs"`
}

func verifyPlanManifest(planPath string) (string, error) {
	sum, err := pfutil.Sha256File(planPath)
	if err != nil {
		return "", err
	}

	planDir := filepath.Dir(planPath)
	shaPath := filepath.Join(planDir, "manifest.sha256")

	b, err := os.ReadFile(shaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("missing plan manifest.sha256")
		}
		return "", err
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
		if parts[0] != sum {
			return "", fmt.Errorf("plan manifest sha mismatch")
		}
	}

	if !found {
		return "", fmt.Errorf("plan manifest.sha256 missing entry: %s", wantFile)
	}

	return sum, nil
}

// ApplyDryRun consumes a rendered plan and writes deterministic “apply lane” artifacts.
// PR2 keeps apply as dry-run only: no network calls, no timestamps, no non-determinism.
func ApplyDryRun(planPath, outDir string) error {
	if err := pfutil.ResetOutDir(outDir); err != nil {
		return err
	}

	planSum, err := verifyPlanManifest(planPath)
	if err != nil {
		return err
	}

	raw, err := os.ReadFile(planPath)
	if err != nil {
		return err
	}

	var plan model.PlanManifest
	if err := pfutil.UnmarshalPlanManifestStrict(raw, &plan); err != nil {
		return fmt.Errorf("invalid plan json: %s", err.Error())
	}
	if err := pfutil.ValidatePlanManifest(&plan); err != nil {
		return err
	}

	runs := make([]RunReport, 0, len(plan.Runs))
	for _, r := range plan.Runs {
		runs = append(runs, RunReport{
			RunID:  r.RunID,
			Left:   r.Left,
			Right:  r.Right,
			Status: "DRY_RUN",
		})
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].RunID < runs[j].RunID })

	report := BatchReport{
		Mode:         "dry_run",
		PlanSHA256:   planSum,
		ProjectID:    plan.ProjectID,
		Region:       plan.Region,
		ServiceName:  plan.ServiceName,
		InputBucket:  plan.InputBucket,
		OutputBucket: plan.OutputBucket,
		Runs:         runs,
	}

	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}

	if err := pfutil.WriteText(filepath.Join(outDir, "batch_report.json"), string(b)+"\n"); err != nil {
		return err
	}

	return pfutil.WriteShaManifest(outDir, []string{"batch_report.json"})
}
