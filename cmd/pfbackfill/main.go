package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	yaml "gopkg.in/yaml.v3"

	"github.com/nicholaskarlson/proof-first-backfills-gcp/internal/apply"
	"github.com/nicholaskarlson/proof-first-backfills-gcp/internal/cloudplan"
	"github.com/nicholaskarlson/proof-first-backfills-gcp/internal/diff"
	"github.com/nicholaskarlson/proof-first-backfills-gcp/internal/local"
	"github.com/nicholaskarlson/proof-first-backfills-gcp/internal/model"
	"github.com/nicholaskarlson/proof-first-backfills-gcp/internal/pack"
	"github.com/nicholaskarlson/proof-first-backfills-gcp/internal/pfutil"
	"github.com/nicholaskarlson/proof-first-backfills-gcp/internal/verify"
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

	seen := map[string]int{}
	for i, r := range c.Runs {
		if r.RunID == "" {
			return fmt.Errorf("runs[%d] missing required field: run_id", i)
		}
		if err := pfutil.ValidateRunID(r.RunID); err != nil {
			return fmt.Errorf("runs[%d] invalid run_id: %s", i, err.Error())
		}
		if prev, ok := seen[r.RunID]; ok {
			return fmt.Errorf("runs[%d] duplicate run_id: %s (already in runs[%d])", i, r.RunID, prev)
		}
		seen[r.RunID] = i
		if r.Left == "" {
			return fmt.Errorf("runs[%d] missing required field: left", i)
		}
		if r.Right == "" {
			return fmt.Errorf("runs[%d] missing required field: right", i)
		}

		if err := pfutil.ValidateObjectKey(r.Left); err != nil {
			return fmt.Errorf("runs[%d] invalid left: %s", i, err.Error())
		}
		if err := pfutil.ValidateObjectKey(r.Right); err != nil {
			return fmt.Errorf("runs[%d] invalid right: %s", i, err.Error())
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

	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		msg := err.Error()
		// Prefer a stable one-liner for unknown-field failures.
		// yaml.v3 typically emits: yaml: unmarshal errors (line N: field X not found in type ...)
		if strings.Contains(msg, "field ") && strings.Contains(msg, "not found in type") {
			var line, field string
			if i := strings.Index(msg, "line "); i != -1 {
				rest := msg[i+len("line "):]
				j := 0
				for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
					j++
				}
				if j > 0 {
					line = rest[:j]
				}
			}
			if i := strings.Index(msg, "field "); i != -1 {
				rest := msg[i+len("field "):]
				j := strings.IndexAny(rest, " \t\r\n")
				if j == -1 {
					field = rest
				} else {
					field = rest[:j]
				}
			}
			if field != "" {
				if line != "" {
					return nil, fmt.Errorf("invalid yaml: unknown field %s (line %s)", field, line)
				}
				return nil, fmt.Errorf("invalid yaml: unknown field %s", field)
			}
		}
		return nil, fmt.Errorf("invalid yaml: %s", msg)
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
	// Demo is part of the proof gate: it must be deterministic and must not depend
	// on prior runs. We clear the demo outDir up front.
	if err := pfutil.ResetOutDir(outDir); err != nil {
		fmt.Println(err.Error())
		return 1
	}

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
		planPath := filepath.Join(inDir, "plan_manifest.json")
		outCase := filepath.Join(outDir, c)
		applyOut := filepath.Join(outCase, "apply")

		localOnly := false
		if _, err := os.Stat(filepath.Join(inDir, "local_only")); err == nil {
			localOnly = true
		}

		diffOnly := false
		if _, err := os.Stat(filepath.Join(inDir, "diff_only")); err == nil {
			diffOnly = true
		}

		cloudPlanOnly := false
		if _, err := os.Stat(filepath.Join(inDir, "cloud_plan_only")); err == nil {
			cloudPlanOnly = true
		}

		wantPack := false
		if _, err := os.Stat(filepath.Join(expDir, "pack")); err == nil {
			wantPack = true
		}

		packOnly := false
		if _, err := os.Stat(filepath.Join(inDir, "pack_only")); err == nil {
			packOnly = true
		}
		if packOnly {
			packOut := filepath.Join(outCase, "pack")
			applyDir := filepath.Join(inDir, "apply")
			verifyDir := filepath.Join(inDir, "verify")
			localDir := filepath.Join(inDir, "local")
			if err := pack.Pack(planPath, applyDir, verifyDir, localDir, packOut); err != nil {
				_ = writeError(packOut, err.Error())
			}
		} else if diffOnly {
			aPack := filepath.Join(inDir, "a_pack")
			bPack := filepath.Join(inDir, "b_pack")
			diffOut := filepath.Join(outCase, "diff")
			if err := diff.Diff(aPack, bPack, diffOut); err != nil {
				_ = writeError(diffOut, err.Error())
			}
		} else if _, err := os.Stat(cfgPath); err == nil {
			// Render-based fixture: load config, render plan artifacts, then apply.
			cfg, err := loadConfig(cfgPath)
			if err != nil {
				_ = writeError(outCase, err.Error())
			} else {
				if err := render(cfg, outCase); err != nil {
					_ = writeError(outCase, err.Error())
				} else {
					// Lane purity: apply failures live in the apply lane folder.
					planOut := filepath.Join(outCase, "plan_manifest.json")
					if err := apply.ApplyDryRun(planOut, applyOut); err != nil {
						_ = writeError(applyOut, err.Error())
					} else {
						verifyOut := filepath.Join(outCase, "verify")
						if err := verify.Verify(planOut, applyOut, verifyOut); err != nil {
							_ = writeError(verifyOut, err.Error())
						} else {
							localOut := filepath.Join(outCase, "local")
							seedDir := filepath.Join(inDir, "seed")
							if _, err := os.Stat(seedDir); err == nil {
								if err := pfutil.CopyTree(seedDir, outCase); err != nil {
									_ = writeError(localOut, err.Error())
								} else if err := local.Exec(planOut, localOut); err != nil {
									_ = writeError(localOut, err.Error())
								} else if wantPack {
									packOut := filepath.Join(outCase, "pack")
									if err := pack.Pack(planOut, applyOut, verifyOut, localOut, packOut); err != nil {
										_ = writeError(packOut, err.Error())
									}
								}
							} else if err := local.Exec(planOut, localOut); err != nil {
								_ = writeError(localOut, err.Error())
							} else if wantPack {
								packOut := filepath.Join(outCase, "pack")
								if err := pack.Pack(planOut, applyOut, verifyOut, localOut, packOut); err != nil {
									_ = writeError(packOut, err.Error())
								}
							}
						}
					}
				}
			}
		} else if _, err := os.Stat(planPath); err == nil {
			if cloudPlanOnly {
				cloudOut := filepath.Join(outCase, "cloud")
				if err := cloudplan.Plan(planPath, cloudOut); err != nil {
					_ = writeError(cloudOut, err.Error())
				}
			} else if localOnly {
				// Local-only fixture: exercise local errors without apply/verify.
				localOut := filepath.Join(outCase, "local")
				seedDir := filepath.Join(inDir, "seed")
				if _, err := os.Stat(seedDir); err == nil {
					if err := pfutil.CopyTree(seedDir, outCase); err != nil {
						_ = writeError(localOut, err.Error())
					} else if err := local.Exec(planPath, localOut); err != nil {
						_ = writeError(localOut, err.Error())
					}
				} else if err := local.Exec(planPath, localOut); err != nil {
					_ = writeError(localOut, err.Error())
				}
			} else {
				// Apply-only fixture: exercise apply errors without changing render.
				//
				// PR23: if fixtures/input/<case>/apply/* exists, seed apply outputs from fixtures
				// and run verify against them (expected-fail verify fixtures).
				seedApply := filepath.Join(inDir, "apply")
				if _, err := os.Stat(seedApply); err == nil {
					if err := pfutil.ResetOutDir(applyOut); err != nil {
						_ = writeError(applyOut, err.Error())
					} else if err := pfutil.CopyTree(seedApply, applyOut); err != nil {
						_ = writeError(applyOut, err.Error())
					} else {
						verifyOut := filepath.Join(outCase, "verify")
						if err := verify.Verify(planPath, applyOut, verifyOut); err != nil {
							_ = writeError(verifyOut, err.Error())
						} else {
							localOut := filepath.Join(outCase, "local")
							seedDir := filepath.Join(inDir, "seed")
							if _, err := os.Stat(seedDir); err == nil {
								if err := pfutil.CopyTree(seedDir, outCase); err != nil {
									_ = writeError(localOut, err.Error())
								} else if err := local.Exec(planPath, localOut); err != nil {
									_ = writeError(localOut, err.Error())
								} else if wantPack {
									packOut := filepath.Join(outCase, "pack")
									if err := pack.Pack(planPath, applyOut, verifyOut, localOut, packOut); err != nil {
										_ = writeError(packOut, err.Error())
									}
								}
							} else if err := local.Exec(planPath, localOut); err != nil {
								_ = writeError(localOut, err.Error())
							}
						}
					}
				} else if err := apply.ApplyDryRun(planPath, applyOut); err != nil {
					_ = writeError(applyOut, err.Error())
				} else {
					verifyOut := filepath.Join(outCase, "verify")
					if err := verify.Verify(planPath, applyOut, verifyOut); err != nil {
						_ = writeError(verifyOut, err.Error())
					} else {
						localOut := filepath.Join(outCase, "local")
						seedDir := filepath.Join(inDir, "seed")
						if _, err := os.Stat(seedDir); err == nil {
							if err := pfutil.CopyTree(seedDir, outCase); err != nil {
								_ = writeError(localOut, err.Error())
							} else if err := local.Exec(planPath, localOut); err != nil {
								_ = writeError(localOut, err.Error())
							} else if wantPack {
								packOut := filepath.Join(outCase, "pack")
								if err := pack.Pack(planPath, applyOut, verifyOut, localOut, packOut); err != nil {
									_ = writeError(packOut, err.Error())
								}
							}
						} else if err := local.Exec(planPath, localOut); err != nil {
							_ = writeError(localOut, err.Error())
						} else if wantPack {
							packOut := filepath.Join(outCase, "pack")
							if err := pack.Pack(planPath, applyOut, verifyOut, localOut, packOut); err != nil {
								_ = writeError(packOut, err.Error())
							}
						}
					}
				}
			}
		} else {
			_ = writeError(outCase, "missing fixture: config.yaml or plan_manifest.json")
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
		fmt.Println("usage: pfbackfill <render|apply|verify|local|cloud-plan|pack|diff|demo> [args]")
		os.Exit(2)
	}
	cmd := os.Args[1]
	if cmd == "-h" || cmd == "--help" || cmd == "help" {
		fmt.Println("usage: pfbackfill <render|apply|verify|local|cloud-plan|pack|diff|demo> [args]")
		os.Exit(0)
	}

	switch cmd {
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

	case "verify":
		fs := flag.NewFlagSet("verify", flag.ExitOnError)
		planPath := fs.String("plan", "", "path to plan_manifest.json (must have sibling manifest.sha256)")
		applyDir := fs.String("apply", "", "apply output directory (must contain batch_report.json + manifest.sha256)")
		outDir := fs.String("out", "", "output directory (cleared)")
		_ = fs.Parse(os.Args[2:])
		if *planPath == "" || *applyDir == "" || *outDir == "" {
			fmt.Println("verify requires --plan, --apply, and --out")
			os.Exit(2)
		}
		if err := verify.Verify(*planPath, *applyDir, *outDir); err != nil {
			_ = writeError(*outDir, err.Error())
			os.Exit(1)
		}

	case "local":
		fs := flag.NewFlagSet("local", flag.ExitOnError)
		planPath := fs.String("plan", "", "path to plan_manifest.json (must have sibling manifest.sha256)")
		outDir := fs.String("out", "", "lane output directory (not cleared; resumable)")
		_ = fs.Parse(os.Args[2:])
		if *planPath == "" || *outDir == "" {
			fmt.Println("local requires --plan and --out")
			os.Exit(2)
		}
		if err := local.Exec(*planPath, *outDir); err != nil {
			_ = writeError(*outDir, err.Error())
			os.Exit(1)
		}
	case "cloud-plan":
		fs := flag.NewFlagSet("cloud-plan", flag.ExitOnError)
		planPath := fs.String("plan", "", "path to plan_manifest.json (must have sibling manifest.sha256)")
		outDir := fs.String("out", "", "output directory (cleared)")
		_ = fs.Parse(os.Args[2:])
		if *planPath == "" || *outDir == "" {
			fmt.Println("cloud-plan requires --plan and --out")
			os.Exit(2)
		}
		if err := cloudplan.Plan(*planPath, *outDir); err != nil {
			_ = writeError(*outDir, err.Error())
			os.Exit(1)
		}

	case "pack":
		fs := flag.NewFlagSet("pack", flag.ExitOnError)
		planPath := fs.String("plan", "", "path to plan_manifest.json (must have sibling manifest.sha256)")
		applyDir := fs.String("apply", "", "apply output directory (must contain batch_report.json + manifest.sha256)")
		verifyDir := fs.String("verify", "", "verify output directory (must contain verify_report.json + manifest.sha256)")
		localDir := fs.String("local", "", "local output directory (must contain local_report.json + local_diff.json + manifest.sha256)")
		outDir := fs.String("out", "", "output directory (cleared)")
		_ = fs.Parse(os.Args[2:])
		if *planPath == "" || *applyDir == "" || *verifyDir == "" || *localDir == "" || *outDir == "" {
			fmt.Println("pack requires --plan, --apply, --verify, --local, and --out")
			os.Exit(2)
		}
		if err := pack.Pack(*planPath, *applyDir, *verifyDir, *localDir, *outDir); err != nil {
			_ = writeError(*outDir, err.Error())
			os.Exit(1)
		}

	case "diff":
		fs := flag.NewFlagSet("diff", flag.ExitOnError)
		aDir := fs.String("a", "", "pack A directory (must contain manifest.sha256)")
		bDir := fs.String("b", "", "pack B directory (must contain manifest.sha256)")
		outDir := fs.String("out", "", "output directory (cleared)")
		_ = fs.Parse(os.Args[2:])
		if *aDir == "" || *bDir == "" || *outDir == "" {
			fmt.Println("diff requires --a, --b, and --out")
			os.Exit(2)
		}
		if err := diff.Diff(*aDir, *bDir, *outDir); err != nil {
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
