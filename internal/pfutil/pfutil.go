package pfutil

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
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

// CopyTree copies a directory tree from srcDir into dstDir.
// Paths are walked and copied in sorted order for determinism.
// File permissions are normalized (dirs: 0755, files: 0644).

var runIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

func ValidateRunID(id string) error {
	if runIDRe.MatchString(id) {
		return nil
	}
	return fmt.Errorf("must match %s", runIDRe.String())
}

func CopyTree(srcDir, dstDir string) error {
	srcDir = filepath.Clean(srcDir)
	dstDir = filepath.Clean(dstDir)

	var relFiles []string
	walkErr := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		relFiles = append(relFiles, rel)
		return nil
	})
	if walkErr != nil {
		return walkErr
	}

	sort.Strings(relFiles)
	for _, rel := range relFiles {
		srcPath := filepath.Join(srcDir, rel)
		dstPath := filepath.Join(dstDir, rel)

		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return err
		}
		b, err := os.ReadFile(srcPath)
		if err != nil {
			return err
		}

		tmp := dstPath + ".tmp"
		if err := os.WriteFile(tmp, b, 0o644); err != nil {
			return err
		}
		if err := os.Rename(tmp, dstPath); err != nil {
			return err
		}
	}
	return nil
}

// ValidateObjectKey enforces a safe, deterministic object-key style path (POSIX-like).
// We allow forward-slash-separated relative paths (e.g., fixtures/demo/left.csv), but forbid:
// - absolute paths (/x)
// - Windows drive paths (C:/x)
// - backslashes
// - empty segments, "." segments, or ".." traversal
// - non-canonical forms (e.g., repeated slashes, trailing slash)
func ValidateObjectKey(k string) error {
	k = strings.TrimSpace(k)
	if k == "" {
		return fmt.Errorf("empty")
	}
	if strings.Contains(k, "\\") {
		return fmt.Errorf("must not contain backslashes; use '/' only")
	}
	if strings.HasPrefix(k, "/") {
		return fmt.Errorf("must be a relative path (no leading '/')")
	}
	if len(k) >= 2 {
		c0 := k[0]
		if ((c0 >= 'A' && c0 <= 'Z') || (c0 >= 'a' && c0 <= 'z')) && k[1] == ':' {
			return fmt.Errorf("must not be a Windows drive path")
		}
	}
	if strings.HasPrefix(k, "~") {
		return fmt.Errorf("must not start with '~'")
	}
	parts := strings.Split(k, "/")
	for _, p := range parts {
		if p == "" {
			return fmt.Errorf("must not contain empty path segments")
		}
		if p == "." || p == ".." {
			return fmt.Errorf("must not contain '.' or '..' segments")
		}
	}
	clean := path.Clean(k)
	if clean != k {
		return fmt.Errorf("must be clean (no repeated slashes, trailing slash, or dot segments)")
	}
	return nil
}
