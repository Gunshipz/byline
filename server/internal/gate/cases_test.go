// cases_test.go ports today's 49 hook test cases (writing-rules-gate.test.sh's
// 33 and draftcheck-gate.test.mjs's 16) onto the new gate, as one shared table
// both TestGate_PortedCases (this file) and TestGate_ParityWithOldHooks
// (parity_test.go) walk. Every value a real person would recognize --
// channel ids, a bot id, a profile file name -- is read from the old scripts
// at run time (see oldHooks in parity_test.go); nothing personal is a
// literal in this file.
package gate

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"byline/internal/paths"
)

// Prompts and tool names used across the table, named the way the brief
// names them.
const (
	draftReplyPrompt = "draft a reply to the vendor about the rate limits"
	draftSlackPrompt = "draft a slack message to the vendor"
	draftMsgPrompt   = "draft a message to the vendor"

	toolEmailDraft = "mcp__claude_ai_Microsoft_365__outlook_create_draft"
	toolEmailSend  = "mcp__claude_ai_Microsoft_365__outlook_send_mail"
	toolEmailReply = "mcp__claude_ai_Microsoft_365__outlook_create_reply_draft"
	toolSlackPost  = "mcp__slack__conversations_add_message"
)

// step is one hook call rendered from a gateCase.
type step struct {
	mode       string         // "session-start" | "prompt" | "read" | "pre-tool"
	text       string         // prompt text, session-start source, or raw stdin for the malformed case
	file       string         // read: "brief" | "profile" | "exemplars"
	tool       string         // pre-tool tool name; "Skill:polish" and "Skill:other" are rendered per side
	input      map[string]any // tool_input; the values "<bot>", "<person>" and "<bodyfile>" are rendered per side
	transcript string         // "" | "mentions" | "reads"
	raw        bool           // send text as raw stdin
}

// gateCase is one ported test case, run by both runCaseNew and runCaseOld.
type gateCase struct {
	name     string // the old test's name (renames noted in the brief)
	origin   string // "wrg §N" or "dcg #N"
	session  bool   // payloads carry session_id "case-<row>"
	receipt  string // "" | "fresh" | "aged5h": a receipt for bodyMD written before the last step
	steps    []step
	want     string   // last pre-tool step: "" | "allow" | "ask" | "deny"; last prompt or session-start step: "armed" | "unarmed"
	wantText []string // substrings the last reason must contain
}

// outcome is what one case run settles to, from its last step.
type outcome struct {
	decision string
	armed    bool
	injected bool
	reason   string
}

// step constructors, one per mode.
func stepPrompt(text string) step  { return step{mode: "prompt", text: text} }
func stepStart(source string) step { return step{mode: "session-start", text: source} }
func stepRead(file string) step    { return step{mode: "read", file: file} }
func stepTool(tool string, input map[string]any) step {
	return step{mode: "pre-tool", tool: tool, input: input}
}
func stepToolLog(tool string, input map[string]any, transcript string) step {
	return step{mode: "pre-tool", tool: tool, input: input, transcript: transcript}
}
func stepRaw(text string) step { return step{mode: "pre-tool", raw: true, text: text} }

// Today's live PreToolUse matchers, copied from ~/.claude/settings.json on
// 2026-11-10 and anchored.
var oldLiveRoutes = map[string]*regexp.Regexp{
	"writing-rules-gate.mjs": regexp.MustCompile(`^(Skill|mcp__slack__conversations_add_message|mcp__claude_ai_Microsoft_365__outlook_(send|draft|reply|forward))$`),
	"draftcheck-gate.mjs":    regexp.MustCompile(`^(Skill|mcp__slack__conversations_add_message|mcp__claude_ai_Microsoft_365__outlook_(send|draft|reply|forward))$`),
	"slack-post-gate.mjs":    regexp.MustCompile(`^mcp__slack__conversations_add_message$`),
}

var portedCases = []gateCase{
	// -- wrg §1: arming ------------------------------------------------
	{name: "draft prompt arms", origin: "wrg §1", session: true,
		steps: []step{stepPrompt(draftReplyPrompt)}, want: "armed"},
	{name: "non-draft prompt does not arm", origin: "wrg §1", session: true,
		steps: []step{stepPrompt("what did the vendor say about the new accounts")}, want: "unarmed"},
	{name: "pipeline-meta prompt does not arm", origin: "wrg §1", session: true,
		steps: []step{stepPrompt("create a hook that forces you to use the humanizer before you draft a message")}, want: "unarmed"},
	{name: "formal-document prompt does not arm", origin: "wrg §1", session: true,
		steps: []step{stepPrompt("write a 6-pager on the filing archive")}, want: "unarmed"},
	{name: "system-reminder notification does not arm", origin: "wrg §1", session: true,
		steps: []step{stepPrompt("<system-reminder>\nAgent \"drafter\" completed: " + draftReplyPrompt + "\n</system-reminder>")}, want: "unarmed"},
	{name: "SYSTEM NOTIFICATION does not arm", origin: "wrg §1", session: true,
		steps: []step{stepPrompt("[SYSTEM NOTIFICATION - NOT USER INPUT] Agent finished. Next step: " + draftReplyPrompt)}, want: "unarmed"},
	{name: "task-notification does not arm", origin: "wrg §1", session: true,
		steps: []step{stepPrompt("<task-notification><status>completed</status><result>" + draftReplyPrompt + "</result></task-notification>")}, want: "unarmed"},

	// -- wrg §2: gate blocks while armed with no rules loaded ----------
	{name: "Write not gated, as live today", origin: "wrg §2", session: true,
		steps: []step{stepPrompt(draftSlackPrompt), stepTool("Write", map[string]any{})}, want: ""},
	{name: "Edit not gated, as live today", origin: "wrg §2", session: true,
		steps: []step{stepPrompt(draftSlackPrompt), stepTool("Edit", map[string]any{})}, want: ""},
	{name: "NotebookEdit not gated, as live today", origin: "wrg §2", session: true,
		steps: []step{stepPrompt(draftSlackPrompt), stepTool("NotebookEdit", map[string]any{})}, want: ""},
	{name: "Agent not gated, as live today", origin: "wrg §2", session: true,
		steps: []step{stepPrompt(draftSlackPrompt), stepTool("Agent", map[string]any{})}, want: ""},
	{name: "slack post blocked", origin: "wrg §2", session: true,
		steps: []step{stepPrompt(draftSlackPrompt), stepTool(toolSlackPost, map[string]any{})}, want: "deny"},
	{name: "draft tool blocked", origin: "wrg §2", session: true,
		steps: []step{stepPrompt(draftSlackPrompt), stepTool(toolEmailDraft, map[string]any{})}, want: "deny"},
	{name: "Skill(polish) blocked", origin: "wrg §2", session: true,
		steps: []step{stepPrompt(draftSlackPrompt), stepTool("Skill:polish", nil)}, want: "deny"},

	// -- wrg §3: non-drafting tools pass through even while armed ------
	{name: "Read allowed", origin: "wrg §3", session: true,
		steps: []step{stepPrompt(draftSlackPrompt), stepTool("Read", map[string]any{})}, want: ""},
	{name: "Grep allowed", origin: "wrg §3", session: true,
		steps: []step{stepPrompt(draftSlackPrompt), stepTool("Grep", map[string]any{})}, want: ""},
	{name: "Glob allowed", origin: "wrg §3", session: true,
		steps: []step{stepPrompt(draftSlackPrompt), stepTool("Glob", map[string]any{})}, want: ""},
	{name: "Bash allowed", origin: "wrg §3", session: true,
		steps: []step{stepPrompt(draftSlackPrompt), stepTool("Bash", map[string]any{})}, want: ""},
	{name: "WebFetch allowed", origin: "wrg §3", session: true,
		steps: []step{stepPrompt(draftSlackPrompt), stepTool("WebFetch", map[string]any{})}, want: ""},
	{name: "mcp__slack__conversations_search_messages allowed", origin: "wrg §3", session: true,
		steps: []step{stepPrompt(draftSlackPrompt), stepTool("mcp__slack__conversations_search_messages", map[string]any{})}, want: ""},
	{name: "Skill(other) allowed", origin: "wrg §3", session: true,
		steps: []step{stepPrompt(draftSlackPrompt), stepTool("Skill:other", nil)}, want: ""},

	// -- wrg §4: gate opens after all three files are read --------------
	{name: "blocked before any rules", origin: "wrg §4", session: true,
		steps: []step{stepPrompt(draftSlackPrompt), stepTool(toolEmailSend, map[string]any{})}, want: "deny"},
	{name: "still blocked with 2 of 3", origin: "wrg §4", session: true,
		steps: []step{stepPrompt(draftSlackPrompt), stepRead("brief"), stepRead("exemplars"), stepTool(toolEmailSend, map[string]any{})}, want: "deny"},
	{name: "allowed with all 3", origin: "wrg §4", session: true,
		steps: []step{stepPrompt(draftSlackPrompt), stepRead("brief"), stepRead("exemplars"), stepRead("profile"), stepTool(toolEmailSend, map[string]any{"body": "Thanks, done."})}, want: ""},
	{name: "Skill(polish) allowed with all 3", origin: "wrg §4", session: true,
		steps: []step{stepPrompt(draftSlackPrompt), stepRead("brief"), stepRead("exemplars"), stepRead("profile"), stepTool(toolEmailSend, map[string]any{"body": "Thanks, done."}), stepTool("Skill:polish", nil)}, want: ""},

	// -- wrg §5: not armed = stay out of the way -------------------------
	{name: "Write allowed when unarmed", origin: "wrg §5", session: true,
		steps: []step{stepTool("Write", map[string]any{})}, want: ""},
	{name: "Agent allowed when unarmed", origin: "wrg §5", session: true,
		steps: []step{stepTool("Agent", map[string]any{})}, want: ""},
	{name: "mcp__slack__conversations_add_message allowed when unarmed", origin: "wrg §5", session: true,
		steps: []step{stepTool(toolSlackPost, map[string]any{})}, want: "ask"},

	// -- wrg §6: fail-closed on malformed payload ------------------------
	{name: "malformed payload denies", origin: "wrg §6", session: true,
		steps: []step{stepRaw("not json")}, want: "deny"},

	// -- wrg §7: reset clears state ---------------------------------------
	{name: "reset clears armed", origin: "wrg §7", session: true,
		steps: []step{stepPrompt(draftMsgPrompt), stepStart("clear")}, want: "unarmed"},
	{name: "compact keeps armed", origin: "wrg §7", session: true,
		steps: []step{stepPrompt(draftMsgPrompt), stepStart("compact")}, want: "armed"},

	// -- wrg §8: transcript fallback counts real Read calls, not mentions -
	{name: "mentions of the files in the log do not count as reads", origin: "wrg §8", session: true,
		steps: []step{stepPrompt(draftMsgPrompt), stepToolLog(toolEmailSend, map[string]any{}, "mentions")}, want: "deny"},
	{name: "real Read calls in the log count when the record was missed", origin: "wrg §8", session: true,
		steps: []step{stepPrompt(draftMsgPrompt), stepToolLog(toolEmailSend, map[string]any{"body": "Thanks, done."}, "reads")}, want: ""},

	// -- dcg: draftcheck-gate ----------------------------------------------
	{name: "blocks Slack post with no receipt", origin: "dcg #1",
		steps: []step{stepTool(toolSlackPost, map[string]any{"channel": "<person>", "text": bodySlack})}, want: "deny"},
	{name: "allows Slack post after draftcheck passes", origin: "dcg #2", receipt: "fresh",
		steps: []step{stepTool(toolSlackPost, map[string]any{"channel": "<person>", "text": bodySlack})}, want: "ask"},
	{name: "allows email_send with HTML body and signature", origin: "dcg #3", receipt: "fresh",
		steps: []step{stepTool(toolEmailSend, map[string]any{"to": []string{"someone@example.com"}, "subject": "x", "body": bodyHTML()})}, want: ""},
	{name: "blocks different unchecked copy while a receipt exists", origin: "dcg #4", receipt: "fresh",
		steps: []step{stepTool(toolSlackPost, map[string]any{"channel": "<person>", "text": longUnchecked})}, want: "deny"},
	{name: "allows a one-line thanks", origin: "dcg #5", receipt: "fresh",
		steps: []step{stepTool(toolSlackPost, map[string]any{"channel": "<person>", "text": "Thanks, got it."})}, want: "ask"},
	{name: "gates a real short reply (~22 words)", origin: "dcg #6",
		steps: []step{stepTool(toolEmailReply, map[string]any{"itemId": "x", "itemChangeKey": "y", "body": shortReal, "saveDraft": true})}, want: "deny"},
	{name: "allows a short single-sentence acknowledgement", origin: "dcg #6b",
		steps: []step{stepTool(toolSlackPost, map[string]any{"channel": "<person>", "text": "Thanks, I will take a look at it today."})}, want: "ask"},
	{name: "gates a short multi-sentence note", origin: "dcg #6b",
		steps: []step{stepTool(toolSlackPost, map[string]any{"channel": "<person>", "text": "Reminder to send the vendor docs. Rough is fine, no formatting needed at all."})}, want: "deny"},
	{name: "allows an emoji or link only post", origin: "dcg #6b",
		steps: []step{stepTool(toolSlackPost, map[string]any{"channel": "<person>", "text": ":+1:"})}, want: "ask"},
	{name: "blocks when the only receipt is 5 hours old", origin: "dcg #7", receipt: "aged5h",
		steps: []step{stepTool(toolSlackPost, map[string]any{"channel": "<person>", "text": bodySlack})}, want: "deny"},
	{name: "allows posts to a bot chat on the exempt list", origin: "dcg #8",
		steps: []step{stepTool(toolSlackPost, map[string]any{"channel": "<bot>", "text": longUnchecked})}, want: "allow"},
	{name: "allows a short draft", origin: "dcg #9",
		steps: []step{stepTool(toolEmailDraft, map[string]any{"body": "Thanks, done."})}, want: ""},
	{name: "ignores unrelated tools", origin: "dcg #10",
		steps: []step{stepTool("Read", map[string]any{"file_path": "/tmp/x"})}, want: ""},
	{name: "blocks a draft via bodyFilePath with no receipt", origin: "dcg #11",
		steps: []step{stepTool(toolEmailDraft, map[string]any{"operation": "create", "bodyFilePath": "<bodyfile>"})}, want: "deny"},
	{name: "allows a draft via bodyFilePath once checked", origin: "dcg #11", receipt: "fresh",
		steps: []step{stepTool(toolEmailDraft, map[string]any{"operation": "create", "bodyFilePath": "<bodyfile>"})}, want: ""},
	{name: "deny reason names the structure check and the spec fields", origin: "dcg #12",
		steps: []step{stepTool(toolSlackPost, map[string]any{"channel": "<person>", "text": bodySlack})}, want: "deny",
		wantText: []string{"structure check", "questions", "kill list"}},
}

func TestGate_PortedCases(t *testing.T) {
	if len(portedCases) != 49 {
		t.Fatalf("want today's 49 cases, have %d", len(portedCases))
	}
	for i, c := range portedCases {
		t.Run(c.name, func(t *testing.T) {
			newHome(t) // exempt_channels ["TESTBOT0001"]
			got := runCaseNew(t, i+1, c, bot)
			switch c.want {
			case "armed":
				if !got.armed || !got.injected && c.steps[len(c.steps)-1].mode == "prompt" {
					t.Fatalf("want armed, got %+v", got)
				}
			case "unarmed":
				if got.armed || got.injected {
					t.Fatalf("want unarmed, got %+v", got)
				}
			default:
				if got.decision != c.want {
					t.Fatalf("want %q, got %+v", c.want, got)
				}
			}
			for _, s := range c.wantText {
				if !strings.Contains(got.reason, s) {
					t.Errorf("reason lacks %q", s)
				}
			}
		})
	}
}

// runCaseNew runs c against the new gate (through Run, exactly as Claude
// Code would), returning the outcome of its last step.
func runCaseNew(t *testing.T, row int, c gateCase, botID string) outcome {
	t.Helper()
	switch c.receipt {
	case "fresh":
		addReceipt(t, bodyMD, 0)
	case "aged5h":
		addReceipt(t, bodyMD, 5*time.Hour)
	}
	sessionID := ""
	if c.session {
		sessionID = fmt.Sprintf("case-%d", row)
	}
	var last hookOutput
	var lastMode string
	for _, s := range c.steps {
		last = runStepNew(t, s, sessionID, botID)
		lastMode = s.mode
	}
	return outcome{
		decision: dec(last),
		reason:   last.HookSpecificOutput.PermissionDecisionReason,
		injected: lastMode == "prompt" && last.HookSpecificOutput.AdditionalContext != "",
		armed:    isArmed(sessionSlug(sessionID)),
	}
}

// runStepNew renders and runs one step through the new gate's Run entry
// point, returning its reply.
func runStepNew(t *testing.T, s step, sessionID, botID string) hookOutput {
	t.Helper()
	payload := map[string]any{}
	if sessionID != "" {
		payload["session_id"] = sessionID
	}
	switch s.mode {
	case "prompt":
		payload["prompt"] = s.text
		o, _ := hook(t, "prompt", payload)
		return o
	case "session-start":
		payload["source"] = s.text
		o, _ := hook(t, "session-start", payload)
		return o
	case "read":
		payload["tool_input"] = map[string]any{"file_path": renderRequiredFile(s.file)}
		o, _ := hook(t, "read", payload)
		return o
	case "pre-tool":
		if s.raw {
			o, _ := hook(t, "pre-tool", s.text)
			return o
		}
		toolName, input := renderToolCallNew(t, s, botID)
		payload["tool_name"] = toolName
		payload["tool_input"] = input
		if s.transcript != "" {
			payload["transcript_path"] = writeTranscriptNew(t, s.transcript)
		}
		o, _ := hook(t, "pre-tool", payload)
		return o
	default:
		t.Fatalf("unknown step mode %q", s.mode)
		return hookOutput{}
	}
}

// renderRequiredFile resolves a read step's "brief" | "profile" | "exemplars"
// key to the actual path the new gate watches for.
func renderRequiredFile(key string) string {
	for _, f := range requiredFiles {
		if f.key == key {
			return f.path()
		}
	}
	panic("unknown required file key " + key)
}

// renderToolCallNew renders a step's tool and input for the new side:
// "Skill:polish" and "Skill:other" render to Byline's own skill shape,
// everything else passes its tool name through with <bot>/<person>/
// <bodyfile> substituted in its input.
func renderToolCallNew(t *testing.T, s step, botID string) (string, map[string]any) {
	switch s.tool {
	case "Skill:polish":
		return "Skill", map[string]any{"skill": "byline", "args": "polish"}
	case "Skill:other":
		return "Skill", map[string]any{"skill": "other-skill"}
	default:
		return s.tool, renderInputTokens(t, s.input, botID)
	}
}

// renderValueTokens substitutes a step's three placeholder tokens -- shared
// by both the new and old sides, so a case's rendered values are identical
// on both.
func renderValueTokens(t *testing.T, v any, botID string) any {
	s, ok := v.(string)
	if !ok {
		return v
	}
	switch s {
	case "<bot>":
		return botID
	case "<person>":
		return person
	case "<bodyfile>":
		p := filepath.Join(t.TempDir(), "body.html")
		if err := os.WriteFile(p, []byte(bodyHTML()), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	default:
		return v
	}
}

func renderInputTokens(t *testing.T, input map[string]any, botID string) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = renderValueTokens(t, v, botID)
	}
	return out
}

// writeTranscriptNew builds the new side's log fixture for a pre-tool step's
// transcript field: "mentions" never counts as a read, "reads" always does.
func writeTranscriptNew(t *testing.T, kind string) string {
	switch kind {
	case "mentions":
		line := mustJSON(map[string]any{
			"type": "user",
			"message": map[string]any{"content": []map[string]any{{
				"type":    "tool_result",
				"content": writingDenyReason([]string{"brief", "profile", "exemplars"}),
			}}},
		})
		return writeLines(t, line)
	case "reads":
		return writeLines(t, readLine(paths.BriefPath()), readLine(paths.ProfilePath()), readLine(paths.ExemplarsPath()))
	default:
		return ""
	}
}
