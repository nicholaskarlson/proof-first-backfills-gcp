package pack

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nicholaskarlson/proof-first-backfills-gcp/internal/pfutil"
)

type PackManifest struct {
	Mode                   string   `json:"mode"`
	PlanSHA256             string   `json:"plan_sha256"`
	ApplyBatchReportSHA256 string   `json:"apply_batch_report_sha256"`
	VerifyReportSHA256     string   `json:"verify_report_sha256"`
	LocalManifestSHA256    string   `json:"local_manifest_sha256"`
	Entries                []string `json:"entries"`
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

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return pfutil.WriteText(dst, string(b))
}

// Pack builds a deterministic handoff pack from lane receipts.
// It is offline-only: copies artifacts + emits a pack_manifest.json and manifest.sha256.
// The pack manifest.sha256 covers every file in the pack (excluding itself).
func Pack(planPath, applyDir, verifyDir, localDir, outDir string) error {
	if err := pfutil.ResetOutDir(outDir); err != nil {
		return err
	}

	planDir := filepath.Dir(planPath)
	planFile := filepath.Base(planPath)

	planSum, err := verifyShaEntry(planDir, planFile, "plan")
	if err != nil {
		return err
	}
	applySum, err := verifyShaEntry(applyDir, "batch_report.json", "apply")
	if err != nil {
		return err
	}
	verifySum, err := verifyShaEntry(verifyDir, "verify_report.json", "verify")
	if err != nil {
		return err
	}

	localMf := filepath.Join(localDir, "manifest.sha256")
	if _, err := os.Stat(localMf); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("missing local manifest.sha256")
		}
		return err
	}
	localMfSum, err := pfutil.Sha256File(localMf)
	if err != nil {
		return err
	}

	// Copy receipts into a portable pack structure.
	if err := copyFile(planPath, filepath.Join(outDir, "plan", planFile)); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(planDir, "manifest.sha256"), filepath.Join(outDir, "plan", "manifest.sha256")); err != nil {
		return err
	}

	if err := copyFile(filepath.Join(applyDir, "batch_report.json"), filepath.Join(outDir, "apply", "batch_report.json")); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(applyDir, "manifest.sha256"), filepath.Join(outDir, "apply", "manifest.sha256")); err != nil {
		return err
	}

	if err := copyFile(filepath.Join(verifyDir, "verify_report.json"), filepath.Join(outDir, "verify", "verify_report.json")); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(verifyDir, "manifest.sha256"), filepath.Join(outDir, "verify", "manifest.sha256")); err != nil {
		return err
	}

	if err := pfutil.CopyTree(localDir, filepath.Join(outDir, "local")); err != nil {
		return err
	}

	entries := []string{
		filepath.ToSlash(filepath.Join("plan", planFile)),
		"plan/manifest.sha256",
		"apply/batch_report.json",
		"apply/manifest.sha256",
		"verify/verify_report.json",
		"verify/manifest.sha256",
		"local/local_diff.json",
		"local/local_report.json",
		"local/manifest.sha256",
	}
	sort.Strings(entries)

	pm := PackManifest{
		Mode:                   "pack",
		PlanSHA256:             planSum,
		ApplyBatchReportSHA256: applySum,
		VerifyReportSHA256:     verifySum,
		LocalManifestSHA256:    localMfSum,
		Entries:                entries,
	}
	b, err := json.MarshalIndent(pm, "", "  ")
	if err != nil {
		return err
	}
	if err := pfutil.WriteText(filepath.Join(outDir, "pack_manifest.json"), string(b)+"\n"); err != nil {
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
	return pfutil.WriteShaManifest(outDir, filtered)
}
