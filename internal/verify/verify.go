package verify

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

type RunStatus struct {
	RunID  string `json:"run_id"`
	Status string `json:"status"`
}

type Check struct {
	Name string `json:"name"`
	OK   bool   `json:"ok"`
}

type VerifyReport struct {
	Mode              string      `json:"mode"`
	OK                bool        `json:"ok"`
	PlanSHA256        string      `json:"plan_sha256"`
	BatchReportSHA256 string      `json:"batch_report_sha256"`
	RunCount          int         `json:"run_count"`
	Runs              []RunStatus `json:"runs"`
	Checks            []Check     `json:"checks"`
}

type batchRun struct {
	RunID  string `json:"run_id"`
	Left   string `json:"left"`
	Right  string `json:"right"`
	Status string `json:"status"`
}

type batchReport struct {
	Mode         string     `json:"mode"`
	PlanSHA256   string     `json:"plan_sha256"`
	ProjectID    string     `json:"project_id"`
	Region       string     `json:"region"`
	ServiceName  string     `json:"service_name"`
	InputBucket  string     `json:"input_bucket"`
	OutputBucket string     `json:"output_bucket"`
	Runs         []batchRun `json:"runs"`
}

func verifyShaEntry(dir, targetFile, kind string) (string, error) {
	sum, err := pfutil.Sha256File(filepath.Join(dir, targetFile))
	if err != nil {
		return "", err
	}
	shaPath := filepath.Join(dir, "manifest.sha256")
	b, err := os.ReadFile(shaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("missing %s manifest.sha256", kind)
		}
		return "", err
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	found := false
	for _, line := range lines {
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 {
			continue
		}
		if parts[1] != targetFile {
			continue
		}
		found = true
		if parts[0] != sum {
			return "", fmt.Errorf("%s manifest sha mismatch", kind)
		}
	}
	if !found {
		return "", fmt.Errorf("%s manifest.sha256 missing entry: %s", kind, targetFile)
	}
	return sum, nil
}

// Verify consumes render + apply artifacts and writes deterministic verify-lane evidence.
// PR4 keeps verify fully offline: hashes + structural checks only.
func Verify(planPath, applyDir, outDir string) error {
	if err := pfutil.ResetOutDir(outDir); err != nil {
		return err
	}

	planDir := filepath.Dir(planPath)
	planSum, err := verifyShaEntry(planDir, filepath.Base(planPath), "plan")
	if err != nil {
		return err
	}

	batchPath := filepath.Join(applyDir, "batch_report.json")
	batchSum, err := verifyShaEntry(applyDir, "batch_report.json", "apply")
	if err != nil {
		return err
	}

	rawPlan, err := os.ReadFile(planPath)
	if err != nil {
		return err
	}
	var plan model.PlanManifest
	if err := json.Unmarshal(rawPlan, &plan); err != nil {
		return fmt.Errorf("invalid plan json: %s", err.Error())
	}
	if len(plan.Runs) == 0 {
		return fmt.Errorf("runs[] must not be empty")
	}

	rawBatch, err := os.ReadFile(batchPath)
	if err != nil {
		return err
	}
	var batch batchReport
	if err := json.Unmarshal(rawBatch, &batch); err != nil {
		return fmt.Errorf("invalid batch json: %s", err.Error())
	}

	// Deterministic checks (no timestamps, no env-dependent paths).
	if batch.PlanSHA256 != planSum {
		return fmt.Errorf("batch_report plan_sha256 mismatch")
	}
	if batch.ProjectID != plan.ProjectID || batch.Region != plan.Region || batch.ServiceName != plan.ServiceName ||
		batch.InputBucket != plan.InputBucket || batch.OutputBucket != plan.OutputBucket {
		return fmt.Errorf("batch_report metadata mismatch")
	}

	planRuns := make(map[string]model.PlanRun, len(plan.Runs))
	seen := map[string]int{}
	for i, r := range plan.Runs {
		if err := pfutil.ValidateRunID(r.RunID); err != nil {
			return fmt.Errorf("runs[%d] invalid run_id: %s", i, err.Error())
		}
		if prev, ok := seen[r.RunID]; ok {
			return fmt.Errorf("runs[%d] duplicate run_id: %s (already in runs[%d])", i, r.RunID, prev)
		}
		seen[r.RunID] = i
		if err := pfutil.ValidateObjectKey(r.Left); err != nil {
			return fmt.Errorf("runs[%d] invalid left: %s", i, err.Error())
		}
		if err := pfutil.ValidateObjectKey(r.Right); err != nil {
			return fmt.Errorf("runs[%d] invalid right: %s", i, err.Error())
		}
		planRuns[r.RunID] = r
	}

	if len(batch.Runs) != len(plan.Runs) {
		return fmt.Errorf("batch_report runs mismatch")
	}

	// Deterministic: sort batch runs by run_id for stable report output.
	sort.Slice(batch.Runs, func(i, j int) bool { return batch.Runs[i].RunID < batch.Runs[j].RunID })

	var runs []RunStatus
	for _, br := range batch.Runs {
		pr, ok := planRuns[br.RunID]
		if !ok {
			return fmt.Errorf("batch_report runs mismatch")
		}
		if br.Left != pr.Left || br.Right != pr.Right {
			return fmt.Errorf("batch_report runs mismatch")
		}
		if br.Status == "" {
			return fmt.Errorf("batch_report runs mismatch")
		}
		runs = append(runs, RunStatus{RunID: br.RunID, Status: br.Status})
	}

	report := VerifyReport{
		Mode:              batch.Mode,
		OK:                true,
		PlanSHA256:        planSum,
		BatchReportSHA256: batchSum,
		RunCount:          len(runs),
		Runs:              runs,
		Checks: []Check{
			{Name: "plan_manifest.sha256", OK: true},
			{Name: "apply/manifest.sha256", OK: true},
			{Name: "batch_report.plan_sha256", OK: true},
			{Name: "batch_report.runs_match_plan", OK: true},
		},
	}

	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := pfutil.WriteText(filepath.Join(outDir, "verify_report.json"), string(b)+"\n"); err != nil {
		return err
	}
	return pfutil.WriteShaManifest(outDir, []string{"verify_report.json"})
}
