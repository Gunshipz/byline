package voice

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"byline/internal/rules"
)

// Measured 2026-11-10 with today's voicecheck.mjs.
const v1 = "Team, I'm going to give this time back today. My tools aren't working right now, so I can't get much done. " +
	"I'd rather we handle questions in Slack, so post there and I'll answer by Wed. You know the drill, and your notes " +
	"are always helpful, so keep them coming. We should be back on track next week.\n"

// none carries no voice_target.baseline: I3 (gate-p12-p5-review.md), every
// Score against it must come back NotMeasured, never scored against any
// shipped fallback.
var none = &rules.Set{}

// testBaseline is an invented baseline for tests, unrelated to any real
// person's calibrated numbers (those live outside the repo, in a
// person's own extras.json; see split_cli.go's voice_baseline).
var testBaseline = Baseline{
	"contraction_rate":   {Lo: 1, Hi: 5},
	"first_person_rate":  {Lo: 3, Hi: 9},
	"sent_len_mean":      {Lo: 10, Hi: 20},
	"sent_len_cv":        {Lo: 0.2, Hi: 0.5},
	"second_person_rate": {Lo: 1, Hi: 5},
	"comma_chain_rate":   {Lo: 0.1, Hi: 0.3},
}

// setWithBaseline returns a Set whose voice_target.baseline is exactly b,
// for tests of the measured path.
func setWithBaseline(t *testing.T, b Baseline) *rules.Set {
	t.Helper()
	baselineJSON, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	my, err := rules.ParseMyRules([]byte(`{"voice_target":{"baseline":` + string(baselineJSON) + `}}`))
	if err != nil {
		t.Fatal(err)
	}
	set, err := rules.Merge(&rules.Core{}, my)
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func TestMeasure_Synthetic(t *testing.T) {
	want := Metrics{Words: 58, Sentences: 5, ContractionRate: 8.62, FirstPersonRate: 8.62, WeRate: 3.45, SecondPersonRate: 3.45,
		SentLenMean: 11.6, SentLenCV: 0.26, ShortSentPct: 0, CommaChainRate: 0.2, SoChainRate: 5.17, FormalityFlags: 0}
	if got := measure(nil, v1); got != want {
		t.Fatalf("%+v", got)
	}
}

func TestScore_SyntheticStiffAndTarget(t *testing.T) {
	full := setWithBaseline(t, testBaseline)
	r := Score(full, v1)
	if r.NotMeasured || r.VoiceScore != 8.3 || r.InRange != 5 || r.Total != 6 || r.Penalty != 0 ||
		r.Detail[0] != (Detail{"contraction_rate", "contractions /100w", 8.62, 1, 5, false, "ABOVE"}) {
		t.Fatalf("%+v", r)
	}
	stiff := "In order to proceed, kindly review the aforementioned items at your earliest convenience. Please be advised that the team will follow up.\n"
	if r := Score(full, stiff); r.Metrics.FormalityFlags != 5 || r.VoiceScore != 0 {
		t.Fatalf("%+v", r)
	}

	// I3: a baseline naming only one metric scores only that metric -- the
	// other five are never filled in from any other source.
	core, _ := rules.LoadCore(os.DirFS("../rules/testdata/synthetic"))
	my, _ := rules.ParseMyRules([]byte(`{"voice_target":{"baseline":{"sent_len_mean":{"lo":10,"hi":20}}}}`))
	partial, err := rules.Merge(core, my)
	if err != nil {
		t.Fatal(err)
	}
	r = Score(partial, v1)
	if r.NotMeasured || r.VoiceScore != 10 || r.Total != 1 || r.InRange != 1 || len(r.Detail) != 1 ||
		r.Detail[0] != (Detail{"sent_len_mean", "mean words/sentence", 11.6, 10, 20, true, "ok"}) {
		t.Fatalf("%+v", r)
	}

	var keys []string
	for _, m := range ScoredMetrics {
		keys = append(keys, m.Key)
	}
	if !reflect.DeepEqual(keys, rules.BaselineMetrics) {
		t.Fatal("ScoredMetrics and rules.BaselineMetrics disagree")
	}
}

// I3 (gate-p12-p5-review.md): before setup there is no baseline, and the
// voice check must say it hasn't measured this person yet -- never fall
// back to any shipped baseline. Metrics are still measured.
func TestScore_NoBaselineIsNotMeasured(t *testing.T) {
	r := Score(none, v1)
	if !r.NotMeasured || r.VoiceScore != 0 || r.InRange != 0 || r.Total != 0 || r.Penalty != 0 || r.Detail != nil {
		t.Fatalf("%+v", r)
	}
	if r.Metrics.Words != 58 {
		t.Fatalf("metrics still measured: %+v", r.Metrics)
	}

	// An empty baseline object is the same as none: nothing to score.
	core, _ := rules.LoadCore(os.DirFS("../rules/testdata/synthetic"))
	my, _ := rules.ParseMyRules([]byte(`{"voice_target":{"baseline":{}}}`))
	empty, err := rules.Merge(core, my)
	if err != nil {
		t.Fatal(err)
	}
	if r := Score(empty, v1); !r.NotMeasured {
		t.Fatalf("empty baseline object must also be not measured: %+v", r)
	}
}

func TestMeasure_PlumbingEmptyNBSPAndSignature(t *testing.T) {
	m := measure(nil, "To: sam@example.com\nSubject: Update\n\n## Status\n- item one here\n| a | b |\nI think we are fine. You should see it soon.\n")
	if m.Words != 10 || m.Sentences != 2 || m.FirstPersonRate != 10 || m.SecondPersonRate != 10 || m.SentLenMean != 5 || m.ShortSentPct != 100 {
		t.Fatalf("plumbing %+v", m)
	}
	if m := measure(nil, ""); m.Words != 1 {
		t.Fatalf("empty text floors Words to 1: %+v", m)
	}
	if n := measure(nil, "word word word. Next one here now.\n"); n.Words != 7 {
		t.Fatalf("NBSP words %d", n.Words)
	}
	if measure([]string{"Jordan Sample"}, "I think so. You agree with me on this.\nJordan Sample\nI I I I I I me me my.\n") != measure(nil, "I think so. You agree with me on this.\n") {
		t.Fatal("signature tail counted")
	}

	// M6 (gate-p12-p5-review.md): an empty signature_lines entry must not
	// cut the draft at its very first newline, long before the real
	// signature. A sign-off block often has a blank line, which produces
	// exactly this entry.
	withBlank := []string{"Jordan Sample", ""}
	body := "I think so.\nYou agree with me on this too, and this line matters.\nJordan Sample\nI I I I I I me me my.\n"
	if got, want := measure(withBlank, body),
		measure(nil, "I think so.\nYou agree with me on this too, and this line matters.\n"); got != want {
		t.Fatalf("empty signature_lines entry: got %+v, want %+v", got, want)
	}
}

func TestMeasure_CRLFMatchesLF(t *testing.T) {
	if measure(nil, strings.ReplaceAll(v1, " So", "\r\n\r\nSo")) != measure(nil, strings.ReplaceAll(v1, " So", "\n\nSo")) {
		t.Fatal("CRLF differs")
	}
}

func TestVoiceCLI_Text(t *testing.T) {
	p := filepath.Join(t.TempDir(), "v1.txt")
	os.WriteFile(p, []byte(v1), 0o600)
	var o, e bytes.Buffer
	RunCLI([]string{p}, setWithBaseline(t, testBaseline), &o, &e)
	l := strings.Split(o.String(), "\n")
	if l[0] != p+"  VOICE 8.3/10  (5/6 in range, penalty 0)" || l[1] != "  ABOVE contractions /100w                   8.62   [1..5]" ||
		l[2] != "  ok   I/my /100w                           8.62   [3..9]" ||
		l[7] != "  info words=58 sents=5 short%=0 we/100w=3.45 so-chain=5.17 stiff=0" {
		t.Fatal(o.String())
	}
}

// I3: with no baseline, voicecheck prints one plain line and nothing else.
func TestVoiceCLI_NoBaselineIsOneLine(t *testing.T) {
	p := filepath.Join(t.TempDir(), "v1.txt")
	os.WriteFile(p, []byte(v1), 0o600)
	var o, e bytes.Buffer
	RunCLI([]string{p}, none, &o, &e)
	if o.String() != "Byline hasn't measured this person's voice yet; setup does that.\n" || e.String() != "" {
		t.Fatalf("stdout %q stderr %q", o.String(), e.String())
	}
}

// M10 (gate-p12-p5-review.md): an unknown flag must be rejected with a
// plain message, never silently read as the draft or corpus path.
func TestVoiceCLI_UnknownFlagRejected(t *testing.T) {
	var o, e bytes.Buffer
	code := RunCLI([]string{"--jsn", "somefile.txt"}, none, &o, &e)
	if code != 2 || o.String() != "" || !strings.Contains(e.String(), `unknown flag "--jsn"`) {
		t.Fatalf("code %d stdout %q stderr %q", code, o.String(), e.String())
	}
}

// M10: an unreadable draft or corpus file gets a plain stderr line, not a
// silent exit 2.
func TestVoiceCLI_UnreadableFileReportsPlainly(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.txt")

	var o, e bytes.Buffer
	if code := RunCLI([]string{missing}, none, &o, &e); code != 2 || o.String() != "" || !strings.Contains(e.String(), missing) {
		t.Fatalf("score: code %d stdout %q stderr %q", code, o.String(), e.String())
	}

	o.Reset()
	e.Reset()
	if code := RunCLI([]string{"--calibrate", missing}, none, &o, &e); code != 2 || o.String() != "" || !strings.Contains(e.String(), missing) {
		t.Fatalf("calibrate: code %d stdout %q stderr %q", code, o.String(), e.String())
	}
}

// I2 (gate-p12-p5-review.md): the old min/max formula is gone (see
// calibrate_test.go for the percentile-based replacement). Too few
// messages altogether still gives nil.
func TestCalibrate_TooFewMessagesIsNil(t *testing.T) {
	if Calibrate("just one message\n") != nil {
		t.Fatal("one message must give nil")
	}
}
