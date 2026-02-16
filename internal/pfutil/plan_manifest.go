package pfutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/nicholaskarlson/proof-first-backfills-gcp/internal/model"
)

// UnmarshalPlanManifestStrict parses plan_manifest.json with DisallowUnknownFields.
//
// Why: defense-in-depth. If someone hand-edits a plan, we want to fail fast on
// extra/typo fields rather than silently ignoring them.
//
// NOTE: Error messages are part of the deterministic contract and may be used by fixtures.
func UnmarshalPlanManifestStrict(raw []byte, out *model.PlanManifest) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	if err := dec.Decode(out); err != nil {
		return normalizePlanManifestDecodeErr(err)
	}
	// Ensure there is no trailing JSON (defense-in-depth).
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing data after plan_manifest.json")
		}
		return err
	}
	return nil
}

func normalizePlanManifestDecodeErr(err error) error {
	const prefix = "json: unknown field "
	msg := err.Error()
	if strings.HasPrefix(msg, prefix) {
		field := strings.Trim(msg[len(prefix):], "\"")
		return fmt.Errorf("unknown field in plan_manifest.json: %s", field)
	}
	return err
}

// ValidatePlanManifest enforces defense-in-depth checks on a rendered plan_manifest.json.
// It is used by plan consumers (apply/verify/local) to fail fast on hand-edited or malformed plans.
//
// NOTE: Error messages are part of the deterministic contract and are used by fixtures.
func ValidatePlanManifest(plan *model.PlanManifest) error {
	if strings.TrimSpace(plan.ProjectID) == "" {
		return fmt.Errorf("missing required field: project_id")
	}
	if strings.TrimSpace(plan.Region) == "" {
		return fmt.Errorf("missing required field: region")
	}
	if strings.TrimSpace(plan.InputBucket) == "" {
		return fmt.Errorf("missing required field: input_bucket")
	}
	if strings.TrimSpace(plan.OutputBucket) == "" {
		return fmt.Errorf("missing required field: output_bucket")
	}
	if strings.TrimSpace(plan.ServiceName) == "" {
		return fmt.Errorf("missing required field: service_name")
	}

	if len(plan.Runs) == 0 {
		return fmt.Errorf("runs[] must not be empty")
	}

	seen := map[string]int{}
	for i, r := range plan.Runs {
		if err := ValidateRunID(r.RunID); err != nil {
			return fmt.Errorf("runs[%d] invalid run_id: %s", i, err.Error())
		}
		if prev, ok := seen[r.RunID]; ok {
			return fmt.Errorf("runs[%d] duplicate run_id: %s (already in runs[%d])", i, r.RunID, prev)
		}
		seen[r.RunID] = i

		if err := ValidateObjectKey(r.Left); err != nil {
			return fmt.Errorf("runs[%d] invalid left: %s", i, err.Error())
		}
		if err := ValidateObjectKey(r.Right); err != nil {
			return fmt.Errorf("runs[%d] invalid right: %s", i, err.Error())
		}
	}

	return nil
}
