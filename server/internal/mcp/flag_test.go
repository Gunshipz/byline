package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"byline/internal/paths"
)

// ruleJSON renders a minimal valid add_rules entry with the given id,
// retired or not.
func ruleJSON(id string, retired bool) string {
	return fmt.Sprintf(`{"id":%q,"family":1,"name":"n","ban":"b","strings":["x"],"fix":"f","bad":"a","good":"c","retired":%t}`, id, retired)
}

func flag(t *testing.T, s *Server, args map[string]any) (map[string]any, bool, string) {
	return callTool(t, s, "byline_flag", args)
}

// findRule returns the rule with id from v (a decoded add_rules list).
func findRule(t *testing.T, v any, id string) map[string]any {
	t.Helper()
	list, _ := v.([]any)
	for _, item := range list {
		m, ok := item.(map[string]any)
		if ok && m["id"] == id {
			return m
		}
	}
	t.Fatalf("no rule %q in %v", id, v)
	return nil
}

// lastJSONLine decodes the last non-empty line of path.
func lastJSONLine(t *testing.T, path string) map[string]any {
	t.Helper()
	data := readFile(t, path)
	lines := strings.Split(strings.TrimRight(data, "\n"), "\n")
	last := lines[len(lines)-1]
	return decodeUseNumber(t, last)
}

// assertViolationFor fails unless lintOut's violations include one for
// ruleID.
func assertViolationFor(t *testing.T, lintOut map[string]any, ruleID string) {
	t.Helper()
	list, _ := lintOut["violations"].([]any)
	for _, item := range list {
		m, _ := item.(map[string]any)
		if m["rule_id"] == ruleID {
			return
		}
	}
	t.Fatalf("no violation for %q in %v", ruleID, lintOut["violations"])
}

func TestFlag_AddsPersonalRuleAndRegeneratesBrief(t *testing.T) {
	setupHome(t)
	writeFile(t, paths.RealVoicePath(), "Thanks, I'll send it Friday.\n")
	s := NewServer()
	out, isErr, _ := flag(t, s, map[string]any{"phrase": "deep dive", "bad": "Let's do a deep dive on costs.", "good": "Let's go through costs line by line.", "family": 3})
	if isErr || out["saved"] != true || out["rule_id"] != "U1" || out["created"] != true || out["real_voice_checked"] != true {
		t.Fatal(out)
	}
	rule := findRule(t, decodeUseNumber(t, readFile(t, paths.MyRulesPath()))["add_rules"], "U1")
	want(t, rule["strings"], []any{"deep dive"})
	want(t, rule["ban"], "deep dive")
	want(t, rule["family"], json.Number("3"))
	want(t, rule["bad"], "Let's do a deep dive on costs.")
	want(t, rule["good"], "Let's go through costs line by line.")
	want(t, rule["flagged"], time.Now().Format("2006-01-02"))
	brief := readFile(t, paths.BriefPath())
	// rules.Brief only ever renders a rule's banned strings, never its bad/
	// good examples (confirmed against core rules too: no core rule's
	// "good" text appears in a generated brief either), so only the new
	// string is checked here.
	if !strings.Contains(brief, "deep dive") {
		t.Fatal("brief not regenerated")
	}
	if strings.Contains(strings.SplitN(brief, "\n", 2)[0], "personal rules saved never") {
		t.Fatal("stamp not updated")
	}
	want(t, len(glob(t, filepath.Join(paths.BackupsDir(), "my-rules.json.*.bak"))), 1)
	last := lastJSONLine(t, paths.FailuresPath())
	want(t, last, map[string]any{"date": time.Now().Format("2006-01-02"), "rule": "U1", "text": "Let's do a deep dive on costs.", "layer": "lint", "rule_exists": false, "note": "flagged phrase: deep dive"})
	lintOut, _, _ := callTool(t, s, "byline_lint", map[string]any{"text": "We need a deep dive."})
	assertViolationFor(t, lintOut, "U1")
}

func TestFlag_RefusesWhenRuleFiresOnRealVoice(t *testing.T) {
	setupHome(t)
	writeFile(t, paths.RealVoicePath(), "Morning all.\r\nI'll circle back on this tomorrow.\r\n")
	before := snapshot(t, os.Getenv("BYLINE_HOME"))
	out, isErr, _ := flag(t, NewServer(), map[string]any{"phrase": "circle back", "bad": "Let's circle back next week.", "good": "Let's decide next week.", "family": 2})
	if isErr || out["saved"] != false || out["reason"] != "fires_on_your_writing" {
		t.Fatal(out)
	}
	want(t, out["rule_id"], "U1") // gate finding M6: the refusal names which rule was rejected
	want(t, out["lines"], []any{map[string]any{"line": json.Number("2"), "text": "I'll circle back on this tomorrow."}})
	want(t, snapshot(t, os.Getenv("BYLINE_HOME")), before) // nothing saved, backed up, logged or regenerated
}

func TestFlag_ExtendsCoreRuleWithOwnExample(t *testing.T) {
	setupHome(t)
	r := firstCoreStringRule(t)
	s := NewServer()
	out, _, _ := flag(t, s, map[string]any{"rule_id": r.ID, "phrase": "zzqx way", "bad": "The zzqx way is best.", "good": "This way is best."})
	if out["saved"] != true || out["created"] != false {
		t.Fatal(out)
	}
	flag(t, s, map[string]any{"rule_id": r.ID, "phrase": "zzqy way", "bad": "A zzqy way out.", "good": "A way out."})
	doc := decodeUseNumber(t, readFile(t, paths.MyRulesPath()))
	want(t, doc["add_strings"].(map[string]any)[r.ID], []any{"zzqx way", "zzqy way"})
	want(t, doc["replace_examples"].(map[string]any)[r.ID], map[string]any{"bad": "The zzqx way is best.", "good": "This way is best."}) // first own example kept
	want(t, lastJSONLine(t, paths.FailuresPath())["rule_exists"], true)
}

func TestFlag_ExtendsPersonalRule(t *testing.T) {
	setupHome(t)
	s := NewServer()
	flag(t, s, map[string]any{"phrase": "deep dive", "bad": "A deep dive.", "good": "A look.", "family": 3})
	flag(t, s, map[string]any{"rule_id": "U1", "phrase": "Deep-dive", "bad": "Deep-dive it.", "good": "Look at it."})
	flag(t, s, map[string]any{"rule_id": "U1", "phrase": "DEEP DIVE", "bad": "DEEP DIVE now.", "good": "Look now."}) // same string, any case: not added twice
	rule := findRule(t, decodeUseNumber(t, readFile(t, paths.MyRulesPath()))["add_rules"], "U1")
	want(t, rule["strings"], []any{"deep dive", "Deep-dive"})
	want(t, rule["bad"], "A deep dive.")
}

func TestFlag_NextIDNeverReused(t *testing.T) {
	setupHome(t)
	writeFile(t, paths.MyRulesPath(), `{"format":1,"add_rules":[`+ruleJSON("U1", false)+`,`+ruleJSON("U3", true)+`]}`) // U3 retired
	out, _, _ := flag(t, NewServer(), map[string]any{"phrase": "game changer", "bad": "A game changer.", "good": "A big help.", "family": 1})
	want(t, out["rule_id"], "U4")
}

func TestFlag_ArgumentRules(t *testing.T) {
	setupHome(t)
	s := NewServer()
	for _, c := range []struct {
		args map[string]any
		msg  string
	}{
		{map[string]any{"phrase": "deep dive", "bad": "No phrase here.", "good": "x", "family": 1}, `"bad" must contain the phrase "deep dive".`},
		{map[string]any{"phrase": "deep dive", "bad": "A deep dive.", "good": "Another deep dive.", "family": 1}, `"good" must not contain the phrase "deep dive".`},
		{map[string]any{"phrase": "deep dive", "bad": "A deep dive.", "good": "A look."}, `"family" (1 to 6) is needed to add a new rule.`},
		{map[string]any{"phrase": "deep dive", "bad": "A deep dive.", "good": "A look.", "family": 7}, `"family" must be 1 to 6.`},
		{map[string]any{"rule_id": "NOPE", "phrase": "deep dive", "bad": "A deep dive.", "good": "A look."}, `There's no rule "NOPE".`},
	} {
		if _, isErr, text := flag(t, s, c.args); !isErr || text != c.msg {
			t.Errorf("%v: %q", c.args, text)
		}
	}
	retired := firstRetiredCoreRule(t) // skip when core has none
	if _, isErr, text := flag(t, s, map[string]any{"rule_id": retired.ID, "phrase": "zz q", "bad": "zz q", "good": "x"}); !isErr || !strings.Contains(text, "retired") {
		t.Errorf("%q", text)
	}
}

// Gate finding M11: flagging a rule_id while my-rules.json is unreadable
// must give the unreadable-file sentence, not "There's no rule", which
// checked only the core-only fallback set currentRules silently degrades
// to.
func TestFlag_WithRuleIDWhileRulesUnreadableGivesUnreadableFileSentence(t *testing.T) {
	setupHome(t)
	writeFile(t, paths.MyRulesPath(), "{not json")
	_, isErr, text := flag(t, NewServer(), map[string]any{"rule_id": "U1", "phrase": "deep dive", "bad": "A deep dive.", "good": "A look."})
	if !isErr || !strings.Contains(text, "can't read your personal rules") {
		t.Fatalf("%q", text)
	}
}

func TestFlag_WithoutRealVoiceSavesAndSaysSo(t *testing.T) {
	setupHome(t)
	out, _, _ := flag(t, NewServer(), map[string]any{"phrase": "deep dive", "bad": "A deep dive.", "good": "A look.", "family": 3})
	want(t, out["saved"], true)
	want(t, out["real_voice_checked"], false)
}

func TestFlag_RefusesWhenPersonalRulesUnreadable(t *testing.T) {
	setupHome(t)
	writeFile(t, paths.MyRulesPath(), "{broken")
	_, isErr, text := flag(t, NewServer(), map[string]any{"phrase": "deep dive", "bad": "A deep dive.", "good": "A look.", "family": 3})
	if !isErr || !strings.Contains(text, "can't read your personal rules") {
		t.Fatalf("%q", text)
	}
	want(t, readFile(t, paths.MyRulesPath()), "{broken")
}

// I4: when every narrower phrase also fires on the person's own writing,
// flag mode saves a pattern with no fixed phrase. byline_flag takes a
// judgment "test" in place of "phrase" (the lead decision) and saves a
// rule with no strings that the brief prints as "name: test".
func TestFlag_I4_SavesPatternAsTest(t *testing.T) {
	setupHome(t)
	writeFile(t, paths.RealVoicePath(), "Thanks, I'll send it Friday.\n")
	s := NewServer()
	test := "A closing line that restates the ask the opening already made."
	out, isErr, text := flag(t, s, map[string]any{"test": test, "name": "Restated ask", "family": 6,
		"bad": "Can you send the file? Again, please send the file.", "good": "Can you send the file?"})
	if isErr || out["saved"] != true || out["rule_id"] != "U1" || out["created"] != true || out["real_voice_checked"] != true {
		t.Fatalf("%v %q", out, text)
	}
	rule := findRule(t, decodeUseNumber(t, readFile(t, paths.MyRulesPath()))["add_rules"], "U1")
	if _, has := rule["strings"]; has {
		t.Fatalf("a pattern rule must have no strings: %v", rule)
	}
	want(t, rule["test"], test)
	want(t, rule["ban"], test)
	want(t, rule["name"], "Restated ask")
	if brief := readFile(t, paths.BriefPath()); !strings.Contains(brief, "restated ask: A closing line that restates the ask the opening already made") {
		t.Fatalf("brief doesn't print the pattern as name: test:\n%s", brief)
	}
	want(t, lastJSONLine(t, paths.FailuresPath())["note"], "flagged pattern: "+test)
}

func TestFlag_I4_PhraseOrTestArgumentRules(t *testing.T) {
	setupHome(t)
	s := NewServer()
	for _, c := range []struct {
		args map[string]any
		msg  string
	}{
		{map[string]any{"bad": "A deep dive.", "good": "A look.", "family": 3}, `"phrase" or "test" is needed.`},
		{map[string]any{"phrase": "deep dive", "test": "Any deep look.", "bad": "A deep dive.", "good": "A look.", "family": 3}, `Give "phrase" or "test", not both.`},
		{map[string]any{"rule_id": "U1", "test": "Any deep look.", "bad": "A deep dive.", "good": "A look."}, `"test" adds a brand-new rule, so give "family" instead of "rule_id".`},
	} {
		if _, isErr, text := flag(t, s, c.args); !isErr || text != c.msg {
			t.Errorf("%v: %q", c.args, text)
		}
	}
}
