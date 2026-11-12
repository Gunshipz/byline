package testpackaging

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// standardHeaderFields are the top-level SKILL.md frontmatter fields the
// Agent Skills standard allows, plus the two Claude Code settings our house
// rule permits on top of it (wiki agent-skills-standard.md, section 3).
var standardHeaderFields = map[string]bool{
	"name": true, "description": true, "license": true, "compatibility": true,
	"metadata": true, "allowed-tools": true, "disable-model-invocation": true,
	"argument-hint": true,
}

// skillMdPaths are the two copies of Byline's SKILL.md that must carry the
// same, standard-compliant header: the source the model loads from this
// repo's skills/ folder, and the committed copy the Go binary embeds.
var skillMdPaths = []string{
	"skills/byline/SKILL.md",
	"server/embed/skills/byline/SKILL.md",
}

var topLevelKeyRe = regexp.MustCompile(`^([a-zA-Z][a-zA-Z0-9_-]*):(.*)$`)

// skillFrontmatter returns the text between SKILL.md's opening and closing
// --- lines.
func skillFrontmatter(t *testing.T, rel, s string) string {
	t.Helper()
	if !strings.HasPrefix(s, "---\n") {
		t.Fatalf("%s doesn't start with a --- line", rel)
	}
	end := strings.Index(s[4:], "\n---\n")
	if end < 0 {
		t.Fatalf("%s frontmatter has no closing --- line", rel)
	}
	return s[4 : 4+end+1]
}

// topLevelHeaderFields returns front's top-level field names, in document
// order: a line that starts in column 0 with "key:", never a block-list
// entry indented under one (allowed-tools' "  - ..." lines).
func topLevelHeaderFields(front string) []string {
	var keys []string
	for _, line := range strings.Split(front, "\n") {
		if m := topLevelKeyRe.FindStringSubmatch(line); m != nil {
			keys = append(keys, m[1])
		}
	}
	return keys
}

// headerValue returns key's one-line value in front, trimmed of quotes;
// "" when key's value spans a block instead, as allowed-tools' list does.
func headerValue(front, key string) string {
	for _, line := range strings.Split(front, "\n") {
		if m := topLevelKeyRe.FindStringSubmatch(line); m != nil && m[1] == key {
			return strings.Trim(strings.TrimSpace(m[2]), `"'`)
		}
	}
	return ""
}

var skillNameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// TestSkillStandard_Header fails if either copy of Byline's SKILL.md
// carries a header field outside the standard's allowed set, a name that
// doesn't match its folder or breaks the naming rule, an empty or
// over-length description, or a file 500 lines or longer (agent-skills-
// standard.md section 3 and section 5's pre-ship checklist). That the two
// copies match each other is TestEmbed_CommittedCopiesInSync's job.
func TestSkillStandard_Header(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range skillMdPaths {
		rel := rel
		t.Run(rel, func(t *testing.T) {
			full := filepath.Join(root, rel)
			data, err := os.ReadFile(full)
			if err != nil {
				t.Fatalf("read %s: %v", rel, err)
			}
			text := string(data)
			front := skillFrontmatter(t, rel, text)

			for _, key := range topLevelHeaderFields(front) {
				if !standardHeaderFields[key] {
					t.Errorf("%s: header field %q is outside the standard's allowed top-level fields", rel, key)
				}
			}

			name := headerValue(front, "name")
			folder := filepath.Base(filepath.Dir(full))
			if name != folder {
				t.Errorf("%s: name %q doesn't match its folder name %q", rel, name, folder)
			}
			if name == "" || len(name) > 64 || !skillNameRe.MatchString(name) {
				t.Errorf("%s: name %q breaks the naming rule (1-64 chars, lowercase letters, numbers and single hyphens, no leading or trailing hyphen)", rel, name)
			}

			desc := headerValue(front, "description")
			if desc == "" {
				t.Errorf("%s: description is empty", rel)
			}
			if len(desc) > 1024 {
				t.Errorf("%s: description is %d characters, want 1,024 or fewer", rel, len(desc))
			}

			if lines := strings.Count(text, "\n"); lines >= 500 {
				t.Errorf("%s: %d lines, want fewer than 500", rel, lines)
			}
		})
	}
}

// evalsCaseStringFields are the evals.json fields each case must carry as a
// non-empty string.
var evalsCaseStringFields = []string{"id", "prompt", "expected_output"}

// TestSkillStandard_Evals fails if skills/byline/evals/evals.json is
// missing, isn't valid JSON, has fewer than 3 cases, or any case lacks id,
// prompt, expected_output or a non-empty assertions list (agent-skills-
// standard.md section 4A and section 5's pre-ship checklist).
func TestSkillStandard_Evals(t *testing.T) {
	root := repoRoot(t)
	rel := filepath.Join("skills", "byline", "evals", "evals.json")
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("%s is missing or unreadable: %v", rel, err)
	}

	var cases []map[string]json.RawMessage
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("%s isn't valid JSON: %v", rel, err)
	}
	if len(cases) < 3 {
		t.Fatalf("%s has %d cases, want at least 3", rel, len(cases))
	}

	for i, c := range cases {
		for _, field := range evalsCaseStringFields {
			raw, ok := c[field]
			if !ok {
				t.Errorf("case %d: missing %q", i, field)
				continue
			}
			var s string
			if err := json.Unmarshal(raw, &s); err != nil || strings.TrimSpace(s) == "" {
				t.Errorf("case %d: %q isn't a non-empty string", i, field)
			}
		}

		raw, ok := c["assertions"]
		if !ok {
			t.Errorf("case %d: missing \"assertions\"", i)
			continue
		}
		var assertions []string
		if err := json.Unmarshal(raw, &assertions); err != nil {
			t.Errorf("case %d: \"assertions\" isn't a list of strings: %v", i, err)
			continue
		}
		if len(assertions) == 0 {
			t.Errorf("case %d: \"assertions\" is empty, want at least one", i)
		}
	}
}
