package gate

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"byline/internal/draftcheck"
	"byline/internal/paths"
)

// Task 3.5's send and structure-check fixtures. person and bot are
// placeholder Slack channel/user ids shaped so they read as fakes, not
// real ones: bot matches newHome's one exempt channel.
const (
	person = "TESTPERSON1"
	bot    = "TESTBOT0001"
)

const bodyMD = `Hi, the platform team gave you the heads up Monday. Here's the short version.

**What's driving it**

The finance group is running this. Two audits found gaps in how monthly reports are filed, and leadership expects a fix this quarter.

**What we're being asked to do**

The main build is a tool that copies every monthly report from the old shared drive into the new archive, across roughly 1,400 folders.

**Effort**

I don't know yet, and there are a lot of unknowns. Across the 26 teams in the pilot there are twelve different filing layouts, so there's no single import to write.
`

var bodySlack = strings.ReplaceAll(bodyMD, "**", "*")

var boldMD = regexp.MustCompile(`\*\*([^*]+)\*\*`)

// bodyHTML wraps each paragraph of bodyMD in <p>, turns **x** into <b>x</b>,
// and adds a head and a signature.
func bodyHTML() string {
	var b strings.Builder
	b.WriteString("<html><head><style>body{font-family:Calibri}</style></head><body>")
	for _, p := range strings.Split(strings.TrimRight(bodyMD, "\n"), "\n\n") {
		b.WriteString("<p>")
		b.WriteString(boldMD.ReplaceAllString(p, "<b>$1</b>"))
		b.WriteString("</p>")
	}
	b.WriteString("<p><b>The Sender</b> | Example Team</p><p>Example Org</p></body></html>")
	return b.String()
}

const longUnchecked = "Hi, a completely different and unchecked message about the quarterly budget reconciliation and the vendor matching work that nobody has reviewed today at any point."
const shortReal = "Some entirely unchecked reply copy about warehouse racking inspections and vendor invoices that nobody has verified or reviewed at any point."

// addReceipt writes text as a draft with a "{}" spec to a fresh temp
// folder, writes a receipt for it through draftcheck.WriteReceipt, and
// moves the receipt file's mtime back by age so bestCoverage's lifetime
// check can be exercised.
func addReceipt(t *testing.T, text string, age time.Duration) {
	t.Helper()
	dir := t.TempDir()
	draft := filepath.Join(dir, "draft.md")
	spec := filepath.Join(dir, "spec.json")
	if err := os.WriteFile(draft, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	when := time.Now().Add(-age)
	receiptPath, err := draftcheck.WriteReceipt(draft, spec, text, when)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(receiptPath, when, when); err != nil {
		t.Fatal(err)
	}
}

// newHome points BYLINE_HOME at a fresh temp folder, writes the three
// voice files and a settings.json with one exempt channel, and returns
// the home path.
func newHome(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), ".byline")
	t.Setenv("BYLINE_HOME", home)
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile := func(path, content string) {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(paths.BriefPath(), "# Brief\n")
	writeFile(paths.ProfilePath(), "# Profile\n")
	writeFile(paths.ExemplarsPath(), "# Samples\n")
	writeFile(paths.SettingsPath(), `{"format":1,"exempt_channels":["TESTBOT0001"]}`)
	return home
}

// hook runs mode through Run with payload as the stdin body: a string is
// sent as is (so a test can send malformed JSON), anything else is
// marshaled first. It returns the decoded reply and the raw stdout.
func hook(t *testing.T, mode string, payload any) (hookOutput, string) {
	t.Helper()
	body, ok := payload.(string)
	if !ok {
		body = mustJSON(payload)
	}
	var out bytes.Buffer
	Run(mode, strings.NewReader(body), &out)
	var o hookOutput
	raw := out.String()
	if raw != "" {
		if err := json.Unmarshal(out.Bytes(), &o); err != nil {
			t.Fatalf("hook %s: stdout %q isn't valid JSON: %v", mode, raw, err)
		}
	}
	return o, raw
}

// dec returns a hookOutput's permission decision ("allow", "deny", "ask",
// or "" when none was set).
func dec(o hookOutput) string {
	return o.HookSpecificOutput.PermissionDecision
}

// mustJSON marshals v, panicking on error. Every caller passes a value
// that always marshals.
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// readLine returns one JSONL transcript line recording a real Read tool
// call of filePath, in the shape Claude Code's transcript uses.
func readLine(filePath string) string {
	return mustJSON(map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"content": []map[string]any{{
				"type": "tool_use",
				"id":   "x",
				"name": "Read",
				"input": map[string]any{
					"file_path": filePath,
				},
			}},
		},
	})
}

// polish returns the pre-tool payload for the polish pass, with
// session_id set only when id isn't empty.
func polish(id string) map[string]any {
	p := map[string]any{"tool_name": "Skill", "tool_input": map[string]any{"skill": "byline", "args": "polish"}}
	if id != "" {
		p["session_id"] = id
	}
	return p
}

// writeLines writes lines joined by "\n" to a new temp file and returns
// its path.
func writeLines(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
