package model

// PlanRun is one unit of work in a batch/backfill plan.
type PlanRun struct {
	RunID string `json:"run_id"`
	Left  string `json:"left"`
	Right string `json:"right"`
}

// PlanManifest is the deterministic “plan you can diff” produced by render.
type PlanManifest struct {
	ProjectID    string    `json:"project_id"`
	Region       string    `json:"region"`
	InputBucket  string    `json:"input_bucket"`
	OutputBucket string    `json:"output_bucket"`
	ServiceName  string    `json:"service_name"`
	Runs         []PlanRun `json:"runs"`
}
