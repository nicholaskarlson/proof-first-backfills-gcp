package diff

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nicholaskarlson/proof-first-backfills-gcp/internal/pfutil"
)

type DriftReport struct {
	Mode            string   `json:"mode"`
	AManifestSHA256 string   `json:"a_manifest_sha256"`
	BManifestSHA256 string   `json:"b_manifest_sha256"`
	Added           []string `json:"added"`
	Removed         []string `json:"removed"`
	Changed         []string `json:"changed"`
}

// readPackManifest reads and validates <packDir>/manifest.sha256 against the files in packDir.
// It returns a map of relative path -> sha256 for entries in manifest.sha256.
func readPackManifest(packDir string) (map[string]string, string, error) {
	mfPath := filepath.Join(packDir, "manifest.sha256")
	b, err := os.ReadFile(mfPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", fmt.Errorf("missing pack manifest.sha256")
		}
		return nil, "", err
	}

	// sha over the manifest file itself for reference in drift_report.json.
	sumBytes := sha256.Sum256(b)
	mfSum := hex.EncodeToString(sumBytes[:])

	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	out := make(map[string]string, len(lines))
	for _, line := range lines {
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 {
			continue
		}
		sha := strings.TrimSpace(parts[0])
		fn := strings.TrimSpace(parts[1])
		if sha == "" || fn == "" {
			continue
		}
		out[fn] = sha
	}

	// Validate: for every entry, the file bytes must match its sha.
	var fns []string
	for fn := range out {
		fns = append(fns, fn)
	}
	sort.Strings(fns)
	for _, fn := range fns {
		got, err := pfutil.Sha256File(filepath.Join(packDir, filepath.FromSlash(fn)))
		if err != nil {
			return nil, "", err
		}
		if got != out[fn] {
			return nil, "", fmt.Errorf("pack manifest sha mismatch")
		}
	}
	return out, mfSum, nil
}

func isEvidencePath(p string) bool {
	// Exclude the pack's own sha manifest (handled separately).
	if p == "manifest.sha256" {
		return false
	}
	// Exclude pack index + all lane manifests: we want drift over evidence bytes, not receipts-about-receipts.
	if p == "pack_manifest.json" {
		return false
	}
	if strings.HasSuffix(p, "/manifest.sha256") {
		return false
	}
	return true
}

// Diff computes drift between two verified packs and writes a deterministic drift_report.json and manifest.sha256.
func Diff(packA, packB, outDir string) error {
	if err := pfutil.ResetOutDir(outDir); err != nil {
		return err
	}

	aMap, aMfSum, err := readPackManifest(packA)
	if err != nil {
		return err
	}
	bMap, bMfSum, err := readPackManifest(packB)
	if err != nil {
		return err
	}

	aEv := make(map[string]string)
	for k, v := range aMap {
		if isEvidencePath(k) {
			aEv[k] = v
		}
	}
	bEv := make(map[string]string)
	for k, v := range bMap {
		if isEvidencePath(k) {
			bEv[k] = v
		}
	}

	added := make([]string, 0)
	removed := make([]string, 0)
	changed := make([]string, 0)
	for k, vB := range bEv {
		vA, ok := aEv[k]
		if !ok {
			added = append(added, k)
			continue
		}
		if vA != vB {
			changed = append(changed, k)
		}
	}
	for k := range aEv {
		if _, ok := bEv[k]; !ok {
			removed = append(removed, k)
		}
	}

	sort.Strings(added)
	sort.Strings(removed)
	sort.Strings(changed)

	rep := DriftReport{
		Mode:            "diff",
		AManifestSHA256: aMfSum,
		BManifestSHA256: bMfSum,
		Added:           added,
		Removed:         removed,
		Changed:         changed,
	}
	j, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	if err := pfutil.WriteText(filepath.Join(outDir, "drift_report.json"), string(j)+"\n"); err != nil {
		return err
	}

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
