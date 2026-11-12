package gate

import (
	"os"
	"slices"
	"testing"

	"byline/internal/paths"
	"byline/internal/state"
)

func TestGateSettings_ErrorFallsBackToDefaults(t *testing.T) {
	newHome(t)
	if s := gateSettings(); !slices.Equal(s.ExemptChannels, []string{"TESTBOT0001"}) {
		t.Fatalf("a good file must be read through state: %+v", s)
	}
	for _, body := range []string{"not json", `{"format":2}`} {
		os.WriteFile(paths.SettingsPath(), []byte(body), 0o600)
		s := gateSettings()
		if !slices.Equal(s.SendTools, state.DefaultSendTools) || len(s.ExemptChannels) != 0 || len(s.ExtraDraftPhrases) != 0 {
			t.Errorf("%s: got %+v, want the defaults and nothing exempt", body, s)
		}
		if b, _ := os.ReadFile(paths.SettingsPath()); string(b) != body {
			t.Errorf("%s: the gate must never rewrite settings.json", body)
		}
	}
}

func TestMatchSendTool(t *testing.T) {
	for name, want := range map[string]string{
		"mcp__slack__conversations_add_message":                                  "mcp__slack__conversations_add_message",
		"mcp__plugin_SomePlugin_slack__conversations_add_message":                "mcp__slack__conversations_add_message",
		"mcp__plugin_Some_Plugin-all_claude_ai_Microsoft_365__outlook_send_mail": "mcp__claude_ai_Microsoft_365__outlook_send_mail",
		"mcp__plugin_x_slack__conversations_add_message":                         "mcp__slack__conversations_add_message",
		"mcp__slack__conversations_search_messages":                              "",
		"Write": "",
		"mcp__plugin_x_slack__conversations_add_message_v2": "",
	} {
		got, ok := matchSendTool(name, state.DefaultSendTools)
		if got != want || ok != (want != "") {
			t.Errorf("%s: got %q %v, want %q", name, got, ok, want)
		}
	}
}
