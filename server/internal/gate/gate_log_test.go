package gate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"byline/internal/paths"
)

// readLogLines reads path's lines, failing the test if it can't.
func readLogLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := strings.TrimRight(string(data), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// TestGateDecisionLog_RecordsDecisionAndCodeNoReasonText: a deny this
// hook invocation actually returns logs the canonical tool name, the
// decision, and a short code, but never the reason text shown to the
// person.
func TestGateDecisionLog_RecordsDecisionAndCodeNoReasonText(t *testing.T) {
	newHome(t)
	hook(t, "prompt", map[string]any{"session_id": "s", "prompt": "draft a slack message to the vendor"})
	hook(t, "pre-tool", map[string]any{"session_id": "s", "tool_name": "mcp__slack__conversations_add_message",
		"tool_input": map[string]any{"channel": bot, "text": longUnchecked}})

	lines := readLogLines(t, paths.GateDecisionLogPath())
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1: %v", len(lines), lines)
	}
	if strings.Contains(lines[0], "haven't been read yet") {
		t.Fatalf("the gate log carries the reason text shown to the person: %q", lines[0])
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &m); err != nil {
		t.Fatalf("not valid JSON: %q", lines[0])
	}
	if m["event"] != "PreToolUse" || m["tool"] != "mcp__slack__conversations_add_message" || m["decision"] != "deny" || m["reason"] != "writing_unread" {
		t.Fatalf("got %+v", m)
	}
	if _, err := time.Parse(time.RFC3339, str(m["time"])); err != nil {
		t.Fatalf("time %v isn't RFC 3339: %v", m["time"], err)
	}
}

// TestGateDecisionLog_NoneWhenNoOpinion: a PreToolUse call the gate has
// nothing to say about still logs one line, decision "none", so a quiet
// gate is told apart from a gate that never ran.
func TestGateDecisionLog_NoneWhenNoOpinion(t *testing.T) {
	newHome(t)
	hook(t, "pre-tool", map[string]any{"tool_name": "Read", "tool_input": map[string]any{}})

	lines := readLogLines(t, paths.GateDecisionLogPath())
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1: %v", len(lines), lines)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &m); err != nil {
		t.Fatal(err)
	}
	if m["tool"] != "Read" || m["decision"] != "none" || m["reason"] != "" {
		t.Fatalf("got %+v", m)
	}
}

// TestGateDecisionLog_UnwritableFolderLeavesDecisionUnchanged: when
// gate-decisions.jsonl's folder can't be created (here, a plain file
// sits where it needs to go), the gate must still fail closed exactly
// as it would without logging at all.
func TestGateDecisionLog_UnwritableFolderLeavesDecisionUnchanged(t *testing.T) {
	home := newHome(t)
	if err := os.WriteFile(filepath.Join(home, "logs"), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	o, _ := hook(t, "pre-tool", "{not json")
	if dec(o) != "deny" || o.HookSpecificOutput.PermissionDecisionReason != malformedDenyReason {
		t.Fatalf("a logging failure changed the gate's own decision: %+v", o)
	}
}

// TestGateDecisionLog_ExtraModeLogsOnlyItsOwnDecisions: the broad
// pre-tool-extra hook runs on every MCP call, including each default send
// tool that pre-tool already judged. When it has no opinion it writes
// nothing, so one send is one line, not a second misleading "none".
func TestGateDecisionLog_ExtraModeLogsOnlyItsOwnDecisions(t *testing.T) {
	newHome(t)
	payload := map[string]any{"tool_name": "mcp__slack__conversations_add_message", "tool_input": map[string]any{"channel": bot, "text": longUnchecked}}
	hook(t, "pre-tool", payload)
	hook(t, "pre-tool-extra", payload)
	if lines := readLogLines(t, paths.GateDecisionLogPath()); len(lines) != 1 {
		t.Fatalf("one send must log one line, got %d: %v", len(lines), lines)
	}
}
