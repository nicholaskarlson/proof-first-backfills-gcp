package cloudplan

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

type Rules struct {
	InputPrefix   string `json:"input_prefix"`
	OutputPrefix  string `json:"output_prefix"`
	LeftName      string `json:"left_name"`
	RightName     string `json:"right_name"`
	SuccessMarker string `json:"success_marker"`
	ErrorMarker   string `json:"error_marker"`
}

type StagedObject struct {
	SourceObject string `json:"source_object"`
	DestObject   string `json:"dest_object"`
}

type Stage struct {
	Left  StagedObject `json:"left"`
	Right StagedObject `json:"right"`
}

type Markers struct {
	SuccessObject string `json:"success_object"`
	ErrorObject   string `json:"error_object"`
}

type RunPlan struct {
	RunID         string  `json:"run_id"`
	Stage         Stage   `json:"stage"`
	TriggerObject string  `json:"trigger_object"`
	Markers       Markers `json:"markers"`
}

type Step struct {
	Seq    int    `json:"seq"`
	Action string `json:"action"`
	RunID  string `json:"run_id"`
	Bucket string `json:"bucket"`

	SourceObject  string `json:"source_object,omitempty"`
	DestObject    string `json:"dest_object,omitempty"`
	SuccessObject string `json:"success_object,omitempty"`
	ErrorObject   string `json:"error_object,omitempty"`
}

type CloudPlan struct {
	Mode       string `json:"mode"`
	PlanSHA256 string `json:"plan_sha256"`

	ProjectID    string `json:"project_id"`
	Region       string `json:"region"`
	ServiceName  string `json:"service_name"`
	InputBucket  string `json:"input_bucket"`
	OutputBucket string `json:"output_bucket"`

	Rules    Rules     `json:"rules"`
	RunCount int       `json:"run_count"`
	Runs     []RunPlan `json:"runs"`
	Steps    []Step    `json:"steps"`
}

const (
	defaultInputPrefix   = "in/"
	defaultOutputPrefix  = "out/"
	defaultLeftName      = "left.csv"
	defaultRightName     = "right.csv"
	defaultSuccessMarker = "_SUCCESS.json"
	defaultErrorMarker   = "_ERROR.json"
)

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

// Plan emits a deterministic cloud execution plan (no network, no timestamps).
// It is a planning artifact only: it proves name mappings, ordering, and marker keys.
func Plan(planPath, outDir string) error {
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

	runs := make([]model.PlanRun, 0, len(plan.Runs))
	for _, r := range plan.Runs {
		runs = append(runs, r)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].RunID < runs[j].RunID })

	outRuns := make([]RunPlan, 0, len(runs))
	outSteps := make([]Step, 0, len(runs)*3)

	seq := 1
	for _, r := range runs {
		leftDest := defaultInputPrefix + r.RunID + "/" + defaultLeftName
		rightDest := defaultInputPrefix + r.RunID + "/" + defaultRightName
		successObj := defaultOutputPrefix + r.RunID + "/" + defaultSuccessMarker
		errorObj := defaultOutputPrefix + r.RunID + "/" + defaultErrorMarker

		outRuns = append(outRuns, RunPlan{
			RunID: r.RunID,
			Stage: Stage{
				Left:  StagedObject{SourceObject: r.Left, DestObject: leftDest},
				Right: StagedObject{SourceObject: r.Right, DestObject: rightDest},
			},
			TriggerObject: rightDest,
			Markers:       Markers{SuccessObject: successObj, ErrorObject: errorObj},
		})

		outSteps = append(outSteps,
			Step{
				Seq:          seq,
				Action:       "stage_left",
				RunID:        r.RunID,
				Bucket:       plan.InputBucket,
				SourceObject: r.Left,
				DestObject:   leftDest,
			},
			Step{
				Seq:          seq + 1,
				Action:       "stage_right",
				RunID:        r.RunID,
				Bucket:       plan.InputBucket,
				SourceObject: r.Right,
				DestObject:   rightDest,
			},
			Step{
				Seq:           seq + 2,
				Action:        "poll_marker",
				RunID:         r.RunID,
				Bucket:        plan.OutputBucket,
				SuccessObject: successObj,
				ErrorObject:   errorObj,
			},
		)
		seq += 3
	}

	cp := CloudPlan{
		Mode:         "cloud_plan",
		PlanSHA256:   planSum,
		ProjectID:    plan.ProjectID,
		Region:       plan.Region,
		ServiceName:  plan.ServiceName,
		InputBucket:  plan.InputBucket,
		OutputBucket: plan.OutputBucket,
		Rules: Rules{
			InputPrefix:   defaultInputPrefix,
			OutputPrefix:  defaultOutputPrefix,
			LeftName:      defaultLeftName,
			RightName:     defaultRightName,
			SuccessMarker: defaultSuccessMarker,
			ErrorMarker:   defaultErrorMarker,
		},
		RunCount: len(outRuns),
		Runs:     outRuns,
		Steps:    outSteps,
	}

	b, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return err
	}
	if err := pfutil.WriteText(filepath.Join(outDir, "cloud_plan.json"), string(b)+"\n"); err != nil {
		return err
	}

	return pfutil.WriteShaManifest(outDir, []string{"cloud_plan.json"})
}
