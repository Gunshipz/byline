package testpackaging

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"byline/internal/state"
)

// hooksDoc is Claude Code's native hooks format, as hooks/hooks.json holds it.
type hooksDoc struct {
	Hooks map[string][]hookGroup `json:"hooks"`
}

type hookGroup struct {
	Matcher string      `json:"matcher"`
	Hooks   []hookEntry `json:"hooks"`
}

type hookEntry struct {
	Type      string   `json:"type"`
	Command   string   `json:"command"`
	Args      []string `json:"args"`
	Timeout   int      `json:"timeout"`
	OnFailure *string  `json:"onFailure"`
	If        string   `json:"if"`
}

// loadHooks reads hooks/hooks.json from the repo root: the file Claude Code
// loads.
func loadHooks(t *testing.T) hooksDoc {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "hooks", "hooks.json"))
	if err != nil {
		t.Fatalf("read hooks/hooks.json: %v", err)
	}
	var doc hooksDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse hooks/hooks.json: %v", err)
	}
	return doc
}

func splitPreToolGroups(t *testing.T, groups []hookGroup) (group1, group2, skill hookGroup) {
	t.Helper()
	var sawGroup1, sawGroup2, sawSkill bool
	for _, g := range groups {
		switch g.Matcher {
		case "^mcp__.*":
			group2, sawGroup2 = g, true
		case "^Skill$":
			skill, sawSkill = g, true
		default:
			group1, sawGroup1 = g, true
		}
	}
	if !sawGroup1 || !sawGroup2 || !sawSkill {
		t.Fatalf("PreToolUse has %d groups, want the send-tools, mcp__.* and Skill groups", len(groups))
	}
	return
}

func TestHooks_TwoSendGroups(t *testing.T) {
	doc := loadHooks(t)
	pre, ok := doc.Hooks["PreToolUse"]
	if !ok {
		t.Fatal("hooks.json has no PreToolUse")
	}
	group1, group2, _ := splitPreToolGroups(t, pre)

	nonMatching := []string{
		"mcp__my-chat__send_message",
		"mcp__slack__conversations_history",
		"mcp__claude_ai_Microsoft_365__outlook_read_mail",
		"mcp__slack__conversations_add_message_v2",
		"Read",
		"Bash",
		"Skill",
	}

	for _, tool := range state.DefaultSendTools {
		wrapped := "mcp__plugin_Some-Plugin-all_" + strings.TrimPrefix(tool, "mcp__")
		for _, name := range []string{tool, wrapped} {
			full, search := matches(group1.Matcher, name)
			if !full || !search {
				t.Errorf("group1 matcher %q: %q full=%v search=%v, want both true", group1.Matcher, name, full, search)
			}
		}
	}
	for _, name := range nonMatching {
		full, search := matches(group1.Matcher, name)
		if full || search {
			t.Errorf("group1 matcher %q: %q full=%v search=%v, want both false", group1.Matcher, name, full, search)
		}
	}

	if group2.Matcher != "^mcp__.*" {
		t.Fatalf("group2 matcher = %q, want %q", group2.Matcher, "^mcp__.*")
	}
	if len(group2.Hooks) != 1 || !strings.HasSuffix(group2.Hooks[0].Command, "hook pre-tool --extra") {
		t.Fatalf("group2 command = %+v, want it to end \"hook pre-tool --extra\"", group2.Hooks)
	}
	if group2.Hooks[0].OnFailure != nil {
		t.Fatalf("group2 has onFailure = %q, want none", *group2.Hooks[0].OnFailure)
	}
	always := append(append([]string{}, state.DefaultSendTools...), "mcp__my-chat__send_message")
	for _, name := range always {
		full, search := matches(group2.Matcher, name)
		if !full || !search {
			t.Errorf("group2 matcher %q: %q full=%v search=%v, want both true", group2.Matcher, name, full, search)
		}
	}

	root := repoRoot(t)
	hookScript, err := os.ReadFile(filepath.Join(root, "bin", "byline-hook"))
	if err != nil {
		t.Fatalf("read bin/byline-hook: %v", err)
	}
	sendToolsRe := regexp.MustCompile(`send_tools="([^"]*)"`)
	m := sendToolsRe.FindStringSubmatch(string(hookScript))
	if m == nil {
		t.Fatal("bin/byline-hook has no send_tools=\"...\" line")
	}
	got := strings.Fields(m[1])
	if !slices.Equal(got, state.DefaultSendTools) {
		t.Fatalf("byline-hook's send_tools = %v, want %v", got, state.DefaultSendTools)
	}
}

// TestHooks_OnlySendGatesBlockOnHookFailure is the send gates' fail-closed
// proof: the send-tools hook, and only it, carries Claude Code's "onFailure": "block" on the command hook
// itself, so a launcher that can't start, times out, exits other than 0
// or 2 or prints bad JSON blocks the send instead of letting it through.
func TestHooks_OnlySendGatesBlockOnHookFailure(t *testing.T) {
	doc := loadHooks(t)
	group1, _, _ := splitPreToolGroups(t, doc.Hooks["PreToolUse"])
	var withOnFailure []hookEntry
	for event, groups := range doc.Hooks {
		for _, g := range groups {
			for _, h := range g.Hooks {
				if h.OnFailure != nil {
					withOnFailure = append(withOnFailure, h)
					if event != "PreToolUse" || g.Matcher != group1.Matcher {
						t.Errorf("onFailure is on %s %q, want only the PreToolUse send-tools group", event, g.Matcher)
					}
				}
			}
		}
	}
	if len(withOnFailure) != 1 {
		t.Fatalf("found %d hooks with onFailure, want exactly 1: %+v", len(withOnFailure), withOnFailure)
	}
	for _, h := range withOnFailure {
		if *h.OnFailure != "block" {
			t.Errorf("onFailure = %q, want %q", *h.OnFailure, "block")
		}
		if h.Timeout != 30 {
			t.Errorf("timeout = %d, want 30", h.Timeout)
		}
	}
}

func TestHooks_SkillGatedWithoutBlock(t *testing.T) {
	doc := loadHooks(t)
	pre, ok := doc.Hooks["PreToolUse"]
	if !ok {
		t.Fatal("hooks.json has no PreToolUse")
	}
	_, _, skill := splitPreToolGroups(t, pre)
	for _, h := range skill.Hooks {
		if h.OnFailure != nil {
			t.Errorf("Skill group hook has onFailure = %q, want none", *h.OnFailure)
		}
	}
	full, _ := matches(skill.Matcher, "Skill")
	if !full {
		t.Errorf("Skill group matcher %q does not match %q", skill.Matcher, "Skill")
	}

	for _, g := range pre {
		full, _ := matches(g.Matcher, "SkillX")
		if full {
			t.Errorf("PreToolUse matcher %q matches %q, want none to", g.Matcher, "SkillX")
		}
	}
}

func TestHooks_GateCommandsUseLauncher(t *testing.T) {
	doc := loadHooks(t)
	for event, groups := range doc.Hooks {
		for _, g := range groups {
			for _, h := range g.Hooks {
				if len(h.Args) != 0 {
					t.Errorf("%s %q: gate hook has args %v, want none", event, g.Matcher, h.Args)
				}
				var mode string
				switch event {
				case "SessionStart":
					mode = "session-start"
				case "UserPromptSubmit":
					mode = "prompt"
				case "PostToolUse":
					mode = "read"
				case "PreToolUse":
					if strings.HasSuffix(h.Command, "hook pre-tool --extra") {
						mode = "pre-tool --extra"
					} else {
						mode = "pre-tool"
					}
				default:
					t.Fatalf("unexpected event %q", event)
				}
				want := `sh "${CLAUDE_PLUGIN_ROOT}/bin/byline-hook" hook ` + mode
				if h.Command != want {
					t.Errorf("%s %q command = %q, want %q", event, g.Matcher, h.Command, want)
				}
			}
		}
	}
}

func TestHooks_GateCommandQuotesPluginRoot(t *testing.T) {
	binDir := stage(t, "plugin root")
	pluginRoot := filepath.Dir(binDir)

	doc := loadHooks(t)
	pre, ok := doc.Hooks["PreToolUse"]
	if !ok {
		t.Fatal("hooks.json has no PreToolUse")
	}
	group1, group2, skill := splitPreToolGroups(t, pre)

	runSH := func(command, stdin string) (stdout string, code int) {
		cmd := exec.Command("sh", "-c", command)
		cmd.Stdin = strings.NewReader(stdin)
		var out bytes.Buffer
		cmd.Stdout = &out
		err := cmd.Run()
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else {
				t.Fatalf("run %q: %v", command, err)
			}
		}
		return out.String(), code
	}
	sub := func(command string) string {
		return strings.ReplaceAll(command, "${CLAUDE_PLUGIN_ROOT}", pluginRoot)
	}

	cases := []struct {
		name    string
		command string
		stdin   string
		want    string
	}{
		{"group1 send", sub(group1.Hooks[0].Command), sendPayload, askLine + "\n"},
		{"group2 send", sub(group2.Hooks[0].Command), sendPayload, ""},
		{"group2 other mcp", sub(group2.Hooks[0].Command), `{"tool_name":"mcp__my-chat__send_message"}`, ""},
		{"skill", sub(skill.Hooks[0].Command), skillPayload, ""},
	}
	for _, c := range cases {
		stdout, code := runSH(c.command, c.stdin)
		if code != 0 {
			t.Errorf("%s: exit code = %d, want 0", c.name, code)
		}
		if stdout != c.want {
			t.Errorf("%s: stdout = %q, want %q", c.name, stdout, c.want)
		}
	}
}

// matches compiles pattern anchored (^(?:pattern)$) and as given, and
// reports whether name matches each form.
func matches(pattern, name string) (full, search bool) {
	fullRe := regexp.MustCompile(`^(?:` + pattern + `)$`)
	searchRe := regexp.MustCompile(pattern)
	return fullRe.MatchString(name), searchRe.MatchString(name)
}
