package version

import (
	"encoding/json"
	"fmt"
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

var noEnv = func(string) string { return "" }

// pluginCopy writes home/.claude/plugins/cache/byline/byline/1.0.<n>/skills/byline/skill.json;
// requires == "" leaves requires_helper out.
func pluginCopy(t *testing.T, home string, n int, requires string) {
	t.Helper()
	body := `{"version":"1.0.0"`
	if requires != "" {
		body += fmt.Sprintf(`,"requires_helper":%q`, requires)
	}
	body += "}"
	path := filepath.Join(home, ".claude", "plugins", "cache", "byline", "byline", fmt.Sprintf("1.0.%d", n), "skills", "byline", "skill.json")
	writeFile(t, path, body)
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
// a plugin copy's skill.json decodes with UseNumber:
// a field outside what we read must survive exactly, not round through
// float64, which loses precision above 2^53.
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

func TestNotice_SkillDeclaringNothingGetsNone(t *testing.T) {
	h := t.TempDir()
	pluginCopy(t, h, 1, "")
	want(t, RequiredHelper(noEnv, h), "")
	want(t, UpdateNotice(noEnv, h), "")
}

func TestNotice_NoPluginCopyGetsNone(t *testing.T) {
	want(t, UpdateNotice(noEnv, t.TempDir()), "")
}

func TestNotice_NewerRequirementGivesTheCommand(t *testing.T) {
	want(t, UpdateStep(noEnv), "run claude plugin update byline@byline in a terminal")
	want(t, Notice("1.0.0", "1.2.0", UpdateStep(noEnv)), "Byline's checks on this computer are version 1.0.0, but its instructions need version 1.2.0, so run claude plugin update byline@byline in a terminal, then quit and reopen the app.")
	h := t.TempDir()
	pluginCopy(t, h, 1, "9.0.0")
	want(t, UpdateNotice(noEnv, h), Notice(Version(), "9.0.0", UpdateStep(noEnv)))
}

func TestNotice_MetOrUnreadableRequirementGetsNone(t *testing.T) {
	for _, r := range []string{"1.0.0", "0.9.0", "not-a-version", "1.1.0-beta"} {
		h := t.TempDir()
		pluginCopy(t, h, 1, r)
		want(t, UpdateNotice(noEnv, h), "")
	}
	want(t, Notice("", "9.0.0", "x"), "")
}

func TestNotice_ConfigDirOverridesHome(t *testing.T) {
	h := t.TempDir()
	pluginCopy(t, h, 1, "1.0.0") // under ~/.claude: ignored once CLAUDE_CONFIG_DIR is set
	config := filepath.Join(h, "other-config")
	writeFile(t, filepath.Join(config, "plugins", "cache", "byline", "byline", "9.0.0", "skills", "byline", "skill.json"), `{"version":"9.0.0","requires_helper":"9.0.0"}`)
	env := func(k string) string {
		if k == "CLAUDE_CONFIG_DIR" {
			return config
		}
		return ""
	}
	want(t, RequiredHelper(env, h), "9.0.0")
}

func TestNotice_NewestVersionWins(t *testing.T) { // numeric, not text order
	h := t.TempDir()
	pluginCopy(t, h, 5, "9.0.0")
	pluginCopy(t, h, 40, "1.0.0")
	want(t, RequiredHelper(noEnv, h), "1.0.0")
}

func TestNotice_EntryWithoutSkillJSONIsSkipped(t *testing.T) {
	h := t.TempDir()
	pluginCopy(t, h, 3, "9.0.0")
	mkdir(t, filepath.Join(h, ".claude/plugins/cache/byline/byline/1.0.9"))          // no skill.json inside
	writeFile(t, filepath.Join(h, ".claude/plugins/cache/byline/byline/1.0.12"), "") // a file, not a folder
	want(t, RequiredHelper(noEnv, h), "9.0.0")
}
