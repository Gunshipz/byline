package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"byline/internal/paths"
	"byline/internal/state"
	"byline/internal/version"
	"byline/internal/voice"
)

// corpusOf joins msgs into a calibration corpus, each separated by its
// own line of "---".
func corpusOf(msgs ...string) string {
	return strings.Join(msgs, "\n---\n")
}

// assertContainsSwitchOff fails unless v (a decoded would_switch_off or
// switch_off list) holds an entry for ruleID and str.
func assertContainsSwitchOff(t *testing.T, v any, ruleID, str string) {
	t.Helper()
	list, _ := v.([]any)
	for _, item := range list {
		m, _ := item.(map[string]any)
		if m["rule_id"] == ruleID && m["string"] == str {
			return
		}
	}
	t.Fatalf("no switch-off entry for %s/%q in %v", ruleID, str, v)
}

// assertNoViolationFor fails if lintOut's violations include one for
// ruleID.
func assertNoViolationFor(t *testing.T, lintOut map[string]any, ruleID string) {
	t.Helper()
	list, _ := lintOut["violations"].([]any)
	for _, item := range list {
		m, _ := item.(map[string]any)
		if m["rule_id"] == ruleID {
			t.Fatalf("unexpected violation for %q in %v", ruleID, lintOut["violations"])
		}
	}
}

func TestCalibrate_IsAReadOnlyPreview(t *testing.T) {
	home := setupHome(t)
	r := firstCoreStringRule(t)
	before := snapshot(t, home)
	msgs := []string{
		"Hi team, quick note before tomorrow's sync. I " + r.Strings[0] + " when it comes to how the rollout is tracking: the vendor contract for the new scheduling tool is signed, the two pilot sites have confirmed their dates, and the training materials are nearly finished, so unless something changes overnight we're on track for next week's launch without any slip in the plan we agreed on last month.",
		"Thanks, that works for me. I moved our planning review to Thursday afternoon so the whole group can join without conflicting with the budget meeting, and I'll send a short agenda beforehand covering the open questions from last week along with the two items the vendor still owes us answers on.",
		"See you Friday. I'll bring the updated slide deck and the latest numbers from the pilot sites so we can walk through what changed since the last review, and if anyone has feedback on the rollout plan before then, just reply here and I'll fold it in before the meeting starts.",
	}
	corpus := corpusOf(msgs...)
	out, isErr, _ := callTool(t, NewServer(), "byline_calibrate", map[string]any{"corpus": corpus})
	if isErr {
		t.Fatal(out)
	}
	want(t, out["words"], json.Number(fmt.Sprint(len(strings.Fields(strings.Join(msgs, " "))))))
	want(t, out["voice_target"], jsonRoundTrip(t, voice.Calibrate(corpus)))
	assertContainsSwitchOff(t, out["would_switch_off"], r.ID, r.Strings[0])
	want(t, snapshot(t, home), before)
}

func TestCalibrate_NeedsThreeMessages(t *testing.T) {
	setupHome(t)
	s := NewServer()
	for tool, args := range map[string]map[string]any{
		"byline_calibrate":  {"corpus": corpusOf("One.", "Two.")},
		"byline_setup_save": {"profile": "p", "exemplars": "e", "real_voice": corpusOf("One.", "Two.")},
	} {
		_, isErr, text := callTool(t, s, tool, args)
		if !isErr || text != "Byline needs at least 3 of your messages of 40 words or more to measure your voice, separated by lines of ---. Add more and try again." {
			t.Errorf("%s: %q", tool, text)
		}
	}
	assertNoFile(t, paths.RealVoicePath()) // setup save wrote nothing
}

// TestCalibrateAndSetupSave_NewerFormatRefusesWithPlainSentence covers
// the lead ruling (2026-11-10): a my-rules.json saved by a newer version
// of Byline must show the same plain sentence state already uses
// elsewhere for this (friendlyStateError on state.ErrNewerFormat), never
// rules.ParseMyRules' raw "my-rules.json format N is newer..." text.
func TestCalibrateAndSetupSave_NewerFormatRefusesWithPlainSentence(t *testing.T) {
	setupHome(t)
	writeFile(t, paths.MyRulesPath(), `{"format": 2}`)
	want := friendlyStateError(state.ErrNewerFormat).Error()
	s := NewServer()
	for tool, args := range map[string]map[string]any{
		"byline_calibrate":  {"corpus": "short"},
		"byline_setup_save": {"profile": "p", "exemplars": "e", "real_voice": "short"},
	} {
		_, isErr, text := callTool(t, s, tool, args)
		if !isErr || text != want {
			t.Errorf("%s: got %q, want %q", tool, text, want)
		}
	}
	assertNoFile(t, paths.RealVoicePath()) // setup save wrote nothing
}

// Gate finding I3: byline_setup_save must check my-rules.json is readable
// before it writes anything. refuseIfNewerFormat only caught
// ErrNewerFormat; a plain unreadable file (hand-edited, truncated JSON)
// slipped through it and currentRules' silent core-only fallback, so the
// three voice files and the real-voice fixture got written, then
// UpdateMyRules refused -- leaving the person looking set up with no
// voice numbers or switch-offs actually saved.
func TestSetupSave_RefusesWhenPersonalRulesUnreadable(t *testing.T) {
	home := setupHome(t)
	writeFile(t, paths.MyRulesPath(), `{"format":1, "add_rules": [`)
	before := snapshot(t, home)
	_, isErr, text := callTool(t, NewServer(), "byline_setup_save", map[string]any{
		"profile": "p", "exemplars": "e",
		"real_voice": corpusOf(
			"Thanks for the quick update on where things stand with the rollout this week. The pilot sites are still on track for next month, the training materials are nearly finished, and I expect to have the final budget numbers ready before Thursday's planning review so everyone can see the full picture at once.",
			"Sounds good, I'll move our sync to Thursday afternoon so it doesn't conflict with the budget review, and I'll send a short agenda ahead of time covering the open questions from last week along with the two items the vendor still owes us answers on before the renewal date.",
			"Done today with the updated deck and the summary notes from this week's reviews. I moved the remaining open items into next week's agenda, confirmed the room booking for Thursday, and let the vendor know we would follow up once finance finishes checking the latest numbers.",
		),
	})
	if !isErr || !strings.Contains(text, "can't read your personal rules") {
		t.Fatalf("%q", text)
	}
	want(t, snapshot(t, home), before) // profile, exemplars and real-voice all untouched
}

func TestSetupSave_WritesFilesAndSwitchesOffOwnWords(t *testing.T) {
	setupHome(t)
	r := firstCoreStringRule(t)
	s := NewServer()
	real := corpusOf(
		"Thanks all for the quick turnaround on this. I "+r.Strings[0]+" when I'm in a hurry, and today was one of those days: the pilot review ran long, the vendor call moved twice, and I still owe the team notes from Tuesday's planning session before anyone can sign off on the rollout.",
		"Sure, Friday works for me on the room booking and the updated numbers. I'll bring the latest pilot results and the open questions from last week's review so we can close out the budget discussion before the next planning cycle starts, and I'll loop in the vendor if anything still needs a follow-up.",
		"Done, see attached for the updated deck and the summary notes from this week's reviews. I moved the open items into next week's agenda so nothing gets lost, and I flagged the two questions the vendor still owes us answers on before we can close out the pilot phase of the rollout.",
	)
	out, isErr, _ := callTool(t, s, "byline_setup_save", map[string]any{"profile": "# Me\nShort and direct.\n", "exemplars": "## One\nThanks, done.\n", "real_voice": real})
	if isErr {
		t.Fatal(out)
	}
	want(t, readFile(t, paths.ProfilePath()), "# Me\nShort and direct.\n")
	want(t, readFile(t, paths.ExemplarsPath()), "## One\nThanks, done.\n")
	want(t, readFile(t, paths.RealVoicePath()), real)
	doc := decodeUseNumber(t, readFile(t, paths.MyRulesPath()))
	off := doc["switch_off"].(map[string]any)[r.ID].([]any)[0].(map[string]any)
	want(t, off["string"], r.Strings[0])
	if !strings.HasPrefix(off["reason"].(string), "You write this yourself") {
		t.Fatal(off)
	}
	want(t, doc["voice_target"], jsonRoundTrip(t, voice.Calibrate(real)))
	want(t, len(toSlice(out["backups"])), 3) // profile.md, exemplars.md and my-rules.json existed; the real-voice file didn't
	lintOut, _, _ := callTool(t, s, "byline_lint", map[string]any{"text": real})
	assertNoViolationFor(t, lintOut, r.ID)
	st, _, _ := callTool(t, s, "byline_status", map[string]any{})
	want(t, st["setup_done"], true)
	if strings.Contains(st["brief_stamp"].(string), "personal rules saved never") {
		t.Fatal("brief not regenerated")
	}
}

// Gate finding I4: the save result must say what it saved and switched
// off, so the model can report it (SKILL.md step 7) without carrying a
// possibly-stale calibrate preview forward.
func TestSetupSave_ResultReportsSavedPathsSwitchedOffAndBriefStamp(t *testing.T) {
	setupHome(t)
	r := firstCoreStringRule(t)
	out, isErr, _ := callTool(t, NewServer(), "byline_setup_save", map[string]any{"profile": "p", "exemplars": "e", "real_voice": corpusOf(
		"Thanks all for the quick turnaround on this. I "+r.Strings[0]+" when I'm in a hurry, and today was one of those days: the pilot review ran long, the vendor call moved twice, and I still owe the team notes from Tuesday's planning session before anyone can sign off on the rollout.",
		"Sure, Friday works for me on the room booking and the updated numbers. I'll bring the latest pilot results and the open questions from last week's review so we can close out the budget discussion before the next planning cycle starts, and I'll loop in the vendor if anything still needs a follow-up.",
		"Done, see attached for the updated deck and the summary notes from this week's reviews. I moved the open items into next week's agenda so nothing gets lost, and I flagged the two questions the vendor still owes us answers on before we can close out the pilot phase of the rollout.",
	)})
	if isErr {
		t.Fatal(out)
	}
	want(t, out["saved"], []any{paths.ProfilePath(), paths.ExemplarsPath(), paths.RealVoicePath(), paths.MyRulesPath()})
	want(t, out["switched_off"], []any{map[string]any{"rule_id": r.ID, "string": r.Strings[0]}})
	if !strings.HasPrefix(out["brief_stamp"].(string), "<!-- byline "+version.Version()+" |") {
		t.Fatalf("brief_stamp missing or stale: %v", out["brief_stamp"])
	}
}

// Gate finding I4: switched_off lists only the switch-offs this save
// actually appended, not every core hit the corpus would still trip --
// a repeat save of the same corpus has nothing new to add.
func TestSetupSave_SwitchedOffOnlyListsNewlyAddedEntries(t *testing.T) {
	setupHome(t)
	r := firstCoreStringRule(t)
	s := NewServer()
	args := map[string]any{"profile": "p", "exemplars": "e", "real_voice": corpusOf(
		"Thanks all for the quick turnaround on this. I "+r.Strings[0]+" when I'm in a hurry, and today was one of those days: the pilot review ran long, the vendor call moved twice, and I still owe the team notes from Tuesday's planning session before anyone can sign off on the rollout.",
		"Sure, Friday works for me on the room booking and the updated numbers. I'll bring the latest pilot results and the open questions from last week's review so we can close out the budget discussion before the next planning cycle starts, and I'll loop in the vendor if anything still needs a follow-up.",
		"Done, see attached for the updated deck and the summary notes from this week's reviews. I moved the open items into next week's agenda so nothing gets lost, and I flagged the two questions the vendor still owes us answers on before we can close out the pilot phase of the rollout.",
	)}
	out1, isErr, _ := callTool(t, s, "byline_setup_save", args)
	if isErr {
		t.Fatal(out1)
	}
	want(t, out1["switched_off"], []any{map[string]any{"rule_id": r.ID, "string": r.Strings[0]}})
	out2, isErr, _ := callTool(t, s, "byline_setup_save", args) // same corpus again: nothing new to switch off
	if isErr {
		t.Fatal(out2)
	}
	want(t, out2["switched_off"], []any{})
}

func TestSetupSave_KeepsExistingPersonalRulesAndUnknownKeys(t *testing.T) {
	setupHome(t)
	writeFile(t, paths.MyRulesPath(), `{"format":1,"add_rules":[`+ruleJSON("U1", false)+`],"later_key":{"a":1}}`)
	callTool(t, NewServer(), "byline_setup_save", map[string]any{"profile": "p", "exemplars": "e", "real_voice": corpusOf("Plain words only.", "Thanks.", "Done today.")})
	doc := decodeUseNumber(t, readFile(t, paths.MyRulesPath()))
	findRule(t, doc["add_rules"], "U1")
	want(t, doc["later_key"], map[string]any{"a": json.Number("1")})
}

func TestSetupSave_ReportsPersonalRuleHitsWithoutChangingThem(t *testing.T) {
	setupHome(t)
	s := NewServer()
	callTool(t, s, "byline_flag", map[string]any{"phrase": "deep dive", "bad": "A deep dive.", "good": "A look.", "family": 3})
	out, _, _ := callTool(t, s, "byline_setup_save", map[string]any{"profile": "p", "exemplars": "e", "real_voice": corpusOf(
		"Line one covers the quick status update for the week.\nWe did a deep dive into the vendor's pricing model this week, walking through every line item in the proposal with finance so we understood exactly what was changing before the renewal date, and we flagged two questions that still need answers before anyone signs off on the new terms.",
		"Thanks for the quick turnaround on the budget numbers this week. I folded the updated figures into the planning deck and sent it to the wider group so everyone has the same view heading into Thursday's review, and I'll flag anything that shifts again before the meeting so nobody is caught off guard.",
		"Done with the review notes from this morning's planning session. I moved the open action items into next week's agenda, confirmed the room booking for Thursday, and let the vendor know we'd follow up once finance finishes checking the updated numbers against what was in the original proposal.",
	)})
	want(t, out["personal_rule_hits"], []any{map[string]any{"rule_id": "U1", "line": json.Number("2")}})
	findRule(t, decodeUseNumber(t, readFile(t, paths.MyRulesPath()))["add_rules"], "U1") // unchanged, still there
}

func TestSetupSave_RejectsBlank(t *testing.T) {
	setupHome(t)
	_, isErr, text := callTool(t, NewServer(), "byline_setup_save", map[string]any{"profile": "  ", "exemplars": "e", "real_voice": "r"})
	if !isErr || text != `"profile" is empty.` {
		t.Fatalf("%q", text)
	}
}

// TestSetupSave_SavesSignatureLines covers the lead ruling (2026-11-10):
// byline_setup_save takes an optional signature_lines argument, the
// person's own sign-off block, saved into my-rules.json signature_lines.
func TestSetupSave_SavesSignatureLines(t *testing.T) {
	setupHome(t)
	out, isErr, _ := callTool(t, NewServer(), "byline_setup_save", map[string]any{
		"profile": "p", "exemplars": "e",
		"real_voice": corpusOf(
			"Thanks for the quick update on where things stand with the rollout this week. The pilot sites are still on track for next month, the training materials are nearly finished, and I expect to have the final budget numbers ready before Thursday's planning review so everyone can see the full picture at once.",
			"Sounds good, I'll move our sync to Thursday afternoon so it doesn't conflict with the budget review, and I'll send a short agenda ahead of time covering the open questions from last week along with the two items the vendor still owes us answers on before the renewal date.",
			"Done today with the updated deck and the summary notes from this week's reviews. I moved the remaining open items into next week's agenda, confirmed the room booking for Thursday, and let the vendor know we would follow up once finance finishes checking the latest numbers.",
		),
		"signature_lines": []any{"Thanks,", "Sam"},
	})
	if isErr {
		t.Fatal(out)
	}
	doc := decodeUseNumber(t, readFile(t, paths.MyRulesPath()))
	want(t, doc["signature_lines"], []any{"Thanks,", "Sam"})
}

func TestSetupSave_RejectsBlankSignatureLine(t *testing.T) {
	setupHome(t)
	_, isErr, text := callTool(t, NewServer(), "byline_setup_save", map[string]any{
		"profile": "p", "exemplars": "e",
		"real_voice":      corpusOf("Thanks.", "Sounds good.", "Done today."),
		"signature_lines": []any{"Thanks,", "  "},
	})
	if !isErr || text != `"signature_lines" entries must not be blank.` {
		t.Fatalf("%q", text)
	}
	assertNoFile(t, paths.RealVoicePath()) // validated before any write
}

func TestSetupSave_OmittedSignatureLinesLeavesExistingAlone(t *testing.T) {
	setupHome(t)
	writeFile(t, paths.MyRulesPath(), `{"format":1,"signature_lines":["Thanks,","Sam"]}`)
	callTool(t, NewServer(), "byline_setup_save", map[string]any{"profile": "p", "exemplars": "e", "real_voice": corpusOf("Plain words only.", "Thanks.", "Done today.")})
	doc := decodeUseNumber(t, readFile(t, paths.MyRulesPath()))
	want(t, doc["signature_lines"], []any{"Thanks,", "Sam"})
}
