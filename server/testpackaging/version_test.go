package testpackaging

import (
	"path/filepath"
	"testing"
)

func TestVersion_SkillJSONShape(t *testing.T) {
	root := repoRoot(t)
	var m map[string]any
	readJSON(t, filepath.Join(root, "skills", "byline", "skill.json"), &m)

	if len(m) != 3 {
		t.Fatalf("skill.json has keys %v, want exactly name, version, requires_helper", m)
	}
	for _, k := range []string{"name", "version", "requires_helper"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("skill.json missing key %q", k)
		}
	}
	if name, _ := m["name"].(string); name != "byline" {
		t.Fatalf("name = %v, want byline", m["name"])
	}
	version, _ := m["version"].(string)
	requires, _ := m["requires_helper"].(string)
	semver(t, version)
	semver(t, requires)
}

func TestVersion_RequiresHelperNotAboveVersion(t *testing.T) {
	root := repoRoot(t)
	var m map[string]any
	readJSON(t, filepath.Join(root, "skills", "byline", "skill.json"), &m)

	version := semver(t, m["version"].(string))
	requires := semver(t, m["requires_helper"].(string))
	if greater(requires, version) {
		t.Fatalf("requires_helper %v is greater than version %v", requires, version)
	}
}

func TestVersion_CompareIsNumeric(t *testing.T) {
	if !greater([3]int{1, 10, 0}, [3]int{1, 9, 0}) {
		t.Fatal("greater({1,10,0},{1,9,0}) = false, want true")
	}
	if greater([3]int{1, 0, 0}, [3]int{1, 0, 0}) {
		t.Fatal("greater({1,0,0},{1,0,0}) = true, want false")
	}
	if greater([3]int{1, 0, 0}, [3]int{1, 0, 1}) {
		t.Fatal("greater({1,0,0},{1,0,1}) = true, want false")
	}
}
