package gate

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"byline/internal/evlog"
	"byline/internal/paths"
	"byline/internal/state"
)

// hookOutput is the JSON Byline writes to stdout for one hook call. Claude
// Code reads hookSpecificOutput's fields to act on the hook, and
// systemMessage (when set) to show the person a line of plain text.
// logTool and logCode never reach that JSON, by design: they're decide's
// own record, for Run's gate-decisions.jsonl line, of which tool this
// decision was about and a short code for why -- never the reason text
// shown to the person.
type hookOutput struct {
	HookSpecificOutput hookSpecific `json:"hookSpecificOutput"`
	SystemMessage      string       `json:"systemMessage,omitempty"`
	logTool            string       `json:"-"`
	logCode            string       `json:"-"`
}

type hookSpecific struct {
	HookEventName            string `json:"hookEventName"`
	AdditionalContext        string `json:"additionalContext,omitempty"`
	PermissionDecision       string `json:"permissionDecision,omitempty"`
	PermissionDecisionReason string `json:"permissionDecisionReason,omitempty"`
}

// modes maps a hook mode name to the function that handles it. Tasks 3.4
// and 3.5 add the pre-tool checks and "pre-tool-extra".
var modes = map[string]func(raw []byte) *hookOutput{
	"session-start":  sessionStart,
	"prompt":         prompt,
	"read":           read,
	"pre-tool":       preTool,
	"pre-tool-extra": preToolExtra,
}

// Run reads in fully, dispatches to modes[mode], and writes the reply to
// out as one JSON object when the mode returns one. An unknown mode reads
// nothing and writes nothing. The launcher that invokes Run always exits
// 0, so the printed reply is the only way a decision reaches Claude Code;
// a panic inside a mode is recovered here and turned into an "ask" for
// pre-tool, and silence for every other mode. For "pre-tool" and
// "pre-tool-extra" -- the two modes that answer a PreToolUse hook -- Run
// is also the single place that logs the decision this call actually
// returned, so a decision computed and discarded inside a mode is never
// logged as if it had taken effect.
func Run(mode string, in io.Reader, out io.Writer) {
	fn, ok := modes[mode]
	if !ok {
		return
	}
	var raw []byte
	defer func() {
		if recover() != nil && mode == "pre-tool" {
			o := decide("", "ask", "crash", crashAskReason)
			writeHookOutput(out, o)
			logGateDecision(mode, raw, o)
		}
	}()
	raw, _ = io.ReadAll(in)
	o := fn(raw)
	writeHookOutput(out, o)
	logGateDecision(mode, raw, o)
}

// logGateDecision appends one line to gate-decisions.jsonl for every
// pre-tool invocation, and for each pre-tool-extra invocation that decided
// something: UTC time, the hook event, the canonical
// tool name, the decision this invocation actually returned ("none" when
// it returned no opinion at all), and a short reason code -- never the
// reason text shown to the person. The tool name is decide's own record
// on o when there is one; otherwise it falls back to the raw payload's
// own tool name. evlog.Append drops any error silently, so a
// logging failure never changes or slows the decision itself.
func logGateDecision(mode string, raw []byte, o *hookOutput) {
	if mode != "pre-tool" && mode != "pre-tool-extra" {
		return
	}
	if mode == "pre-tool-extra" && o == nil {
		return // it runs on every MCP call, and pre-tool already logged the default send tools
	}
	decision, code, tool := "none", "", ""
	if o != nil {
		decision = o.HookSpecificOutput.PermissionDecision
		code = o.logCode
		tool = o.logTool
	}
	if tool == "" {
		if p, ok := parsePayload(raw); ok {
			tool = str(field(p, "tool_name", "toolName"))
		}
	}
	evlog.Append(paths.GateDecisionLogPath(), map[string]any{
		"time":     time.Now().UTC().Format(time.RFC3339),
		"event":    "PreToolUse",
		"tool":     tool,
		"decision": decision,
		"reason":   code,
	})
}

func writeHookOutput(out io.Writer, o *hookOutput) {
	if o == nil {
		return
	}
	if b, err := json.Marshal(o); err == nil {
		out.Write(b)
	}
}

// parsePayload decodes raw as a JSON object. Valid JSON that isn't an
// object -- null, an array, a bare string or number -- counts as malformed,
// same as invalid JSON.
func parsePayload(raw []byte) (map[string]any, bool) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, false
	}
	m, ok := v.(map[string]any)
	return m, ok
}

// field returns the value of the first of keys that is present in p with a
// non-nil value, so a mode can read either Claude Code's snake_case or
// camelCase field name.
func field(p map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := p[k]; ok && v != nil {
			return v
		}
	}
	return nil
}

// str returns v as a string, or "" when v isn't one.
func str(v any) string {
	s, _ := v.(string)
	return s
}

// toolInput returns a hook payload's tool_input (or toolInput), or an
// empty map when neither is present.
func toolInput(p map[string]any) map[string]any {
	if m, ok := field(p, "tool_input", "toolInput").(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

// decide is a PreToolUse reply that allows, denies or asks about a tool
// call, with reason shown to the person. tool and code are never shown
// to anyone; they're logGateDecision's record of which tool this was
// about and a short, stable code for reason, for gate-decisions.jsonl.
func decide(tool, decision, code, reason string) *hookOutput {
	return &hookOutput{
		HookSpecificOutput: hookSpecific{
			HookEventName:            "PreToolUse",
			PermissionDecision:       decision,
			PermissionDecisionReason: reason,
		},
		logTool: tool,
		logCode: code,
	}
}

// addContext is a reply that injects context for event, with systemMessage
// shown to the person when it isn't empty.
func addContext(event, context, systemMessage string) *hookOutput {
	return &hookOutput{
		HookSpecificOutput: hookSpecific{HookEventName: event, AdditionalContext: context},
		SystemMessage:      systemMessage,
	}
}

// sessionStart runs on Claude Code's SessionStart hook: it makes sure the
// voice folder exists, resets a fresh session's state, prunes state and
// receipts no session will read again (M7), and warns when setup is
// missing, broken or incomplete.
func sessionStart(raw []byte) *hookOutput {
	p, ok := parsePayload(raw)
	if !ok {
		return nil
	}
	voiceErr := state.EnsureVoiceFolder()

	source := str(field(p, "source"))
	if source == "startup" || source == "clear" {
		resetSession(sessionSlug(str(field(p, "session_id", "sessionId"))))
	}
	pruneState(stateMaxAge)
	pruneReceipts(receiptLifetime) // M7

	// I4: a real voice-folder problem (a cut-off my-rules.json, a folder
	// that can't be created) gets EnsureVoiceFolder's own plain sentences,
	// never the generic "isn't set up yet" text, which would send an
	// already set-up person back through setup instead of at the cause.
	if voiceErr != nil {
		msg := voiceErr.Error()
		return addContext("SessionStart", msg, msg)
	}

	var missing []string
	for _, f := range requiredFiles {
		if !fileExists(f.path()) {
			missing = append(missing, f.name)
		}
	}
	if len(missing) > 0 {
		return addContext("SessionStart", missingContext(missing), missingSystemMessage)
	}

	if setupNotDone() {
		return addContext("SessionStart", templateContext, templateSystemMessage)
	}

	// M3: settings.json can't be read -- gateSettings still falls back to
	// defaults, but silently, so name the problem once here.
	if _, err := state.LoadSettings(); err != nil {
		msg := settingsProblemMessage(err)
		return addContext("SessionStart", msg, msg)
	}
	return nil
}

// setupNotDone reports whether profile.md still starts with the setup
// template marker -- the one check sessionStart already used to warn that
// setup hasn't run, now shared with prompt's first-message setup offer.
func setupNotDone() bool {
	b, err := os.ReadFile(paths.ProfilePath())
	return err == nil && strings.HasPrefix(string(b), setupTemplateMarker)
}

// settingsProblemMessage names, in one plain line, why settings.json
// couldn't be read, for sessionStart to show when gateSettings's silent
// fallback (M3) needs explaining.
func settingsProblemMessage(err error) string {
	return fmt.Sprintf("Byline can't read its settings (%s), so it's using the defaults for now: no bot chats, no extra draft phrases. "+
		"Fix settings.json, or ask Claude to redo setup, to restore them.", err)
}

// prompt runs on Claude Code's UserPromptSubmit hook: a real turn from the
// person (never a machine turn, isMachineTurn) can add two independent
// pieces of context. While setup hasn't run (setupNotDone), the first
// prompt of a session gets setupOfferContext, once per session (the
// setup-offered mark); it fires whatever the message asks, not only a
// draft request. A draft request also arms the session and gets
// promptContext, exactly as before. Either, both or neither may apply, so
// they're joined when both do.
func prompt(raw []byte) *hookOutput {
	p, ok := parsePayload(raw)
	if !ok {
		return nil
	}
	text := str(field(p, "prompt"))
	if text == "" {
		return nil
	}
	if isMachineTurn(text) {
		return nil
	}

	slug := sessionSlug(str(field(p, "session_id", "sessionId")))

	var contexts []string
	if setupNotDone() && slug != "" && !setupOffered(slug) {
		markSetupOffered(slug)
		contexts = append(contexts, setupOfferContext)
	}

	if detectDraftIntent(text, gateSettings().ExtraDraftPhrases) {
		if slug != "" {
			arm(slug)
		}
		contexts = append(contexts, promptContext())
	}

	if len(contexts) == 0 {
		return nil
	}
	return addContext("UserPromptSubmit", strings.Join(contexts, "\n\n"), "")
}

// read runs on Claude Code's PostToolUse hook for Read: it records that a
// session has actually read one of the three required files. It never
// produces a reply.
func read(raw []byte) *hookOutput {
	p, ok := parsePayload(raw)
	if !ok {
		return nil
	}
	slug := sessionSlug(str(field(p, "session_id", "sessionId")))
	key := requiredKey(str(field(toolInput(p), "file_path", "filePath")))
	if slug != "" && key != "" {
		recordRead(slug, key)
	}
	return nil
}

// preTool runs on Claude Code's PreToolUse hook. A malformed payload is
// denied outright, so a crashed or not-yet-built check never silently
// lets a send through; otherwise checkTool decides.
func preTool(raw []byte) *hookOutput {
	p, ok := parsePayload(raw)
	if !ok {
		return decide("", "deny", "malformed", malformedDenyReason)
	}
	return checkTool(p, false)
}

// checkTool is the shared body of both pre-tool modes ("pre-tool" and,
// from Task 3.5, "pre-tool-extra"). It stops the polish pass or a send
// until the three writing files have been read in this conversation, then
// (Task 3.5) runs the structure-check and Slack-approval checks on a send.
// extraOnly is true only for "pre-tool-extra", which sees every MCP tool
// call: it acts only on a settings send tool that isn't already one of
// pre-tool's defaults, so the two modes never both act on the same call.
func checkTool(p map[string]any, extraOnly bool) *hookOutput {
	name := str(field(p, "tool_name", "toolName"))
	s := gateSettings()
	canonical, isSend := matchSendTool(name, s.SendTools)

	if extraOnly && (!isSend || slices.Contains(state.DefaultSendTools, canonical)) {
		return nil
	}

	input := toolInput(p)
	if writingGated(name, input, isSend) {
		if out := writingStop(p); out != nil {
			return out
		}
	}
	if isSend {
		return sendChecks(canonical, input, s)
	}
	return nil
}

// writingStop stops a gated step in an armed session until the three
// writing files have been read in this conversation.
func writingStop(p map[string]any) *hookOutput {
	slug := sessionSlug(str(field(p, "session_id", "sessionId")))
	if slug == "" || !isArmed(slug) {
		return nil
	}
	transcriptPath := str(field(p, "transcript_path", "transcriptPath"))
	if missing := missingReads(slug, transcriptPath); len(missing) > 0 {
		tool := str(field(p, "tool_name", "toolName"))
		out := decide(tool, "deny", "writing_unread", writingDenyReason(missing))
		out.SystemMessage = writingDenySystemMessage
		return out
	}
	return nil
}

// preToolExtra runs on Claude Code's PreToolUse hook for every MCP tool
// call (Phase 6's broader matcher). A malformed payload gives nil: this
// mode sees every MCP call, including ones Byline has nothing to do with,
// so it never fails closed the way preTool's malformed case does.
func preToolExtra(raw []byte) *hookOutput {
	p, ok := parsePayload(raw)
	if !ok {
		return nil
	}
	return checkTool(p, true)
}
