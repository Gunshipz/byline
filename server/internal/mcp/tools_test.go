package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"byline/internal/factlint"
	"byline/internal/lint"
	"byline/internal/paths"
	"byline/internal/version"
	"byline/internal/voice"
)

func TestStatus_FreshFolder(t *testing.T) {
	home := setupHome(t)
	out, isErr, _ := callTool(t, NewServer(), "byline_status", map[string]any{})
	if isErr {
		t.Fatal()
	}
	want(t, out["version"], version.Version())
	want(t, out["voice_folder"], home)
	want(t, out["setup_done"], false)
	want(t, out["files"], map[string]any{"brief": true, "profile": true, "exemplars": true, "real_voice": false})
	want(t, out["problems"], []any{})
	want(t, out["update_notice"], nil)
	if !strings.HasPrefix(out["brief_stamp"].(string), "<!-- byline "+version.Version()+" |") {
		t.Fatal(out["brief_stamp"])
	}
}

func TestStatus_ReportsUnreadablePersonalRules(t *testing.T) {
	setupHome(t)
	writeFile(t, paths.MyRulesPath(), "{not json")
	out, _, _ := callTool(t, NewServer(), "byline_status", map[string]any{})
	assertOneContains(t, toStrings(out["problems"]), "can't read your personal rules")
}

// Gate finding M2: a newer-format settings.json must be reported as a
// settings problem, never with the personal-rules sentence (which names
// the wrong file and the wrong fix).
func TestStatus_NewerFormatSettingsIsReportedAsASettingsProblem(t *testing.T) {
	setupHome(t)
	writeFile(t, paths.SettingsPath(), `{"format":2}`)
	out, _, _ := callTool(t, NewServer(), "byline_status", map[string]any{})
	problems := toStrings(out["problems"])
	assertOneContains(t, problems, "settings")
	for _, p := range problems {
		if strings.Contains(p, "personal rules") {
			t.Fatalf("settings problem used the personal-rules sentence: %q", p)
		}
	}
}

// Gate finding M3: status must re-check the files on each call, not keep
// replaying the server's start-time error forever once the person fixes
// the file.
func TestStatus_StopsReportingAProblemOnceTheFileIsFixed(t *testing.T) {
	setupHome(t)
	s := NewServer()
	writeFile(t, paths.MyRulesPath(), "{not json")
	first, _, _ := callTool(t, s, "byline_status", map[string]any{}) // triggers start, capturing the broken file
	assertOneContains(t, toStrings(first["problems"]), "can't read your personal rules")

	writeFile(t, paths.MyRulesPath(), `{"format":1}`) // fixed; same server, start never re-runs
	second, _, _ := callTool(t, s, "byline_status", map[string]any{})
	want(t, second["problems"], []any{})
}

// TestLintTool_BrokenPersonalRulesFallsBackWithWarning covers the gate
// finding that mcp's currentRules duplicated the CLI's loadSet: a broken
// my-rules.json must not stop a checker tool outright, the way it
// already doesn't stop the CLI. The tool puts the one plain warning in
// its own result, never on stderr (that belongs to the MCP transport).
func TestLintTool_BrokenPersonalRulesFallsBackWithWarning(t *testing.T) {
	setupHome(t)
	writeFile(t, paths.MyRulesPath(), "{not json")
	out, isErr, _ := callTool(t, NewServer(), "byline_lint", map[string]any{"text": "Plain words."})
	if isErr {
		t.Fatal(out)
	}
	want(t, out["clean"], true)
	want(t, out["warning"], "my-rules.json isn't valid; using core rules only")
}

func TestBrief_ReturnsTheThreeFiles(t *testing.T) {
	setupHome(t)
	out, _, _ := callTool(t, NewServer(), "byline_brief", map[string]any{})
	want(t, out["brief"], readFile(t, paths.BriefPath()))
	want(t, out["profile"], readFile(t, paths.ProfilePath()))
	want(t, out["exemplars"], readFile(t, paths.ExemplarsPath()))
	// Gate finding M6: byline_brief's result is missing a paths object.
	want(t, out["paths"], map[string]any{
		"brief":     paths.BriefPath(),
		"profile":   paths.ProfilePath(),
		"exemplars": paths.ExemplarsPath(),
	})
}

func TestBrief_MissingFileIsAPlainError(t *testing.T) {
	setupHome(t)
	s := NewServer()
	callTool(t, s, "byline_status", map[string]any{}) // start has run; deleting now is a mid-session loss
	os.Remove(paths.BriefPath())
	_, isErr, text := callTool(t, s, "byline_brief", map[string]any{})
	if !isErr || text != "Byline's brief file is missing from "+paths.BriefPath()+". Quit and reopen the app so Byline can rebuild it." {
		t.Fatalf("%q", text)
	}
}

func TestLintTool_MatchesLintPackage(t *testing.T) {
	setupHome(t)
	r := firstCoreStringRule(t) // first non-retired core rule with strings
	text := "We will " + r.Strings[0] + " today."
	out, _, _ := callTool(t, NewServer(), "byline_lint", map[string]any{"text": text})
	set, _, _ := currentRules()
	want(t, out["clean"], false)
	want(t, out["violations"], jsonRoundTrip(t, lint.Run(set, text)))
	clean, _, _ := callTool(t, NewServer(), "byline_lint", map[string]any{"text": "Plain words."})
	want(t, clean["clean"], true)
	want(t, clean["violations"], []any{})
}

func TestFactlintTool_MatchesFactlintPackage(t *testing.T) {
	setupHome(t)
	d, s := "Costs rose 40% in Q3.", "Costs rose 4% in Q3."
	out, _, _ := callTool(t, NewServer(), "byline_factlint", map[string]any{"draft": d, "source": s})
	set, _, _ := currentRules()
	want(t, out["flags"], jsonRoundTrip(t, factlint.Run(set, d, s)))
	want(t, out["clean"], len(factlint.Run(set, d, s)) == 0)
}

func TestDraftcheckTool_ReceiptOnlyOnPass(t *testing.T) {
	setupHome(t)
	s := NewServer()
	out, _, _ := callTool(t, s, "byline_draftcheck", map[string]any{"draft": testdata(t, "draftcheck/pass_draft.txt"), "spec": jsonObj(t, testdata(t, "draftcheck/fail_spec.json"))})
	want(t, out["pass"], false)
	want(t, out["receipt_written"], false)
	want(t, len(glob(t, filepath.Join(paths.ReceiptsDir(), "*"))), 0)
	out, _, _ = callTool(t, s, "byline_draftcheck", map[string]any{"draft": testdata(t, "draftcheck/pass_draft.txt"), "spec": jsonObj(t, testdata(t, "draftcheck/pass_spec.json"))})
	want(t, out["pass"], true)
	want(t, out["receipt_written"], true)
	want(t, len(glob(t, filepath.Join(paths.ReceiptsDir(), "*"))), 1)
}

func TestVoicecheckTool_MatchesVoicePackage(t *testing.T) {
	setupHome(t)
	writeFile(t, paths.MyRulesPath(), `{"format":1,"voice_target":{"baseline":{
		"contraction_rate":{"lo":0,"hi":6.0},
		"first_person_rate":{"lo":2.9,"hi":10.5},
		"sent_len_mean":{"lo":12,"hi":24},
		"sent_len_cv":{"lo":0.35,"hi":0.85},
		"second_person_rate":{"lo":0.9,"hi":7.0},
		"comma_chain_rate":{"lo":0,"hi":0.45}
	}}}`)
	set, _, _ := currentRules()
	text := "Thanks. I'll send the numbers by Friday."
	out, _, _ := callTool(t, NewServer(), "byline_voicecheck", map[string]any{"text": text})
	want(t, out, jsonRoundTrip(t, voice.Score(set, text)))
}

// TestVoicecheckTool_NotMeasuredIsPlainSentence covers I3 for the MCP
// voice tool: a templates-only voice folder (setup never run, so
// voice_target carries no baseline) must come back as one plain sentence,
// never a score of zeros.
func TestVoicecheckTool_NotMeasuredIsPlainSentence(t *testing.T) {
	setupHome(t) // templates-only: my-rules.json's voice_target is {}
	set, _, _ := currentRules()
	if !voice.Score(set, "Thanks. I'll send the numbers by Friday.").NotMeasured {
		t.Fatal("test setup assumption failed: templates-only should be NotMeasured")
	}
	out, isErr, _ := callTool(t, NewServer(), "byline_voicecheck", map[string]any{"text": "Thanks. I'll send the numbers by Friday."})
	if isErr {
		t.Fatal(out)
	}
	want(t, out["not_measured"], true)
	want(t, out["message"], "Byline hasn't measured this person's voice yet; setup does that.")
	if _, has := out["voice_score"]; has {
		t.Fatalf("zeros presented as a score: %v", out)
	}
}
