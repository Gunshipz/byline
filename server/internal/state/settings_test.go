package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"byline/internal/paths"
)

func TestSettings_DefaultsForMissingKeys(t *testing.T) {
	testHome(t)
	EnsureVoiceFolder()
	writeFile(t, paths.SettingsPath(), `{"format":1}`)
	s, err := LoadSettings()
	if err != nil || !reflect.DeepEqual(s.SendTools, DefaultSendTools) || len(s.ExemptChannels) != 0 {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestSettings_HandEditCannotDropADefault(t *testing.T) {
	testHome(t)
	EnsureVoiceFolder()
	writeFile(t, paths.SettingsPath(), `{"format":1,"send_tools":["mcp__x__y"]}`)
	s, _ := LoadSettings()
	want(t, s.SendTools, append(append([]string{}, DefaultSendTools...), "mcp__x__y"))
}

func TestSettings_TemplateMatchesDefaults(t *testing.T) {
	var s Settings
	json.Unmarshal([]byte(tmpl(t, "settings.json")), &s)
	want(t, s.SendTools, DefaultSendTools)
	want(t, s.Format, 1)
}

func TestSettings_ValidateRejects(t *testing.T) {
	ok := Settings{Format: 1, SendTools: DefaultSendTools}
	for name, c := range map[string]struct {
		mut func(*Settings)
		msg string
	}{
		"drops a default":    {func(s *Settings) { s.SendTools = DefaultSendTools[1:] }, fmt.Sprintf("can't remove %q", DefaultSendTools[0])},
		"not a tool name":    {func(s *Settings) { s.SendTools = append(DefaultSendTools, "slack") }, `"slack" isn't a tool name`},
		"not a channel id":   {func(s *Settings) { s.ExemptChannels = []string{"general"} }, `"general" isn't a Slack channel id`},
		"empty draft phrase": {func(s *Settings) { s.ExtraDraftPhrases = []string{" "} }, "empty"},
	} {
		s := ok
		c.mut(&s)
		if err := ValidateSettings(s); err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := ValidateSettings(ok); err != nil {
		t.Fatal(err)
	}
}

func TestSettings_SavePatchKeepsOtherKeysAndBacksUp(t *testing.T) {
	testHome(t)
	EnsureVoiceFolder()
	writeFile(t, paths.SettingsPath(), `{"format":1,"send_tools":[],"custom":"keep me"}`)
	s, backup, err := SaveSettings(map[string]any{"exempt_channels": []any{"CTESTCHANNL"}})
	if err != nil || backup == "" || !reflect.DeepEqual(s.ExemptChannels, []string{"CTESTCHANNL"}) {
		t.Fatalf("%+v %q %v", s, backup, err)
	}
	got := decodeUseNumber(t, readFile(t, paths.SettingsPath()))
	want(t, got["custom"], "keep me")
}

// TestSettings_MissingListsReadAsEmptyNeverNull reproduces gate review
// M5: exempt_channels and extra_draft_phrases came back as a Go nil
// slice when the key was missing, which encodes to JSON null instead of
// [].
func TestSettings_MissingListsReadAsEmptyNeverNull(t *testing.T) {
	testHome(t)
	EnsureVoiceFolder()
	writeFile(t, paths.SettingsPath(), `{"format":1}`)
	s, err := LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if s.ExemptChannels == nil {
		t.Fatal("ExemptChannels is nil, want a non-nil empty slice")
	}
	if s.ExtraDraftPhrases == nil {
		t.Fatal("ExtraDraftPhrases is nil, want a non-nil empty slice")
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	json.Unmarshal(b, &got)
	if got["exempt_channels"] == nil {
		t.Fatalf("exempt_channels encodes as JSON null: %s", b)
	}
	if got["extra_draft_phrases"] == nil {
		t.Fatalf("extra_draft_phrases encodes as JSON null: %s", b)
	}
}

// TestSettings_NullIsUnreadableNotAPanic reproduces gate review M1: a
// settings.json of exactly `null` decodes to a nil map with no error, so
// SaveSettings's "doc[k] = v" panicked on assignment into a nil map
// instead of refusing cleanly.
func TestSettings_NullIsUnreadableNotAPanic(t *testing.T) {
	testHome(t)
	EnsureVoiceFolder()
	writeFile(t, paths.SettingsPath(), "null")
	if _, err := LoadSettings(); err == nil {
		t.Fatal("LoadSettings: want an error for a null document, not success")
	}
	if _, _, err := SaveSettings(map[string]any{"exempt_channels": []any{"CTESTCHANNL"}}); err == nil {
		t.Fatal("SaveSettings: want an error for a null document, not a panic")
	}
	want(t, readFile(t, paths.SettingsPath()), "null") // never rewritten
}

// TestSettings_SaveRetriesOnConcurrentWrite reproduces gate review M10:
// SaveSettings had no re-read-and-retry like UpdateMyRules already has,
// so when another session's save landed between this save's read and
// its write, whichever write lands last won, silently dropping the
// other save -- only the backup showed it ever existed. Deterministic
// injection (settingsRaceHookForTests), the same shape as
// TestState_UpdateMyRulesRetriesOnConcurrentWrite's callback trick,
// stands in for a second process saving at the worst moment: real
// concurrent goroutines in one test process would instead collide on
// WriteFileAtomic's single pid-named temp file, a different, unrelated
// problem.
func TestSettings_SaveRetriesOnConcurrentWrite(t *testing.T) {
	testHome(t)
	EnsureVoiceFolder()
	calls := 0
	settingsRaceHookForTests = func() {
		calls++
		if calls == 1 { // another session saves between our read and our write
			writeFile(t, paths.SettingsPath(), `{"format":1,"send_tools":[],"other":"their save"}`)
		}
	}
	defer func() { settingsRaceHookForTests = nil }()
	s, _, err := SaveSettings(map[string]any{"exempt_channels": []any{"CTESTCHANNL"}})
	if err != nil || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
	want(t, s.ExemptChannels, []string{"CTESTCHANNL"}) // ours
	got := decodeUseNumber(t, readFile(t, paths.SettingsPath()))
	want(t, got["other"], "their save") // theirs, not dropped
}

// TestSettings_NoTelemetryField reproduces gate review I5: the telemetry
// setting was saved but nothing honoured it, so a person who opted out
// was told usage counting was off while it carried on. The setting is
// dropped entirely, so a save never writes a telemetry key, even when an
// old file on disk still carries one from before this fix.
func TestSettings_NoTelemetryField(t *testing.T) {
	testHome(t)
	EnsureVoiceFolder()
	writeFile(t, paths.SettingsPath(), `{"format":1,"send_tools":[],"telemetry":false}`)
	s, backup, err := SaveSettings(map[string]any{"exempt_channels": []any{"CTESTCHANNL"}})
	if err != nil || backup == "" {
		t.Fatalf("%+v %q %v", s, backup, err)
	}
	got := decodeUseNumber(t, readFile(t, paths.SettingsPath()))
	if _, has := got["telemetry"]; has {
		t.Fatalf("settings.json still carries telemetry after save: %v", got)
	}
}

// TestSettings_TrailingContentIsUnreadable reproduces gate review I2's
// settings.json repro: a hand edit that closes the object too early and
// leaves more text after it. The file must be reported unreadable and
// never rewritten.
func TestSettings_TrailingContentIsUnreadable(t *testing.T) {
	testHome(t)
	EnsureVoiceFolder()
	trailing := `{"format":1,"send_tools":[]}` + "\n" +
		`, "exempt_channels":["CTESTCHANNL"], "extra_draft_phrases":["quick note to"]}`
	writeFile(t, paths.SettingsPath(), trailing)
	if _, err := LoadSettings(); err == nil {
		t.Fatal("LoadSettings: want an error for trailing content")
	}
	if _, _, err := SaveSettings(map[string]any{"exempt_channels": []any{"CTESTCHANNL"}}); err == nil {
		t.Fatal("SaveSettings: want an error for trailing content")
	}
	want(t, readFile(t, paths.SettingsPath()), trailing) // never rewritten
}

func TestSettings_NewerFormatRefused(t *testing.T) {
	testHome(t)
	EnsureVoiceFolder()
	writeFile(t, paths.SettingsPath(), `{"format":2}`)
	if _, _, err := SaveSettings(map[string]any{"exempt_channels": []any{"CTESTCHANNL"}}); !errors.Is(err, ErrNewerFormat) {
		t.Fatal(err)
	}
	want(t, readFile(t, paths.SettingsPath()), `{"format":2}`)
}

// A file saved by a Windows editor can start with a UTF-8 BOM (gate review M2).
func TestState_LeadingBOMStillReads(t *testing.T) {
	testHome(t)
	EnsureVoiceFolder()
	writeFile(t, paths.SettingsPath(), "\xef\xbb\xbf"+`{"format":1,"send_tools":["mcp__x__y"]}`)
	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("settings.json with a BOM: %v", err)
	}
	want(t, s.SendTools, append(append([]string{}, DefaultSendTools...), "mcp__x__y"))
	writeFile(t, paths.MyRulesPath(), "\xef\xbb\xbf"+`{"format":1,"known_names":["Jordan"]}`)
	if _, err := LoadMyRules(); err != nil {
		t.Fatalf("my-rules.json with a BOM: %v", err)
	}
}
