package gate

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"byline/internal/paths"
	"byline/internal/state"
)

func TestWritingGated(t *testing.T) {
	sk := func(s, a string) map[string]any { return map[string]any{"skill": s, "args": a} }
	for _, c := range []struct {
		name  string
		input map[string]any
		send  bool
		want  bool
	}{
		// Today's code gates these five, but today's settings never route them, and neither does Phase 6.
		{"Write", nil, false, false}, {"Edit", nil, false, false}, {"NotebookEdit", nil, false, false},
		{"Agent", nil, false, false}, {"Task", nil, false, false},
		{"Read", nil, false, false}, {"Grep", nil, false, false}, {"Glob", nil, false, false},
		{"Bash", nil, false, false}, {"WebFetch", nil, false, false}, {toolSlackSearch, nil, false, false},
		{"mcp__anything__send", nil, true, true},
		{"Skill", sk("byline", "polish"), false, true},
		{"Skill", sk("SomePlugin:byline", "Polish the draft"), false, true},
		{"Skill", map[string]any{"skill": "byline"}, false, false},
		{"Skill", sk("byline", "write a reply"), false, false},
		{"Skill", sk("other-skill", "polish"), false, false},
	} {
		if got := writingGated(c.name, c.input, c.send); got != c.want {
			t.Errorf("%s %v: got %v, want %v", c.name, c.input, got, c.want)
		}
	}
}

func TestPreTool_DeniesUntilAllThreeRead(t *testing.T) {
	newHome(t)
	hook(t, "prompt", map[string]any{"session_id": "s", "prompt": "draft a slack message to the vendor"})
	w := polish("s")
	o, _ := hook(t, "pre-tool", w)
	if dec(o) != "deny" || o.HookSpecificOutput.PermissionDecisionReason != writingDenyReason([]string{"brief", "profile", "exemplars"}) ||
		o.SystemMessage != writingDenySystemMessage {
		t.Fatalf("nothing read: got %+v", o)
	}
	for _, p := range []string{paths.BriefPath(), paths.ExemplarsPath()} {
		hook(t, "read", map[string]any{"session_id": "s", "tool_input": map[string]any{"file_path": p}})
	}
	if o, _ = hook(t, "pre-tool", w); o.HookSpecificOutput.PermissionDecisionReason != writingDenyReason([]string{"profile"}) {
		t.Fatalf("two of three: got %+v", o)
	}
	hook(t, "read", map[string]any{"session_id": "s", "tool_input": map[string]any{"file_path": paths.ProfilePath()}})
	if _, raw := hook(t, "pre-tool", w); raw != "" {
		t.Fatalf("all three read: got %s", raw)
	}
}

func TestPreTool_UnarmedPassesThrough(t *testing.T) {
	newHome(t)
	for _, p := range []map[string]any{polish("u"), polish("")} {
		if _, raw := hook(t, "pre-tool", p); raw != "" {
			t.Errorf("%v: got %s", p, raw)
		}
	}
}

func TestTranscript_CountsOnlyRealReads(t *testing.T) {
	newHome(t)
	msg := func(role string, content any) string {
		return mustJSON(map[string]any{"type": role, "message": map[string]any{"content": content}})
	}
	mention := msg("user", []any{map[string]any{"type": "tool_result", "content": writingDenyReason([]string{"brief", "profile", "exemplars"})}})
	// The next three all contain "Read" in quotes, so they get past the quick filter and test the parser itself.
	notToolUse := msg("user", []any{map[string]any{"type": "tool_result", "name": "Read", "content": paths.BriefPath()}})
	otherFile := readLine("/tmp/other.md")
	asText := mustJSON(map[string]any{"type": "user", "tool": "Read", "message": map[string]any{"content": paths.BriefPath()}})
	grep := msg("assistant", []any{map[string]any{"type": "tool_use", "name": "Grep", "input": map[string]any{"file_path": paths.BriefPath()}}})
	if got := transcriptReads(writeLines(t, mention, notToolUse, otherFile, asText, grep)); len(got) != 0 {
		t.Fatalf("mentions counted as reads: %v", got)
	}
	reads := []string{readLine(paths.ExemplarsPath()), readLine(paths.BriefPath()), readLine(paths.ProfilePath()), readLine(paths.BriefPath())}
	want := []string{"brief", "profile", "exemplars"}
	if got := transcriptReads(writeLines(t, reads...)); !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	crlf := filepath.Join(t.TempDir(), "crlf.jsonl")
	os.WriteFile(crlf, []byte(strings.Join(reads, "\r\n")), 0o600)
	if got := transcriptReads(crlf); !slices.Equal(got, want) {
		t.Fatalf("CRLF log: got %v", got)
	}
	if transcriptReads("") != nil || transcriptReads("/no/such/file") != nil {
		t.Fatal("no log must give nil")
	}
}

func TestPreTool_TranscriptFallbackRecordsReads(t *testing.T) {
	newHome(t)
	hook(t, "prompt", map[string]any{"session_id": "s", "prompt": "draft a message to the vendor"})
	tr := writeLines(t, readLine(paths.BriefPath()), readLine(paths.ProfilePath()), readLine(paths.ExemplarsPath()))
	_, raw := hook(t, "pre-tool", map[string]any{"sessionId": "s", "toolName": "Skill", "toolInput": map[string]any{"skill": "byline", "args": "polish"}, "transcriptPath": tr})
	if raw != "" {
		t.Fatalf("real reads in the log must open the gate: %s", raw)
	}
	for _, f := range requiredFiles {
		if !hasRead("s", f.key) {
			t.Errorf("fallback did not record %s", f.key)
		}
	}
}

func TestGate_BackslashReadPathCounts(t *testing.T) {
	home := newHome(t)
	win := func(name string) string { return `C:\Users\someone\` + filepath.Base(home) + `\` + name }
	hook(t, "read", map[string]any{"session_id": "r", "tool_input": map[string]any{"file_path": win("brief.md")}})
	if !hasRead("r", "brief") {
		t.Fatal("a backslash path in the read hook was not recorded")
	}
	tr := writeLines(t, readLine(win("brief.md")), readLine(win("profile.md")), readLine(win("exemplars.md")))
	hook(t, "prompt", map[string]any{"session_id": "w", "prompt": "draft a reply to the vendor"})
	call := polish("w")
	call["transcript_path"] = tr
	if _, raw := hook(t, "pre-tool", call); raw != "" {
		t.Fatalf("backslash paths in the log did not count: %s", raw)
	}
}

func TestGate_HugeTranscriptTailStartsMidLine(t *testing.T) {
	newHome(t)
	item := mustJSON(map[string]any{"type": "tool_use", "id": "x", "name": "Read", "input": map[string]any{"file_path": paths.BriefPath()}})
	// One line longer than the tail window, with a real-looking Read after the filler, so the cut part still contains "Read".
	head := `{"type":"assistant","message":{"content":[{"type":"text","text":"` + strings.Repeat("x", transcriptTailBytes+1<<20) + `"},` + item + "]}}\n"
	if len(head) <= transcriptTailBytes {
		t.Fatal("the first line must be longer than the tail window")
	}
	call := polish("h")
	t.Run("reads after the cut count", func(t *testing.T) {
		resetSession("h")
		hook(t, "prompt", map[string]any{"session_id": "h", "prompt": "draft a reply to the vendor"})
		tr := writeLines(t, head+readLine(paths.BriefPath()), readLine(paths.ProfilePath()), readLine(paths.ExemplarsPath()))
		call["transcript_path"] = tr
		if _, raw := hook(t, "pre-tool", call); raw != "" {
			t.Fatalf("got %.200s", raw)
		}
	})
	t.Run("only the cut line: denied for unread files, not for parse errors", func(t *testing.T) {
		resetSession("h")
		hook(t, "prompt", map[string]any{"session_id": "h", "prompt": "draft a reply to the vendor"})
		call["transcript_path"] = writeLines(t, strings.TrimSuffix(head, "\n"))
		o, _ := hook(t, "pre-tool", call)
		if o.HookSpecificOutput.PermissionDecisionReason != writingDenyReason([]string{"brief", "profile", "exemplars"}) {
			t.Fatalf("got %+v", o)
		}
	})
}

func TestGate_FreshInstallTemplatesDoNotDeadlock(t *testing.T) {
	draft := map[string]any{"session_id": "f", "prompt": "draft a reply to the vendor"}
	start := map[string]any{"session_id": "f", "source": "startup"}
	t.Run("empty voice folder", func(t *testing.T) {
		home := newHome(t)
		os.RemoveAll(home)
		if err := os.MkdirAll(home, 0o700); err != nil {
			t.Fatal(err)
		}
		if o, _ := hook(t, "session-start", start); o.SystemMessage != templateSystemMessage {
			t.Fatalf("session start: got %+v", o)
		}
		for _, f := range requiredFiles {
			if _, err := os.Stat(f.path()); err != nil {
				t.Fatalf("session start did not create %s", f.name)
			}
		}
		// The profile is still templated, so this first prompt also carries
		// setupOfferContext alongside promptContext (joined by "\n\n").
		if o, _ := hook(t, "prompt", draft); !strings.Contains(o.HookSpecificOutput.AdditionalContext, promptContext()) {
			t.Fatalf("prompt: got %+v", o)
		}
		if o, _ := hook(t, "pre-tool", polish("f")); dec(o) != "deny" {
			t.Fatal("polish before reading must be stopped")
		}
		for _, f := range requiredFiles {
			hook(t, "read", map[string]any{"session_id": "f", "tool_input": map[string]any{"file_path": f.path()}})
		}
		if _, raw := hook(t, "pre-tool", polish("f")); raw != "" {
			t.Fatalf("still stopped after reading the three template files: %s", raw)
		}
	})
	t.Run("voice folder can't be made", func(t *testing.T) {
		blocker := filepath.Join(t.TempDir(), "file")
		os.WriteFile(blocker, nil, 0o600)
		t.Setenv("BYLINE_HOME", filepath.Join(blocker, ".byline")) // no folder can be made under a file
		// I4: this is EnsureVoiceFolder's own error, not the generic
		// "isn't set up yet" text.
		wantErr := state.EnsureVoiceFolder()
		if wantErr == nil {
			t.Fatal("fixture must make EnsureVoiceFolder fail")
		}
		if o, _ := hook(t, "session-start", start); o.SystemMessage != wantErr.Error() {
			t.Fatalf("session start: got %+v, want %v", o, wantErr)
		}
		if o, _ := hook(t, "prompt", draft); o.HookSpecificOutput.AdditionalContext == "" {
			t.Fatal("the prompt context is still injected")
		}
		if _, raw := hook(t, "pre-tool", polish("f")); raw != "" {
			t.Fatalf("nothing can be read, so nothing to wait for: %s", raw)
		}
	})
	t.Run("files removed mid-session", func(t *testing.T) {
		newHome(t)
		hook(t, "prompt", draft)
		os.Remove(paths.ProfilePath())
		os.Remove(paths.ExemplarsPath())
		hook(t, "read", map[string]any{"session_id": "f", "tool_input": map[string]any{"file_path": paths.BriefPath()}})
		if _, raw := hook(t, "pre-tool", polish("f")); raw != "" {
			t.Fatalf("files that aren't on disk must not be waited for: %s", raw)
		}
	})
}
