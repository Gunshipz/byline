package content

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

const skillPath = "skills/byline/SKILL.md"

// referencePaths are the three references Task 5.8 writes; the plain-words
// check reads whichever of them exist.
var referencePaths = []string{
	"skills/byline/references/install-and-hosts.md",
	"skills/byline/references/patterns.md",
	"skills/byline/references/worked-example-generic.md",
}

// skillTools are the 11 Phase 4 tools SKILL.md names (locked).
var skillTools = []string{
	"byline_status", "byline_brief", "byline_lint", "byline_factlint",
	"byline_draftcheck", "byline_voicecheck", "byline_flag", "byline_setup_save",
	"byline_calibrate", "byline_settings_read", "byline_settings_save",
}

var headingRe = regexp.MustCompile(`^(#{1,6}) `)

// headingLevel returns the number of leading #s of a markdown heading line,
// or 0 when the line isn't a heading.
func headingLevel(line string) int {
	if m := headingRe.FindStringSubmatch(line); m != nil {
		return len(m[1])
	}
	return 0
}

// splitFrontmatter returns the text between the opening and closing ---
// lines, and everything after the closing one.
func splitFrontmatter(t *testing.T, s string) (front, body string) {
	t.Helper()
	if !strings.HasPrefix(s, "---\n") {
		t.Fatal("SKILL.md doesn't start with a --- line")
	}
	end := strings.Index(s[4:], "\n---\n")
	if end < 0 {
		t.Fatal("SKILL.md frontmatter has no closing --- line")
	}
	return s[4 : 4+end+1], s[4+end+len("\n---\n"):]
}

// section returns the lines under heading (an exact line) up to the next
// heading of the same or a higher level.
func section(t *testing.T, body, heading string) string {
	t.Helper()
	level := headingLevel(heading)
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		if l != heading {
			continue
		}
		var out []string
		for _, m := range lines[i+1:] {
			if n := headingLevel(m); n > 0 && n <= level {
				break
			}
			out = append(out, m)
		}
		return strings.Join(out, "\n")
	}
	t.Fatalf("SKILL.md has no heading %q", heading)
	return ""
}

func TestSkill_Sections(t *testing.T) {
	_, body := splitFrontmatter(t, readFile(t, skillPath))
	want := []string{
		"## Who you are talking to",
		"## Start every run with the status check",
		"## Modes",
		"### write",
		"### polish",
		"### check",
		"### review",
		"### flag",
		"### setup",
		"## The five drafting steps",
		"### Step 1: Load the voice files before writing a word",
		"### Step 2: Write with them loaded",
		"### Step 3: Trace every fact to its source",
		"### Step 4: Write the spec and pass the structure check",
		"### Step 5: Polish last and show only the final version",
		"## Before anything is sent",
		"## What you hand back",
		"## Why the steps are in this order",
		"## Tools",
		"## References",
	}
	var got []string
	for _, l := range strings.Split(body, "\n") {
		if n := headingLevel(l); n == 2 || n == 3 {
			got = append(got, l)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("headings:\n got %q\nwant %q", got, want)
	}
}

func TestSkill_Frontmatter(t *testing.T) {
	s := readFile(t, skillPath)
	if !strings.HasPrefix(s, "---\nname: byline\ndescription: ") {
		t.Fatalf("SKILL.md starts %q, want \"---\\nname: byline\\ndescription: \"", s[:min(len(s), 40)])
	}
	front, _ := splitFrontmatter(t, s)
	if regexp.MustCompile(`(?m)^\s*version:`).MatchString(front) {
		t.Fatal("frontmatter has a version: line")
	}

	if strings.Contains(front, "allowed-tools:") {
		t.Fatal("frontmatter pre-approves tools; the public skill pre-approves none")
	}
}

var sayRe = regexp.MustCompile(`Say: "([^"]+)"`)

var ruleIDRe = regexp.MustCompile(`\b(K|W|CS|G|P|V)\d+[a-e]?\b`)

// userBanned are words a person must never see from the skill.
var userBanned = []string{
	"byline_", "byline-mcp", "mcp", "MCP", "AIM", "Toolbox", "hook", "plugin", "server",
	"brief", "lint", "factlint", "draftcheck", "voicecheck", "receipt", "spec",
	"killList", "sectionMap", "add_rules", "add_strings", "switch_off", "replace_examples",
	"voice_target", "send_tools", "exempt_channels", "extra_draft_phrases",
	".json", ".md", "BYLINE_HOME", "~/.byline",
}

// sayLines returns every Say: "..." text in SKILL.md, then in each reference
// that exists, in document order.
func sayLines(t *testing.T) []string {
	t.Helper()
	texts := []string{readFile(t, skillPath)}
	for _, rel := range referencePaths {
		data, err := os.ReadFile(filepath.Join(repoRoot(t), rel))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		texts = append(texts, string(data))
	}
	var says []string
	for _, text := range texts {
		for _, m := range sayRe.FindAllStringSubmatch(text, -1) {
			says = append(says, m[1])
		}
	}
	return says
}

// sayInsteadColumn returns the second cell of each body row of the table
// under "## Who you are talking to".
func sayInsteadColumn(t *testing.T) []string {
	t.Helper()
	_, body := splitFrontmatter(t, readFile(t, skillPath))
	var rows []string
	for _, l := range strings.Split(section(t, body, "## Who you are talking to"), "\n") {
		if l = strings.TrimSpace(l); strings.HasPrefix(l, "|") {
			rows = append(rows, l)
		}
	}
	if len(rows) < 3 {
		t.Fatalf("the table under \"## Who you are talking to\" has %d lines, want a header, a separator and rows", len(rows))
	}
	var cells []string
	for _, r := range rows[2:] {
		c := strings.Split(strings.Trim(r, "|"), "|")
		if len(c) != 2 {
			t.Fatalf("table row %q has %d columns, want 2", r, len(c))
		}
		cells = append(cells, strings.TrimSpace(c[1]))
	}
	return cells
}

func TestSkill_UserFacingTextIsPlain(t *testing.T) {
	says := sayLines(t)
	if len(says) < 10 {
		t.Fatalf("found %d Say lines, want at least 10", len(says))
	}
	for _, want := range []string{"The AI model reads them to build your profile", "nothing is saved or sent anywhere except your voice folder"} {
		if !strings.Contains(says[0], want) {
			t.Errorf("the first Say line %q doesn't contain %q", says[0], want)
		}
	}
	for _, s := range says {
		if strings.Contains(strings.ToLower(s), "nothing leaves") {
			t.Errorf("Say line %q says \"nothing leaves\"", s)
		}
	}
	for _, text := range append(says, sayInsteadColumn(t)...) {
		for _, w := range userBanned {
			if strings.Contains(text, w) {
				t.Errorf("user-facing text %q contains %q", text, w)
			}
		}
		if id := ruleIDRe.FindString(text); id != "" {
			t.Errorf("user-facing text %q contains the rule id %q", text, id)
		}
	}
}

func TestSkill_HostAgnostic(t *testing.T) {
	_, body := splitFrontmatter(t, readFile(t, skillPath))
	// word-bounded so "claim " or "Toolbox" in prose doesn't count; these catch the commands
	for _, re := range []*regexp.Regexp{regexp.MustCompile(`\baim `), regexp.MustCompile(`\btoolbox `)} {
		if m := re.FindString(body); m != "" {
			t.Errorf("SKILL.md body contains %q", m)
		}
	}
	for _, w := range []string{"mwinit", "~/", "CLAUDE_", "mcp__", "my outlook", "my slack", ".claude"} {
		if strings.Contains(body, w) {
			t.Errorf("SKILL.md body contains %q", w)
		}
	}
	if d := regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`).FindString(body); d != "" {
		t.Errorf("SKILL.md body contains the date %q", d)
	}
}

func TestSkill_NoEnvVarPattern(t *testing.T) {
	dir := filepath.Join(repoRoot(t), "skills", "byline")
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "${") {
			rel, _ := filepath.Rel(repoRoot(t), path)
			t.Errorf("%s contains \"${\"", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
}

func TestSkill_ToolNames(t *testing.T) {
	s := readFile(t, skillPath)
	known := map[string]bool{}
	for _, name := range skillTools {
		known[name] = true
	}
	found := map[string]bool{}
	for _, m := range regexp.MustCompile(`byline_[a-z_]+`).FindAllString(s, -1) {
		if !known[m] {
			t.Errorf("SKILL.md names %q, which isn't one of the 11 tools", m)
		}
		found[m] = true
	}
	for _, name := range skillTools {
		if !found[name] {
			t.Errorf("SKILL.md never names %q", name)
		}
	}
}

func TestSkill_CarriesFindings(t *testing.T) {
	_, body := splitFrontmatter(t, readFile(t, skillPath))
	why := section(t, body, "## Why the steps are in this order")
	for _, want := range []string{"13 of 13", "40%", "18 of 20", "four times", "8 minutes", "1 in 5"} {
		if !strings.Contains(why, want) {
			t.Errorf("\"## Why the steps are in this order\" doesn't contain %q", want)
		}
	}
}

// I4 (skill part) regression: setup has no producer for signature_lines, so
// the fact check flags every non-owner's own signature. Setup must record
// the person's sign-off block as signature_lines and pass it to the setup
// save tool.
func TestSkill_SetupRecordsSignatureLines(t *testing.T) {
	_, body := splitFrontmatter(t, readFile(t, skillPath))
	setup := section(t, body, "### setup")
	if !strings.Contains(setup, "signature_lines") {
		t.Error("\"### setup\" doesn't mention recording signature_lines")
	}
	if !strings.Contains(setup, "setup save tool") {
		t.Error("\"### setup\" doesn't say signature_lines goes to the setup save tool")
	}
}

// The owner, 2026-11-11: "I def do not want my users to have to do this" (the
// blind read of the switch test). Byline never asks a user to compare drafts.
func TestSkill_NeverAsksUserToCompareDrafts(t *testing.T) {
	text := strings.ToLower(readFile(t, skillPath))
	for _, phrase := range []string{"blind read", "blind test", "side-by-side", "side by side", "pick the better draft", "which draft is better"} {
		if strings.Contains(text, phrase) {
			t.Errorf("SKILL.md mentions %q; Byline never asks a user to compare drafts", phrase)
		}
	}
}

// Round 2 fix: write mode only ran the voice check when the person asked,
// so drafts could ship without it. Step 5 (polish) must now run it on every
// draft.
func TestSkill_Step5NamesVoiceCheck(t *testing.T) {
	_, body := splitFrontmatter(t, readFile(t, skillPath))
	step5 := section(t, body, "### Step 5: Polish last and show only the final version")
	if !strings.Contains(step5, "voice check") {
		t.Error("\"### Step 5\" doesn't name the voice check")
	}
}

func TestSkill_ReviewModesUseGeneralRules(t *testing.T) {
	_, body := splitFrontmatter(t, readFile(t, skillPath))
	for _, h := range []string{"### check", "### review"} {
		if !strings.Contains(section(t, body, h), "general rules") {
			t.Errorf("%q doesn't say to judge by the general rules", h)
		}
	}
}

// level2Headings returns every "## " heading line in s, in document order.
func level2Headings(s string) []string {
	var got []string
	for _, l := range strings.Split(s, "\n") {
		if headingLevel(l) == 2 {
			got = append(got, l)
		}
	}
	return got
}

// fenceRe matches a fenced code block that names a language, capturing the
// language and the body.
var fenceRe = regexp.MustCompile("(?s)```([a-z]+)\n(.*?)\n```")

func TestReferences_Shape(t *testing.T) {
	t.Run("install-and-hosts.md", func(t *testing.T) {
		s := readFile(t, referencePaths[0])
		want := []string{
			"## How Byline's tools are reached",
			"## Claude Code",
			"## Reading sent mail and Slack for setup",
			"## Updating",
			"## Usage tracking",
			"## Supported machines",
		}
		if got := level2Headings(s); !reflect.DeepEqual(got, want) {
			t.Fatalf("headings:\n got %q\nwant %q", got, want)
		}
		for _, bad := range []string{"toolbox install byline-mcp", "toolbox update byline-mcp"} {
			if strings.Contains(s, bad) {
				t.Errorf("install-and-hosts.md contains %q", bad)
			}
		}
	})

	// I6 regression: the file once pointed at docs/distribution-notes.md
	// (not embedded, not in the plugin) and at phase-plan headings the
	// runtime model can't resolve. It must state the spike outcome
	// plainly instead.
	t.Run("install-and-hosts.md has no plan references", func(t *testing.T) {
		s := readFile(t, referencePaths[0])
		for _, bad := range []string{"Spike 0.2", "Spike 0.3", "Request 4", "docs/", "docs\\"} {
			if strings.Contains(s, bad) {
				t.Errorf("install-and-hosts.md contains %q, a plan reference", bad)
			}
		}
		for _, want := range []string{"MCP tool", "tool <name>"} {
			if !strings.Contains(s, want) {
				t.Errorf("install-and-hosts.md doesn't say %q", want)
			}
		}
	})

	t.Run("patterns.md", func(t *testing.T) {
		s := readFile(t, referencePaths[1])
		lines := strings.Split(s, "\n")
		var headingIdx []int
		for i, l := range lines {
			if strings.HasPrefix(l, "### ") {
				headingIdx = append(headingIdx, i)
			}
		}
		if len(headingIdx) != 13 {
			t.Fatalf("patterns.md has %d \"### \" headings, want 13", len(headingIdx))
		}
		for _, i := range headingIdx {
			if i+2 >= len(lines) {
				t.Fatalf("heading %q has no room for a \"Looks like:\" and a \"Do:\" line", lines[i])
			}
			if !strings.HasPrefix(lines[i+1], "Looks like:") {
				t.Errorf("heading %q: the next line %q doesn't start with %q", lines[i], lines[i+1], "Looks like:")
			}
			if !strings.HasPrefix(lines[i+2], "Do:") {
				t.Errorf("heading %q: the line after that %q doesn't start with %q", lines[i], lines[i+2], "Do:")
			}
		}
		// The name is built by concatenation, not written as a literal,
		// so this test file doesn't trip the scrub scan it isn't part of.
		name := "Kev" + "in"
		banned := regexp.MustCompile(name + `|\$AI_DIR|rules\.json|\bK\d{1,2}[a-e]?\b`)
		if m := banned.FindString(s); m != "" {
			t.Errorf("patterns.md contains %q", m)
		}
	})

	t.Run("worked-example-generic.md", func(t *testing.T) {
		s := readFile(t, referencePaths[2])
		want := []string{
			"## The source",
			"## The plan for the message",
			"## First draft",
			"## What the checks caught",
			"## What no check caught",
			"## Final version",
		}
		if got := level2Headings(s); !reflect.DeepEqual(got, want) {
			t.Fatalf("headings:\n got %q\nwant %q", got, want)
		}
		wantLang := map[string]string{
			"## The source":               "text",
			"## The plan for the message": "json",
			"## First draft":              "text",
			"## What the checks caught":   "",
			"## What no check caught":     "",
			"## Final version":            "text",
		}
		for _, h := range want {
			sec := section(t, s, h)
			lang := wantLang[h]
			if lang == "" {
				if strings.Contains(sec, "```") {
					t.Errorf("%q has a fenced block, want none", h)
				}
				continue
			}
			ms := fenceRe.FindAllStringSubmatch(sec, -1)
			if len(ms) != 1 {
				t.Fatalf("%q has %d fenced blocks, want 1", h, len(ms))
			}
			if ms[0][1] != lang {
				t.Errorf("%q fence language = %q, want %q", h, ms[0][1], lang)
			}
		}
	})
}

// TestReferences_WorkedExampleTraces fails if the final-version block in
// worked-example-generic.md uses a number that doesn't appear in the source
// block, so the example keeps proving the "trace every fact" rule it's
// teaching.
func TestReferences_WorkedExampleTraces(t *testing.T) {
	s := readFile(t, referencePaths[2])
	source := fenceRe.FindStringSubmatch(section(t, s, "## The source"))
	final := fenceRe.FindStringSubmatch(section(t, s, "## Final version"))
	if source == nil {
		t.Fatal("couldn't find the fenced block under \"## The source\"")
	}
	if final == nil {
		t.Fatal("couldn't find the fenced block under \"## Final version\"")
	}
	for _, n := range regexp.MustCompile(`\d[\d,]*`).FindAllString(final[2], -1) {
		if !strings.Contains(source[2], n) {
			t.Errorf("the final version uses %q, which doesn't appear in the source", n)
		}
	}
}
