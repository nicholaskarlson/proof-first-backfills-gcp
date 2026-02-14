package pfutil

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

func IsUnsafeOutDir(p string) bool {
	if p == "" {
		return true
	}
	clean := filepath.Clean(p)
	if clean == "." || clean == ".." || clean == string(filepath.Separator) {
		return true
	}
	// Windows volume roots (e.g. C:\)
	if runtime.GOOS == "windows" {
		vol := filepath.VolumeName(clean)
		if vol != "" && (clean == vol+"\\" || clean == vol) {
			return true
		}
	}
	return false
}

func ResetOutDir(out string) error {
	if IsUnsafeOutDir(out) {
		return fmt.Errorf("refusing unsafe --out: %q", out)
	}
	if err := os.RemoveAll(out); err != nil {
		return err
	}
	return os.MkdirAll(out, 0o755)
}

func WriteText(path, s string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(s), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func Sha256File(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func ListFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return rerr
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func DiffTrees(gotRoot, expRoot string) error {
	got, err := ListFiles(gotRoot)
	if err != nil {
		return err
	}
	exp, err := ListFiles(expRoot)
	if err != nil {
		return err
	}
	if strings.Join(got, "\n") != strings.Join(exp, "\n") {
		return fmt.Errorf("tree mismatch:\nGOT:\n%s\n\nEXP:\n%s\n", strings.Join(got, "\n"), strings.Join(exp, "\n"))
	}
	for _, rel := range got {
		gb, err := os.ReadFile(filepath.Join(gotRoot, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		eb, err := os.ReadFile(filepath.Join(expRoot, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		if string(gb) != string(eb) {
			return fmt.Errorf("file mismatch: %s", rel)
		}
	}
	return nil
}

func WriteShaManifest(outDir string, filenames []string) error {
	sort.Strings(filenames)
	var b strings.Builder
	for _, fn := range filenames {
		sum, err := Sha256File(filepath.Join(outDir, fn))
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s  %s\n", sum, fn)
	}
	return WriteText(filepath.Join(outDir, "manifest.sha256"), b.String())
}
