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

// filler returns a string of exactly n space-separated words, so a test
// can build a calibration corpus that reaches minSetupWords without
// hand-typing thousands of words.
func filler(n int) string {
	return strings.Repeat("word ", n-1) + "word"
}

// underMinCorpus is 3 real messages (each over voice.Calibrate's 3-of-40
// floor) that together hold well under minSetupWords.
var underMinCorpus = corpusOf(
	"Thanks for the quick update on where things stand with the rollout this week. The pilot sites are still on track for next month, the training materials are nearly finished, and I expect to have the final budget numbers ready before Thursday's planning review so everyone can see the full picture at once.",
	"Sounds good, I'll move our sync to Thursday afternoon so it doesn't conflict with the budget review, and I'll send a short agenda ahead of time covering the open questions from last week along with the two items the vendor still owes us answers on before the renewal date.",
	"Done today with the updated deck and the summary notes from this week's reviews. I moved the remaining open items into next week's agenda, confirmed the room booking for Thursday, and let the vendor know we would follow up once finance finishes checking the latest numbers.",
)

// atOrAboveMinCorpus is 3 qualifying messages whose total is comfortably
// at or above minSetupWords (1000), without hand-typing that much text.
var atOrAboveMinCorpus = corpusOf(filler(1400), filler(1400), filler(1400))

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

// assertNoSwitchOff fails if v (a decoded would_switch_off or switch_off
// list) holds an entry for ruleID and str.
func assertNoSwitchOff(t *testing.T, v any, ruleID, str string) {
	t.Helper()
	list, _ := v.([]any)
	for _, item := range list {
		m, _ := item.(map[string]any)
		if m["rule_id"] == ruleID && m["string"] == str {
			t.Fatalf("unexpected switch-off entry for %s/%q in %v", ruleID, str, v)
		}
	}
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
		"Thanks, that works for me. I " + r.Strings[0] + " too on this one, and I moved our planning review to Thursday afternoon so the whole group can join without conflicting with the budget meeting, and I'll send a short agenda beforehand covering the open questions from last week along with the two items the vendor still owes us answers on.",
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
		"Sure, Friday works for me on the room booking and the updated numbers. I "+r.Strings[0]+" on this too, and I'll bring the latest pilot results and the open questions from last week's review so we can close out the budget discussion before the next planning cycle starts, and I'll loop in the vendor if anything still needs a follow-up.",
		"Done, see attached for the updated deck and the summary notes from this week's reviews. I moved the open items into next week's agenda so nothing gets lost, and I flagged the two questions the vendor still owes us answers on before we can close out the pilot phase of the rollout.",
	)
	out, isErr, _ := callTool(t, s, "byline_setup_save", map[string]any{"profile": "# Me\nShort and direct.\n", "exemplars": "## One\nThanks, done.\n", "real_voice": real, "accept_fewer_words": true})
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
	out, isErr, _ := callTool(t, NewServer(), "byline_setup_save", map[string]any{"profile": "p", "exemplars": "e", "accept_fewer_words": true, "real_voice": corpusOf(
		"Thanks all for the quick turnaround on this. I "+r.Strings[0]+" when I'm in a hurry, and today was one of those days: the pilot review ran long, the vendor call moved twice, and I still owe the team notes from Tuesday's planning session before anyone can sign off on the rollout.",
		"Sure, Friday works for me on the room booking and the updated numbers. I "+r.Strings[0]+" on this too, and I'll bring the latest pilot results and the open questions from last week's review so we can close out the budget discussion before the next planning cycle starts, and I'll loop in the vendor if anything still needs a follow-up.",
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
	args := map[string]any{"profile": "p", "exemplars": "e", "accept_fewer_words": true, "real_voice": corpusOf(
		"Thanks all for the quick turnaround on this. I "+r.Strings[0]+" when I'm in a hurry, and today was one of those days: the pilot review ran long, the vendor call moved twice, and I still owe the team notes from Tuesday's planning session before anyone can sign off on the rollout.",
		"Sure, Friday works for me on the room booking and the updated numbers. I "+r.Strings[0]+" on this too, and I'll bring the latest pilot results and the open questions from last week's review so we can close out the budget discussion before the next planning cycle starts, and I'll loop in the vendor if anything still needs a follow-up.",
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
	out, _, _ := callTool(t, s, "byline_setup_save", map[string]any{"profile": "p", "exemplars": "e", "accept_fewer_words": true, "real_voice": corpusOf(
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
		"profile": "p", "exemplars": "e", "accept_fewer_words": true,
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

// TestSetupSave_RefusesBelowMinimumWords: a real-voice sample under
// minSetupWords is refused, in one plain sentence, and nothing is written.
func TestSetupSave_RefusesBelowMinimumWords(t *testing.T) {
	home := setupHome(t)
	before := snapshot(t, home)
	_, isErr, text := callTool(t, NewServer(), "byline_setup_save", map[string]any{"profile": "p", "exemplars": "e", "real_voice": underMinCorpus})
	if !isErr || !strings.Contains(text, fmt.Sprintf("fewer than the %d", minSetupWords)) {
		t.Fatalf("got %q", text)
	}
	want(t, snapshot(t, home), before)
}

// TestSetupSave_SavesWithAcceptFewerWords: the same under-minimum sample
// saves once accept_fewer_words is true.
func TestSetupSave_SavesWithAcceptFewerWords(t *testing.T) {
	setupHome(t)
	out, isErr, _ := callTool(t, NewServer(), "byline_setup_save", map[string]any{
		"profile": "p", "exemplars": "e", "real_voice": underMinCorpus, "accept_fewer_words": true,
	})
	if isErr {
		t.Fatal(out)
	}
	want(t, readFile(t, paths.RealVoicePath()), underMinCorpus)
}

// TestSetupSave_SavesAtOrAboveMinimumWords: a sample at or above
// minSetupWords saves with no accept_fewer_words needed.
func TestSetupSave_SavesAtOrAboveMinimumWords(t *testing.T) {
	setupHome(t)
	out, isErr, _ := callTool(t, NewServer(), "byline_setup_save", map[string]any{"profile": "p", "exemplars": "e", "real_voice": atOrAboveMinCorpus})
	if isErr {
		t.Fatal(out)
	}
	words, _ := out["words"].(json.Number).Int64()
	if words < minSetupWords {
		t.Fatalf("got %d words, want at least %d", words, minSetupWords)
	}
	want(t, readFile(t, paths.RealVoicePath()), atOrAboveMinCorpus)
}

// TestCalibrate_NotesUnderMinimumWithoutRefusing: byline_calibrate never
// refuses for too few words; it adds a note only when the sample is under
// minSetupWords.
func TestCalibrate_NotesUnderMinimumWithoutRefusing(t *testing.T) {
	setupHome(t)
	s := NewServer()

	under, isErr, _ := callTool(t, s, "byline_calibrate", map[string]any{"corpus": underMinCorpus})
	if isErr {
		t.Fatal(under)
	}
	note, _ := under["note"].(string)
	if !strings.Contains(note, fmt.Sprintf("fewer than the %d", minSetupWords)) {
		t.Fatalf("want an under-minimum note, got %v", under)
	}

	atOrAbove, isErr, _ := callTool(t, s, "byline_calibrate", map[string]any{"corpus": atOrAboveMinCorpus})
	if isErr {
		t.Fatal(atOrAbove)
	}
	if _, ok := atOrAbove["note"]; ok {
		t.Fatalf("want no note at or above the minimum, got %v", atOrAbove)
	}
}

// TestCalibrate_LeavesOutAMarkdownMessage: a message with markdown
// structure (here, a bulleted list) is left out of the corpus entirely --
// it's not in "words", and it's reported through left_out/left_out_words.
func TestCalibrate_LeavesOutAMarkdownMessage(t *testing.T) {
	setupHome(t)
	md := "- Shipped the thing\n- Fixed a bug\n- Closed the ticket and let finance know the numbers were correct before the renewal date arrived next month for everyone involved in the review."
	plain := filler(50)
	corpus := corpusOf(plain, plain, plain, md)
	out, isErr, _ := callTool(t, NewServer(), "byline_calibrate", map[string]any{"corpus": corpus})
	if isErr {
		t.Fatal(out)
	}
	want(t, out["left_out"], json.Number("1"))
	want(t, out["left_out_words"], json.Number(fmt.Sprint(len(strings.Fields(md)))))
	want(t, out["words"], json.Number(fmt.Sprint(3*len(strings.Fields(plain)))))
}

// TestCalibrate_HashHeadingNeedsASpace: "# Status" is a heading and is left
// out, but a Slack "#channel" or "#1234" at the start of a line is kept.
func TestCalibrate_HashHeadingNeedsASpace(t *testing.T) {
	setupHome(t)
	heading := "# Status\n" + filler(45)
	channel := "#general is down again, " + filler(45)
	plain := filler(50)
	out, isErr, _ := callTool(t, NewServer(), "byline_calibrate", map[string]any{"corpus": corpusOf(plain, plain, channel, heading)})
	if isErr {
		t.Fatal(out)
	}
	want(t, out["left_out"], json.Number("1"))
	want(t, out["left_out_words"], json.Number(fmt.Sprint(len(strings.Fields(heading)))))
}

// TestCalibrate_LeavesOutAMessageWithTwoDistinctCoreHits: a message that
// trips 2 distinct core phrase rules is left out of the corpus, even
// though neither phrase's rule is switched off by it (that message's
// hits never make it into would_switch_off at all).
func TestCalibrate_LeavesOutAMessageWithTwoDistinctCoreHits(t *testing.T) {
	setupHome(t)
	rA, rB := firstTwoCoreStringRules(t)
	bad := filler(30) + " " + rA.Strings[0] + " and " + rB.Strings[0] + " " + filler(10)
	plain := filler(50)
	corpus := corpusOf(plain, plain, plain, bad)
	out, isErr, _ := callTool(t, NewServer(), "byline_calibrate", map[string]any{"corpus": corpus})
	if isErr {
		t.Fatal(out)
	}
	want(t, out["left_out"], json.Number("1"))
	want(t, out["left_out_words"], json.Number(fmt.Sprint(len(strings.Fields(bad)))))
	want(t, out["words"], json.Number(fmt.Sprint(3*len(strings.Fields(plain)))))
	assertNoSwitchOff(t, out["would_switch_off"], rA.ID, rA.Strings[0])
	assertNoSwitchOff(t, out["would_switch_off"], rB.ID, rB.Strings[0])
}

// TestCalibrate_MessageWithOneCoreHitIsKept: a message that trips only 1
// core phrase rule is kept (it isn't "2 or more distinct", the leave-out
// threshold), so its words still count.
func TestCalibrate_MessageWithOneCoreHitIsKept(t *testing.T) {
	setupHome(t)
	r := firstCoreStringRule(t)
	msg := filler(45) + " " + r.Strings[0]
	plain := filler(50)
	corpus := corpusOf(msg, plain, plain)
	out, isErr, _ := callTool(t, NewServer(), "byline_calibrate", map[string]any{"corpus": corpus})
	if isErr {
		t.Fatal(out)
	}
	want(t, out["left_out"], json.Number("0"))
	want(t, out["words"], json.Number(fmt.Sprint(len(strings.Fields(msg))+2*len(strings.Fields(plain)))))
}

// TestCalibrate_SwitchesOffOnlyWhenPhraseIsInTwoKeptMessages: one hit
// anywhere used to be enough to switch a core phrase off. Now it takes 2
// distinct kept messages using it.
func TestCalibrate_SwitchesOffOnlyWhenPhraseIsInTwoKeptMessages(t *testing.T) {
	setupHome(t)
	r := firstCoreStringRule(t)
	s := NewServer()
	msg1 := filler(45) + " " + r.Strings[0]
	msg2 := filler(50)
	msg3 := filler(50)

	onlyOne, isErr, _ := callTool(t, s, "byline_calibrate", map[string]any{"corpus": corpusOf(msg1, msg2, msg3)})
	if isErr {
		t.Fatal(onlyOne)
	}
	assertNoSwitchOff(t, onlyOne["would_switch_off"], r.ID, r.Strings[0])

	msg2WithPhrase := msg2 + " " + r.Strings[0]
	twoNow, isErr, _ := callTool(t, s, "byline_calibrate", map[string]any{"corpus": corpusOf(msg1, msg2WithPhrase, msg3)})
	if isErr {
		t.Fatal(twoNow)
	}
	assertContainsSwitchOff(t, twoNow["would_switch_off"], r.ID, r.Strings[0])
}

// TestSetupSave_FloorCountsKeptWordsOnly: minSetupWords is checked against
// the kept words, not the raw corpus's word count. A corpus that only
// reaches minSetupWords because of a message that gets left out must
// still be refused.
func TestSetupSave_FloorCountsKeptWordsOnly(t *testing.T) {
	home := setupHome(t)
	plain := filler(50) // 3 of these qualify for the 3-of-40 rule, 150 words kept
	drafted := "- " + filler(5000)
	corpus := corpusOf(plain, plain, plain, drafted)
	if len(strings.Fields(messagesOnly(corpus))) < minSetupWords {
		t.Fatal("test corpus must reach minSetupWords before filtering, to prove the floor isn't checked against it")
	}
	before := snapshot(t, home)
	_, isErr, text := callTool(t, NewServer(), "byline_setup_save", map[string]any{"profile": "p", "exemplars": "e", "real_voice": corpus})
	if !isErr || !strings.Contains(text, fmt.Sprintf("found only %d words", 3*len(strings.Fields(plain)))) {
		t.Fatalf("got %q", text)
	}
	want(t, snapshot(t, home), before)
}

// TestSetupSave_ReportsLeftOutMessages: both byline_calibrate and
// byline_setup_save report left_out and left_out_words, with the same
// numbers for the same corpus, and the saved real-voice file holds only
// the kept messages, joined by "\n---\n".
func TestSetupSave_ReportsLeftOutMessages(t *testing.T) {
	setupHome(t)
	s := NewServer()
	md := "- Shipped the thing\n- Fixed a bug this week."
	plain := filler(50)
	corpus := corpusOf(plain, plain, plain, md)

	calibrated, isErr, _ := callTool(t, s, "byline_calibrate", map[string]any{"corpus": corpus})
	if isErr {
		t.Fatal(calibrated)
	}
	want(t, calibrated["left_out"], json.Number("1"))
	want(t, calibrated["left_out_words"], json.Number(fmt.Sprint(len(strings.Fields(md)))))

	saved, isErr, _ := callTool(t, s, "byline_setup_save", map[string]any{"profile": "p", "exemplars": "e", "accept_fewer_words": true, "real_voice": corpus})
	if isErr {
		t.Fatal(saved)
	}
	want(t, saved["left_out"], calibrated["left_out"])
	want(t, saved["left_out_words"], calibrated["left_out_words"])
	want(t, readFile(t, paths.RealVoicePath()), corpusOf(plain, plain, plain))
}
