package version

import (
	"fmt"
	"path/filepath"
	"testing"
)

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
