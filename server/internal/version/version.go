// Package version reads Byline's own declared version from its embedded
// skill, and compares it with the version another app's install declares it
// needs.
package version

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"byline/embed"
)

const (
	PluginName    = "byline"
	SkillID       = "byline"
	UpdateCommand = "claude plugin update byline@byline"
)

var versionRe = regexp.MustCompile(`^\d+(\.\d+)*$`)

// Version returns the version declared in our own embedded skill.json, or
// "" if it can't be read or parsed.
func Version() string {
	data, err := embed.Files.ReadFile("skills/" + SkillID + "/skill.json")
	if err != nil {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(data, &m) != nil {
		return ""
	}
	s, _ := m["version"].(string)
	return s
}

// Compare compares two dotted numeric version strings part by part; a
// missing trailing part counts as 0. It returns -1, 0 or 1.
func Compare(a, b string) int {
	ap := strings.Split(a, ".")
	bp := strings.Split(b, ".")
	n := len(ap)
	if len(bp) > n {
		n = len(bp)
	}
	for i := 0; i < n; i++ {
		var av, bv int
		if i < len(ap) {
			av, _ = strconv.Atoi(ap[i])
		}
		if i < len(bp) {
			bv, _ = strconv.Atoi(bp[i])
		}
		if av != bv {
			if av > bv {
				return 1
			}
			return -1
		}
	}
	return 0
}

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

// readJSONFile decodes a plugin copy's skill.json with UseNumber, so an unknown numeric field survives exactly rather
// than rounding through float64.
func readJSONFile(path string, v any) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	dec.UseNumber()
	return dec.Decode(v) == nil
}

// UpdateStep names the one step that gets the right version installed.
func UpdateStep(getenv func(string) string) string {
	return "run " + UpdateCommand + " in a terminal"
}

// Notice formats the update-needed sentence, or "" if either version isn't
// a plain dotted number.
func Notice(ours, needed, step string) string {
	if !versionRe.MatchString(ours) || !versionRe.MatchString(needed) {
		return ""
	}
	return "Byline's checks on this computer are version " + ours + ", but its instructions need version " + needed +
		", so " + step + ", then quit and reopen the app."
}

// UpdateNotice is Notice for this computer, or "" when there's nothing to
// say: no declared requirement, an unreadable one, or one we already meet.
func UpdateNotice(getenv func(string) string, home string) (notice string) {
	defer func() { recover() }()
	needed := RequiredHelper(getenv, home)
	ours := Version()
	if !versionRe.MatchString(ours) || !versionRe.MatchString(needed) {
		return ""
	}
	if Compare(ours, needed) >= 0 {
		return ""
	}
	return Notice(ours, needed, UpdateStep(getenv))
}
