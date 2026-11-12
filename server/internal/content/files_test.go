package content

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"byline/internal/state"
)

// TestTemplates_Shape checks the five voice-folder templates that Phase 4's
// install copies into $BYLINE_HOME: their JSON shape, and the locked
// template marker on the two files setup replaces.
func TestTemplates_Shape(t *testing.T) {
	t.Run("my-rules.json", func(t *testing.T) {
		var got map[string]any
		data := readFile(t, "templates/my-rules.json")
		if err := json.Unmarshal([]byte(data), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		want := map[string]any{
			"format":           float64(1),
			"add_rules":        []any{},
			"add_strings":      map[string]any{},
			"switch_off":       map[string]any{},
			"replace_examples": map[string]any{},
			"voice_target":     map[string]any{},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("my-rules.json = %#v, want %#v", got, want)
		}
	})

	t.Run("settings.json", func(t *testing.T) {
		var got map[string]any
		data := readFile(t, "templates/settings.json")
		if err := json.Unmarshal([]byte(data), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(got) != 4 {
			t.Fatalf("settings.json has keys %v, want exactly format, send_tools, exempt_channels, extra_draft_phrases", got)
		}
		if format, _ := got["format"].(float64); format != 1 {
			t.Fatalf("format = %v, want 1", got["format"])
		}
		if ec, ok := got["exempt_channels"].([]any); !ok || len(ec) != 0 {
			t.Fatalf("exempt_channels = %v, want []", got["exempt_channels"])
		}
		if dp, ok := got["extra_draft_phrases"].([]any); !ok || len(dp) != 0 {
			t.Fatalf("extra_draft_phrases = %v, want []", got["extra_draft_phrases"])
		}
		if _, has := got["telemetry"]; has {
			t.Fatalf("settings.json still has a telemetry key, want it dropped entirely: %v", got)
		}
		var wantTools []any
		for _, tool := range state.DefaultSendTools {
			wantTools = append(wantTools, tool)
		}
		if !reflect.DeepEqual(got["send_tools"], wantTools) {
			t.Fatalf("send_tools = %v, want %v", got["send_tools"], wantTools)
		}
	})

	const marker = "<!-- byline template: setup replaces this file -->"

	t.Run("profile.md starts with the template marker", func(t *testing.T) {
		data := readFile(t, "templates/profile.md")
		if firstLine(data) != marker {
			t.Fatalf("first line = %q, want %q", firstLine(data), marker)
		}
	})

	t.Run("exemplars.md starts with the template marker", func(t *testing.T) {
		data := readFile(t, "templates/exemplars.md")
		if firstLine(data) != marker {
			t.Fatalf("first line = %q, want %q", firstLine(data), marker)
		}
	})

	t.Run("my-stance.md has no template marker", func(t *testing.T) {
		data := readFile(t, "templates/my-stance.md")
		if strings.Contains(data, marker) {
			t.Fatal("my-stance.md contains the template marker, want none")
		}
	})

	t.Run("exemplars.md has no sample text", func(t *testing.T) {
		data := readFile(t, "templates/exemplars.md")
		for _, line := range strings.Split(data, "\n") {
			if strings.HasPrefix(line, ">") {
				t.Fatalf("exemplars.md has a line starting with %q: %q", ">", line)
			}
		}
	})
}

// firstLine returns the text before the first newline.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
