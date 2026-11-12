package content

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
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
	// file is <root>/server/internal/content/helpers_test.go
	return filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(file))))
}

// readFile reads a repo-relative path and fails the test if it's missing.
func readFile(t *testing.T, rel string) string {
	t.Helper()
	path := filepath.Join(repoRoot(t), rel)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readFile: %s: %v", rel, err)
	}
	return string(data)
}

// privateDir returns BYLINE_PRIVATE_FIXTURES, or skips the test when it's
// unset.
func privateDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("BYLINE_PRIVATE_FIXTURES")
	if dir == "" {
		t.Skip("BYLINE_PRIVATE_FIXTURES unset")
	}
	return dir
}

// ruleFile is the shape shared by core and legacy rule files.
type ruleFile struct {
	Version     string           `json:"version"`
	Families    []map[string]any `json:"families"`
	Rules       []map[string]any `json:"rules"`
	VoiceTarget map[string]any   `json:"voice_target"`
}

// loadRules reads and parses a rule file at path, used for both core and
// legacy files.
func loadRules(t *testing.T, path string) ruleFile {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("loadRules: read %s: %v", path, err)
	}
	var rf ruleFile
	if err := json.Unmarshal(data, &rf); err != nil {
		t.Fatalf("loadRules: parse %s: %v", path, err)
	}
	return rf
}
