package mcp

import (
	"strings"
	"testing"

	"byline/internal/paths"
)

func TestSettingsTools_ReadAndSave(t *testing.T) {
	setupHome(t)
	s := NewServer()
	out, _, _ := callTool(t, s, "byline_settings_read", map[string]any{})
	want(t, out["path"], paths.SettingsPath())
	saved, isErr, _ := callTool(t, s, "byline_settings_save", map[string]any{"exempt_channels": []any{"CTESTCHANNL"}})
	if isErr || saved["backup"] == "" {
		t.Fatal(saved)
	}
	_, isErr, text := callTool(t, s, "byline_settings_save", map[string]any{})
	if !isErr || text != "byline_settings_save needs at least one of: send_tools, exempt_channels, extra_draft_phrases." {
		t.Fatalf("%q", text)
	}
	_, isErr, text = callTool(t, s, "byline_settings_save", map[string]any{"send_tools": []any{requiredSendTool}})
	if !isErr || !strings.Contains(text, "can't remove") {
		t.Fatalf("%q", text)
	}
	if _, isErr, text = callTool(t, s, "byline_settings_save", map[string]any{"send_tools": []any{"ok", 3}}); !isErr || text != `"send_tools" must be a list of strings.` {
		t.Fatalf("%q", text)
	}
}

// TestSettingsTools_NewerFormatIsReportedAsASettingsProblem reproduces
// gate review M2 for the two settings tools themselves (tool_status.go
// already covers byline_status): a newer-format settings.json must
// refuse with a settings-specific sentence, never the personal-rules
// sentence (which names the wrong file and the wrong fix).
func TestSettingsTools_NewerFormatIsReportedAsASettingsProblem(t *testing.T) {
	setupHome(t)
	writeFile(t, paths.SettingsPath(), `{"format":2}`)
	s := NewServer()
	for _, c := range []struct {
		tool string
		args map[string]any
	}{
		{"byline_settings_read", map[string]any{}},
		{"byline_settings_save", map[string]any{"exempt_channels": []any{"CTESTCHANNL"}}},
	} {
		_, isErr, text := callTool(t, s, c.tool, c.args)
		if !isErr {
			t.Fatalf("%s: want an error for a newer-format settings.json", c.tool)
		}
		if !strings.Contains(text, "settings") {
			t.Errorf("%s: %q doesn't mention settings", c.tool, text)
		}
		if strings.Contains(text, "personal rules") {
			t.Errorf("%s: %q used the personal-rules sentence", c.tool, text)
		}
	}
}

// TestSettingsTools_NoTelemetry reproduces gate review I5: the telemetry
// setting was saved but nothing honoured it, so a person who opted out
// was told usage counting was off while it carried on. The setting is
// dropped entirely: neither tool mentions, accepts or returns it.
func TestSettingsTools_NoTelemetry(t *testing.T) {
	setupHome(t)
	s := NewServer()
	out, _, _ := callTool(t, s, "byline_settings_read", map[string]any{})
	if _, has := out["settings"].(map[string]any)["telemetry"]; has {
		t.Fatalf("byline_settings_read still returns telemetry: %v", out["settings"])
	}
	_, isErr, text := callTool(t, s, "byline_settings_save", map[string]any{"telemetry": false})
	if !isErr {
		t.Fatal("byline_settings_save still accepts telemetry, want it refused as an unknown argument")
	}
	if want := `byline_settings_save does not take "telemetry". It takes: send_tools, exempt_channels, extra_draft_phrases.`; text != want {
		t.Fatalf("got %q, want %q", text, want)
	}
}
