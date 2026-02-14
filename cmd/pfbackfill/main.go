package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/nicholaskarlson/proof-first-backfills-gcp/internal/apply"
	"github.com/nicholaskarlson/proof-first-backfills-gcp/internal/model"
	"github.com/nicholaskarlson/proof-first-backfills-gcp/internal/pfutil"
	"gopkg.in/yaml.v3"
)

type Run struct {
	RunID string `yaml:"run_id"`
	Left  string `yaml:"left"`
	Right string `yaml:"right"`
}

type Config struct {
	ProjectID    string `yaml:"project_id"`
	Region       string `yaml:"region"`
	InputBucket  string `yaml:"input_bucket"`
	OutputBucket string `yaml:"output_bucket"`
	ServiceName  string `yaml:"service_name"`
	Runs         []Run  `yaml:"runs"`
}

func (c *Config) Validate() error {
	if c.ProjectID == "" {
		return fmt.Errorf("missing required field: project_id")
	}
	if c.Region == "" {
		return fmt.Errorf("missing required field: region")
	}
	if c.InputBucket == "" {
		return fmt.Errorf("missing required field: input_bucket")
	}
	if c.OutputBucket == "" {
		return fmt.Errorf("missing required field: output_bucket")
	}
	if c.ServiceName == "" {
		return fmt.Errorf("missing required field: service_name")
	}
	if len(c.Runs) == 0 {
		return fmt.Errorf("missing required field: runs")
	}
	for i, r := range c.Runs {
		if r.RunID == "" {
			return fmt.Errorf("runs[%d] missing required field: run_id", i)
		}
		if r.Left == "" {
			return fmt.Errorf("runs[%d] missing required field: left", i)
		}
		if r.Right == "" {
			return fmt.Errorf("runs[%d] missing required field: right", i)
		}
	}
	return nil
}

func loadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("invalid yaml: %s", err.Error())
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func writeError(outDir, msg string) error {
	if err := pfutil.ResetOutDir(outDir); err != nil {
		return err
	}
	return pfutil.WriteText(filepath.Join(outDir, "error.txt"), msg+"\n")
}

func render(cfg *Config, outDir string) error {
	if err := pfutil.ResetOutDir(outDir); err != nil {
		return err
	}

	// Deterministic: sort runs by run_id.
	runs := make([]model.PlanRun, 0, len(cfg.Runs))
	for _, r := range cfg.Runs {
		runs = append(runs, model.PlanRun{RunID: r.RunID, Left: r.Left, Right: r.Right})
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].RunID < runs[j].RunID })

	manifest := model.PlanManifest{
		ProjectID:    cfg.ProjectID,
		Region:       cfg.Region,
		InputBucket:  cfg.InputBucket,
		OutputBucket: cfg.OutputBucket,
		ServiceName:  cfg.ServiceName,
		Runs:         runs,
	}

	b, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := pfutil.WriteText(filepath.Join(outDir, "plan_manifest.json"), string(b)+"\n"); err != nil {
		return err
	}
	return pfutil.WriteShaManifest(outDir, []string{"plan_manifest.json"})
}

func demo(outDir string) int {
	casesDir := "fixtures/input"
	entries, err := os.ReadDir(casesDir)
	if err != nil {
		fmt.Println(err.Error())
		return 1
	}
	var cases []string
	for _, e := range entries {
		if e.IsDir() {
			cases = append(cases, e.Name())
		}
	}
	sort.Strings(cases)

	for _, c := range cases {
		inDir := filepath.Join(casesDir, c)
		expDir := filepath.Join("fixtures/expected", c)
		cfgPath := filepath.Join(inDir, "config.yaml")
		outCase := filepath.Join(outDir, c)

		cfg, err := loadConfig(cfgPath)
		if err != nil {
			_ = writeError(outCase, err.Error())
		} else {
			if err := render(cfg, outCase); err != nil {
				_ = writeError(outCase, err.Error())
			} else {
				planPath := filepath.Join(outCase, "plan_manifest.json")
				applyOut := filepath.Join(outCase, "apply")
				if err := apply.ApplyDryRun(planPath, applyOut); err != nil {
					_ = writeError(outCase, err.Error())
				}
			}
		}

		if err := pfutil.DiffTrees(outCase, expDir); err != nil {
			fmt.Println(err.Error())
			return 1
		}
	}

	fmt.Println("OK")
	return 0
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: pfbackfill <render|apply|demo> [args]")
		os.Exit(2)
	}

	switch os.Args[1] {
	case "render":
		fs := flag.NewFlagSet("render", flag.ExitOnError)
		cfgPath := fs.String("config", "", "path to config.yaml")
		outDir := fs.String("out", "", "output directory (cleared)")
		_ = fs.Parse(os.Args[2:])
		if *cfgPath == "" || *outDir == "" {
			fmt.Println("render requires --config and --out")
			os.Exit(2)
		}
		cfg, err := loadConfig(*cfgPath)
		if err != nil {
			_ = writeError(*outDir, err.Error())
			os.Exit(1)
		}
		if err := render(cfg, *outDir); err != nil {
			_ = writeError(*outDir, err.Error())
			os.Exit(1)
		}

	case "apply":
		fs := flag.NewFlagSet("apply", flag.ExitOnError)
		planPath := fs.String("plan", "", "path to plan_manifest.json (must have sibling manifest.sha256)")
		outDir := fs.String("out", "", "output directory (cleared)")
		_ = fs.Parse(os.Args[2:])
		if *planPath == "" || *outDir == "" {
			fmt.Println("apply requires --plan and --out")
			os.Exit(2)
		}
		if err := apply.ApplyDryRun(*planPath, *outDir); err != nil {
			_ = writeError(*outDir, err.Error())
			os.Exit(1)
		}

	case "demo":
		fs := flag.NewFlagSet("demo", flag.ExitOnError)
		outDir := fs.String("out", "", "output directory (cleared)")
		_ = fs.Parse(os.Args[2:])
		if *outDir == "" {
			fmt.Println("demo requires --out")
			os.Exit(2)
		}
		os.Exit(demo(*outDir))

	default:
		fmt.Println("unknown command:", os.Args[1])
		os.Exit(2)
	}
}
