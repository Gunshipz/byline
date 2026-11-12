package testpackaging

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"testing"
)

// repoRoot returns the repository root, resolved from this source file's own
// path so it works regardless of the test's current working directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("repoRoot: runtime.Caller failed")
	}
	// file is <root>/server/testpackaging/helpers_test.go
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}

// copyTree copies every file under src to the same relative paths under dst.
// A missing src is not an error: there is simply nothing to copy.
func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatalf("copyTree: stat %s: %v", src, err)
	}
	if !info.IsDir() {
		t.Fatalf("copyTree: %s is not a directory", src)
	}
	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copyTree %s -> %s: %v", src, dst, err)
	}
}

// buildHost builds byline-mcp for the host platform with a plain `go build`
// (not build.sh) and returns the binary's path.
func buildHost(t *testing.T) string {
	t.Helper()
	root := repoRoot(t)
	bin := filepath.Join(t.TempDir(), "byline-mcp")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/byline-mcp")
	cmd.Dir = filepath.Join(root, "server")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build byline-mcp: %v\n%s", err, out)
	}
	return bin
}

// readJSON reads path and decodes it into v.
func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readJSON: read %s: %v", path, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("readJSON: parse %s: %v", path, err)
	}
}

var semverRe = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)$`)

// semver fails the test unless s is exactly three dot-separated integers,
// and returns them.
func semver(t *testing.T, s string) [3]int {
	t.Helper()
	m := semverRe.FindStringSubmatch(s)
	if m == nil {
		t.Fatalf("semver: %q is not major.minor.patch", s)
	}
	var out [3]int
	for i := 0; i < 3; i++ {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			t.Fatalf("semver: %q: %v", s, err)
		}
		out[i] = n
	}
	return out
}

// greater reports whether a is numerically greater than b.
func greater(a, b [3]int) bool {
	for i := 0; i < 3; i++ {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

// treeFiles returns every regular file under root, relative path to a
// content hash. A missing root is not an error: it simply has no files.
func treeFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return files
		}
		t.Fatalf("treeFiles: stat %s: %v", root, err)
	}
	if !info.IsDir() {
		t.Fatalf("treeFiles: %s is not a directory", root)
	}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		files[rel] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatalf("treeFiles: walk %s: %v", root, err)
	}
	return files
}

// sameTree lists every file that differs, is missing, or is extra between a
// and b. An empty result means the two trees are identical. Either path may
// be absent (an absent tree simply has no files).
func sameTree(t *testing.T, a, b string) []string {
	t.Helper()
	aFiles := treeFiles(t, a)
	bFiles := treeFiles(t, b)
	var diffs []string
	for rel, aHash := range aFiles {
		bHash, ok := bFiles[rel]
		switch {
		case !ok:
			diffs = append(diffs, "missing in "+b+": "+rel)
		case aHash != bHash:
			diffs = append(diffs, "differs: "+rel)
		}
	}
	for rel := range bFiles {
		if _, ok := aFiles[rel]; !ok {
			diffs = append(diffs, "extra in "+b+": "+rel)
		}
	}
	sort.Strings(diffs)
	return diffs
}
