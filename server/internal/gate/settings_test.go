package gate

import (
	"os"
	"slices"
	"strings"
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
	trim := func(tool string) string { return strings.TrimPrefix(tool, "mcp__") }
	cases := map[string]string{
		toolSlackPost:           toolSlackPost,
		inPlugin(toolSlackPost): toolSlackPost,
		"mcp__plugin_Some_Plugin-all_" + trim(toolEmailSend): toolEmailSend,
		toolSlackSearch: "",
		"Write":         "",
		"mcp__plugin_x_" + trim(toolSlackPost) + "_v2": "",
	}
	// Every default loaded through a plugin maps back to itself, never to
	// another default whose name it happens to end with.
	for _, tool := range state.DefaultSendTools {
		cases["mcp__plugin_x_"+trim(tool)] = tool
	}
	for name, want := range cases {
		got, ok := matchSendTool(name, state.DefaultSendTools)
		if got != want || ok != (want != "") {
			t.Errorf("%s: got %q %v, want %q", name, got, ok, want)
		}
	}
}
