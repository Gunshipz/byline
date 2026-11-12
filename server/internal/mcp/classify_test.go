package mcp

import (
	"sort"
	"testing"

	"byline/internal/paths"
)

// readOnlyTools are the Byline tools that never change a file;
// writeTools are the ones that can.
var (
	readOnlyTools = []string{"byline_status", "byline_brief", "byline_lint", "byline_factlint",
		"byline_voicecheck", "byline_calibrate", "byline_settings_read"}
	writeTools = []string{"byline_draftcheck", "byline_flag", "byline_setup_save", "byline_settings_save"}
)

// TestTools_EveryToolClassifiedExactlyOnce locks Tools()'s order and checks
// readOnlyTools plus writeTools names every tool Tools()
// returns, exactly once each.
func TestTools_EveryToolClassifiedExactlyOnce(t *testing.T) {
	var names []string
	for _, tl := range Tools() {
		names = append(names, tl.Name)
	}
	want(t, names, []string{"byline_status", "byline_brief", "byline_lint", "byline_factlint", "byline_draftcheck",
		"byline_voicecheck", "byline_flag", "byline_calibrate", "byline_setup_save", "byline_settings_read",
		"byline_settings_save"})
	both := append(append([]string{}, readOnlyTools...), writeTools...)
	sort.Strings(both)
	sorted := append([]string{}, names...)
	sort.Strings(sorted)
	want(t, both, sorted)
}

// TestTools_ReadOnlyToolsWriteNothing proves readOnlyTools really
// is read-only: calling every one of them leaves the voice folder
// byte-for-byte unchanged.
func TestTools_ReadOnlyToolsWriteNothing(t *testing.T) {
	home := setupHome(t)
	writeFile(t, paths.RealVoicePath(), "Thanks, done.\n")
	s := NewServer()
	before := snapshot(t, home)
	args := map[string]map[string]any{
		"byline_lint":       {"text": "Plain."},
		"byline_factlint":   {"draft": "a", "source": "b"},
		"byline_voicecheck": {"text": "Plain."},
		"byline_calibrate": {"corpus": corpusOf(
			"Thanks, done with the quick review of the latest numbers from this week's pilot sites, and I folded the updated figures into the planning deck before sending it around to the wider group ahead of Thursday's budget meeting next week, so everyone has the same view going in.",
			"Sure, I can take the room booking and the updated agenda for Thursday, and I'll loop in the vendor once finance finishes checking the latest numbers against what was in the original proposal from last month's review, before we close anything out this quarter.",
			"See you then. I'll bring the summary notes from this week's reviews and the open questions from last week along with the two items the vendor still owes us answers on before the renewal date next month arrives for the team.",
		)},
	}
	for _, name := range readOnlyTools {
		a := args[name]
		if a == nil {
			a = map[string]any{}
		}
		if _, isErr, text := callTool(t, s, name, a); isErr {
			t.Fatalf("%s: %s", name, text)
		}
	}
	want(t, snapshot(t, home), before)
}
