package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"byline/internal/paths"
)

// readLogLines reads path's lines, failing the test if it can't, for
// Part 2's two observability logs.
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

// TestToolCallLog_WritesLineWithFieldsNoValues: a tools/call result
// appends one line naming the tool, the outcome, how long it took, and
// the argument's NAME, never its value.
func TestToolCallLog_WritesLineWithFieldsNoValues(t *testing.T) {
	setupHome(t)
	secret := "a-distinctive-value-7f3a-never-in-the-log"
	out, isErr, _ := callTool(t, testServer(), "echo", map[string]any{"text": secret})
	if isErr || out["echo"] != secret {
		t.Fatalf("unexpected result: %v %v", out, isErr)
	}

	lines := readLogLines(t, paths.ToolCallLogPath())
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1: %v", len(lines), lines)
	}
	if strings.Contains(lines[0], secret) {
		t.Fatalf("the log line carries the argument's own value: %q", lines[0])
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &m); err != nil {
		t.Fatalf("not valid JSON: %q", lines[0])
	}
	want(t, m["tool"], "echo")
	want(t, m["outcome"], "ok")
	want(t, m["args"], []any{"text"})
	if _, err := time.Parse(time.RFC3339, str(m["time"])); err != nil {
		t.Fatalf("time %v isn't RFC 3339: %v", m["time"], err)
	}
	if _, ok := m["duration_ms"]; !ok {
		t.Fatal("no duration_ms field")
	}
}

// str returns v as a string, or "" when v isn't one (test-only, mirrors
// the gate package's helper of the same name).
func str(v any) string {
	s, _ := v.(string)
	return s
}

// TestToolCallLog_RejectedOutcome: a call checkArgs refuses logs
// outcome "rejected", not "error".
func TestToolCallLog_RejectedOutcome(t *testing.T) {
	setupHome(t)
	callTool(t, testServer(), "echo", map[string]any{}) // missing required "text"

	lines := readLogLines(t, paths.ToolCallLogPath())
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1: %v", len(lines), lines)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &m); err != nil {
		t.Fatal(err)
	}
	want(t, m["outcome"], "rejected")
}

// TestToolCallLog_UnwritableFolderLeavesResultUnchanged: when
// tool-calls.jsonl's folder can't be created (here, a plain file sits
// where it needs to go), the tool's own result must come back exactly
// as it would without logging at all.
func TestToolCallLog_UnwritableFolderLeavesResultUnchanged(t *testing.T) {
	home := setupHome(t)
	if err := os.WriteFile(filepath.Join(home, "logs"), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, isErr, text := callTool(t, testServer(), "echo", map[string]any{"text": "hi"})
	if isErr || out["echo"] != "hi" || text != `{"echo":"hi"}` {
		t.Fatalf("a logging failure changed the tool result: %v %v %q", out, isErr, text)
	}
}
