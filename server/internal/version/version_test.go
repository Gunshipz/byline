package version

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func want(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The embedded copy's version is the one the source skill.json declares.
func TestVersion_ReadsEmbeddedSkillJSON(t *testing.T) {
	var m struct{ Version string }
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "skills", SkillID, "skill.json"))
	if err != nil || json.Unmarshal(b, &m) != nil || m.Version == "" {
		t.Fatalf("read the source skill.json: %v", err)
	}
	want(t, Version(), m.Version)
}

func TestCompare(t *testing.T) {
	for _, c := range []struct {
		a, b string
		w    int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.10", "1.0.9", 1},
		{"1.0", "1.0.0", 0},
		{"0.9.9", "1.0.0", -1},
	} {
		if g := Compare(c.a, c.b); g != c.w {
			t.Errorf("Compare(%q,%q)=%d want %d", c.a, c.b, g, c.w)
		}
	}
}

// TestReadJSONFile_PreservesLargeIntegers pins the Phase 4 constraint that
// a copy's skill.json decodes with UseNumber: a field outside what we
// read must survive exactly, not round through float64, which loses
// precision above 2^53.
func TestReadJSONFile_PreservesLargeIntegers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skill.json")
	writeFile(t, path, `{"requires_helper":"1.0.0","big":9007199254740993}`)
	var m map[string]any
	if !readJSONFile(path, &m) {
		t.Fatal("readJSONFile failed")
	}
	num, ok := m["big"].(json.Number)
	if !ok {
		t.Fatalf("big = %#v (%T), want json.Number", m["big"], m["big"])
	}
	want(t, num.String(), "9007199254740993")
}
