// Package version reads Byline's own declared version from its embedded
// skill, and compares it with the version another app's install declares it
// needs.
package version

import (
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"strings"

	"byline/embed"
)

// SkillID is the embedded skill's directory name under skills/.
const SkillID = "byline"

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

// RequiredHelper and UpdateStep are declared per copy in copy_specific.go:
// where the newest install lives, and the wording of the step that
// updates it, differ by copy.

// readJSONFile decodes a copy's skill.json with UseNumber, so an unknown
// numeric field survives exactly rather than rounding through float64.
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
