package version

import (
	"path/filepath"
)

const (
	PluginName    = "byline"
	UpdateCommand = "claude plugin update byline@byline"
)

// RequiredHelper returns the requires_helper version declared by the newest
// Byline plugin copy in Claude Code's plugin cache
// (<config>/plugins/cache/<marketplace>/byline/<version>/), so a helper
// still running from an older copy after an update can say so. "" if
// there is none, or it can't be read or parsed.
func RequiredHelper(getenv func(string) string, home string) string {
	config := getenv("CLAUDE_CONFIG_DIR")
	if config == "" {
		config = filepath.Join(home, ".claude")
	}
	skills, err := filepath.Glob(filepath.Join(config, "plugins", "cache", "*", PluginName, "*", "skills", SkillID, "skill.json"))
	if err != nil {
		return ""
	}
	bestVersion, bestPath := "", ""
	for _, p := range skills {
		v := filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(p))))
		if !versionRe.MatchString(v) {
			continue
		}
		if bestPath == "" || Compare(v, bestVersion) > 0 {
			bestVersion, bestPath = v, p
		}
	}
	if bestPath == "" {
		return ""
	}
	var m map[string]any
	if !readJSONFile(bestPath, &m) {
		return ""
	}
	s, _ := m["requires_helper"].(string)
	return s
}

// UpdateStep names the one step that gets the right version installed.
func UpdateStep(getenv func(string) string) string {
	return "run " + UpdateCommand + " in a terminal"
}
