package gate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"byline/internal/paths"
	"byline/internal/state"
)

func TestRun_UnknownModeIsSilent(t *testing.T) {
	newHome(t)
	for _, m := range []string{"", "nope"} {
		var out bytes.Buffer
		Run(m, strings.NewReader(`{"session_id":"s","prompt":"draft a reply to the vendor"}`), &out)
		if out.Len() != 0 || isArmed("s") {
			t.Errorf("mode %q printed %q", m, out.String())
		}
	}
}

func TestRun_PanicIsRecovered(t *testing.T) {
	newHome(t)
	for _, m := range []string{"pre-tool", "prompt", "read", "session-start"} {
		orig := modes[m]
		modes[m] = func([]byte) *hookOutput { panic("boom") }
		var out bytes.Buffer
		Run(m, strings.NewReader("{}"), &out)
		modes[m] = orig
		if m == "pre-tool" {
			var o hookOutput
			json.Unmarshal(out.Bytes(), &o)
			if dec(o) != "ask" || o.HookSpecificOutput.PermissionDecisionReason != crashAskReason {
				t.Fatalf("pre-tool panic: got %s", out.String())
			}
		} else if out.Len() != 0 {
			t.Fatalf("%s panic printed %s", m, out.String())
		}
	}
}

func TestRequiredKey(t *testing.T) {
	home := newHome(t)
	for in, want := range map[string]string{
		paths.BriefPath():                                           "brief",
		strings.ToUpper(paths.ProfilePath()):                        "profile",
		`C:\Users\someone\` + filepath.Base(home) + `\exemplars.md`: "exemplars",
		filepath.Join(home, "backups", "brief.md"):                  "",
		"/elsewhere/brief.md":                                       "",
		filepath.Join(home, "my-stance.md"):                         "",
		"":                                                          "",
	} {
		if got := requiredKey(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestRead_RecordsOnlyRequiredFiles(t *testing.T) {
	newHome(t)
	hook(t, "read", map[string]any{"session_id": "a", "tool_input": map[string]any{"file_path": paths.BriefPath()}})
	hook(t, "read", map[string]any{"sessionId": "b", "toolInput": map[string]any{"file_path": paths.ExemplarsPath()}})
	hook(t, "read", map[string]any{"session_id": "c", "tool_input": map[string]any{"filePath": paths.ProfilePath()}})
	_, raw := hook(t, "read", map[string]any{"session_id": "d", "tool_input": map[string]any{"file_path": "/tmp/other.md"}})
	if !hasRead("a", "brief") || !hasRead("b", "exemplars") || !hasRead("c", "profile") {
		t.Fatal("a required read was not recorded")
	}
	if m, _ := filepath.Glob(filepath.Join(paths.StateDir(), "d.*")); len(m) != 0 || raw != "" {
		t.Fatal("an unrelated read left state or printed output")
	}
}

func TestGate_ParallelReadsAllRecorded(t *testing.T) {
	newHome(t)
	for i := 0; i < 50; i++ {
		id := fmt.Sprintf("p%d", i)
		var wg sync.WaitGroup
		for _, f := range requiredFiles {
			wg.Add(1)
			go func(p string) {
				defer wg.Done()
				Run("read", strings.NewReader(mustJSON(map[string]any{"session_id": id, "tool_input": map[string]any{"file_path": p}})), io.Discard)
			}(f.path())
		}
		wg.Wait()
		for _, f := range requiredFiles {
			if !hasRead(id, f.key) {
				t.Fatalf("round %d lost the %s read", i, f.key)
			}
		}
	}
}

func TestSessionStart_ResetAndPrune(t *testing.T) {
	newHome(t)
	for src, kept := range map[string]bool{"startup": false, "clear": false, "compact": true, "resume": true, "": true} {
		id := "r-" + src
		arm(id)
		recordRead(id, "brief")
		arm("stale")
		when := time.Now().Add(-8 * 24 * time.Hour)
		os.Chtimes(filepath.Join(paths.StateDir(), "stale.armed"), when, when)
		hook(t, "session-start", map[string]any{"session_id": id, "source": src})
		if isArmed(id) != kept || hasRead(id, "brief") != kept {
			t.Errorf("source %q: kept=%v, want %v", src, isArmed(id), kept)
		}
		if isArmed("stale") {
			t.Errorf("source %q: 8-day-old state not pruned", src)
		}
	}
}

// TestSessionStart_PrunesOldReceipts is M7: session start also removes
// receipts older than the receipt lifetime, not only session state.
func TestSessionStart_PrunesOldReceipts(t *testing.T) {
	newHome(t)
	addReceipt(t, bodyMD, receiptLifetime+time.Hour)
	addReceipt(t, bodyMD, 0)
	hook(t, "session-start", map[string]any{"source": "startup"})
	entries, err := os.ReadDir(paths.ReceiptsDir())
	if err != nil || len(entries) != 1 {
		t.Fatalf("want 1 receipt left after session start, got %d (%v)", len(entries), err)
	}
}

func TestSessionStart_Warnings(t *testing.T) {
	newHome(t)
	start := map[string]any{"source": "startup"}
	if _, raw := hook(t, "session-start", start); raw != "" {
		t.Fatalf("set up: want silence, got %s", raw)
	}
	os.WriteFile(paths.ProfilePath(), []byte("<!-- byline template: setup replaces this file -->\n# Profile\n"), 0o600)
	o, _ := hook(t, "session-start", start)
	if o.HookSpecificOutput.HookEventName != "SessionStart" || o.SystemMessage != templateSystemMessage ||
		o.HookSpecificOutput.AdditionalContext != templateContext {
		t.Fatalf("templates: got %+v", o)
	}
	os.Remove(paths.ExemplarsPath())
	hook(t, "session-start", start)
	if _, err := os.Stat(paths.ExemplarsPath()); err != nil {
		t.Fatal("session start must restore a missing file through state.EnsureVoiceFolder")
	}
	blocker := filepath.Join(t.TempDir(), "file")
	os.WriteFile(blocker, nil, 0o600)
	t.Setenv("BYLINE_HOME", filepath.Join(blocker, ".byline")) // no folder can be made under a file
	// I4: EnsureVoiceFolder's own error replaces the generic "isn't set up
	// yet" text, since the folder here isn't simply unset up -- it can't
	// be created at all, and that's a different problem with a different
	// fix. wantErr is read by calling the same function a second time:
	// since nothing it does can succeed here, it fails the same way twice.
	wantErr := state.EnsureVoiceFolder()
	o, _ = hook(t, "session-start", start)
	if wantErr == nil {
		t.Fatal("fixture must make EnsureVoiceFolder fail")
	}
	if o.SystemMessage != wantErr.Error() || o.HookSpecificOutput.AdditionalContext != wantErr.Error() {
		t.Fatalf("voice folder can't be made: got %+v, want the EnsureVoiceFolder error %v", o, wantErr)
	}
}

// TestSessionStart_VoiceFolderErrorNamesCause is I4: when the voice folder
// exists but EnsureVoiceFolder can't use it -- here, a cut-off
// my-rules.json -- session start must say that, not send an already
// set-up person back through setup.
func TestSessionStart_VoiceFolderErrorNamesCause(t *testing.T) {
	newHome(t)
	os.WriteFile(paths.MyRulesPath(), []byte(`{"format":1,"add_rules":[{"id":"K1"}]`), 0o600) // cut off mid-array
	os.Remove(paths.BriefPath())
	wantErr := state.EnsureVoiceFolder()
	if wantErr == nil {
		t.Fatal("fixture must make EnsureVoiceFolder fail")
	}
	o, _ := hook(t, "session-start", map[string]any{"source": "startup"})
	if o.SystemMessage != wantErr.Error() || o.HookSpecificOutput.AdditionalContext != wantErr.Error() {
		t.Fatalf("got %+v, want the EnsureVoiceFolder error %v", o, wantErr)
	}
	if o.SystemMessage == missingSystemMessage {
		t.Fatal("must not fall back to the generic \"isn't set up yet\" text")
	}
}

// TestSessionStart_BrokenSettings is M3: a settings.json the gate can't
// read silently falls back to the defaults (no bot chats, no extra draft
// phrases); session start must name that, in one plain line, instead of
// leaving it unexplained.
func TestSessionStart_BrokenSettings(t *testing.T) {
	newHome(t)
	os.WriteFile(paths.SettingsPath(), []byte("not json"), 0o600)
	_, err := state.LoadSettings()
	if err == nil {
		t.Fatal("fixture must make settings.json unreadable")
	}
	want := settingsProblemMessage(err)
	o, _ := hook(t, "session-start", map[string]any{"source": "startup"})
	if o.SystemMessage != want || o.HookSpecificOutput.AdditionalContext != want {
		t.Fatalf("got %+v, want %q", o, want)
	}
}

func TestPrompt_DraftArmsAndInjects(t *testing.T) {
	newHome(t)
	o, _ := hook(t, "prompt", map[string]any{"session_id": "s", "prompt": "draft a reply to the vendor about the rate limits"})
	hs := o.HookSpecificOutput
	if !isArmed("s") || hs.HookEventName != "UserPromptSubmit" || hs.AdditionalContext != promptContext() {
		t.Fatalf("got %+v", o)
	}
	for _, p := range []string{paths.BriefPath(), paths.ProfilePath(), paths.ExemplarsPath()} {
		if !strings.Contains(hs.AdditionalContext, p) {
			t.Errorf("context does not name %s", p)
		}
	}
	o, _ = hook(t, "prompt", map[string]any{"prompt": "draft a reply to the vendor"})
	if o.HookSpecificOutput.AdditionalContext == "" {
		t.Error("no session id: the context is still injected")
	}
	if e, _ := os.ReadDir(paths.StateDir()); len(e) != 1 {
		t.Errorf("no session id must arm nothing; state has %d files", len(e))
	}
}

func TestPrompt_SilentCases(t *testing.T) {
	newHome(t)
	for name, p := range map[string]any{
		"malformed":    "not json",
		"empty prompt": map[string]any{"session_id": "s", "prompt": ""},
		"not a string": map[string]any{"session_id": "s", "prompt": 5},
		"machine turn": map[string]any{"session_id": "s", "prompt": "<task-notification>draft a reply to the vendor</task-notification>"},
		"not a draft":  map[string]any{"session_id": "s", "prompt": "what did the vendor say about the new accounts"},
	} {
		if _, raw := hook(t, "prompt", p); raw != "" || isArmed("s") {
			t.Errorf("%s: printed %q, armed %v", name, raw, isArmed("s"))
		}
	}
}

func TestPrompt_ExtraDraftPhrases(t *testing.T) {
	newHome(t)
	os.WriteFile(paths.SettingsPath(), []byte(`{"format":1,"extra_draft_phrases":["tell the vendor"]}`), 0o600)
	hook(t, "prompt", map[string]any{"session_id": "s", "prompt": "tell the vendor the plan"})
	if !isArmed("s") {
		t.Fatal("a phrase from settings must arm")
	}
}

func TestPreTool_MalformedDenies(t *testing.T) {
	newHome(t)
	for _, raw := range []string{"not json", "", "[]", "null"} {
		var out bytes.Buffer
		Run("pre-tool", strings.NewReader(raw), &out)
		var o hookOutput
		json.Unmarshal(out.Bytes(), &o)
		if dec(o) != "deny" || o.HookSpecificOutput.PermissionDecisionReason != malformedDenyReason {
			t.Errorf("%q: got %s", raw, out.String())
		}
	}
}
