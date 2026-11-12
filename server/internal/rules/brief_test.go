package rules

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"byline/internal/paths"
)

var fixed = time.Date(2026, 11, 10, 21, 4, 5, 0, time.UTC)
var synthFS = os.DirFS("testdata/synthetic")

func synthSet(t *testing.T) *Set { s, _ := Merge(synth(t), nil); return s }

func TestBrief_MatchesOldGeneratorOnSynthetic(t *testing.T) {
	lines := strings.SplitN(Brief(synthSet(t), "", "", "1.2.3", fixed), "\n", 3)
	golden, _ := os.ReadFile("testdata/synthetic/brief.golden")
	if lines[1] != "# Writing rules: composition brief" || lines[2] != string(golden) {
		t.Fatalf("title %q\nbody:\n%s", lines[1], lines[2])
	}
}

func TestBrief_StampLine(t *testing.T) {
	if got := strings.SplitN(Brief(synthSet(t), "", "", "1.2.3", fixed), "\n", 2)[0]; got !=
		"<!-- byline 1.2.3 | core rules 0.9.0 | personal rules saved never | generated 2026-11-10T21:04:05Z -->" {
		t.Fatal(got)
	}
}

func TestBrief_StanceAppendedVerbatim(t *testing.T) {
	got := Brief(synthSet(t), "# Core stance\n\nS1. Propose.\n\n", "## Mine\n\nM1. Mine.\n", "1.2.3", fixed)
	if !strings.HasSuffix(got, "**Zeta custom:** Custom key last.\n\n# Core stance\n\nS1. Propose.\n\n## Mine\n\nM1. Mine.\n") {
		t.Fatal(got)
	}
}

func TestBrief_NeverPrintsBaselineOrNote(t *testing.T) {
	s := synthSet(t)
	s.voice = append(s.voice, KV{"baseline", json.RawMessage(`{"sent_len_mean":{"lo":10,"hi":20}}`)})
	if got := Brief(s, "", "", "1.2.3", fixed); strings.Contains(got, "Baseline") || strings.Contains(got, "**Note:**") {
		t.Fatal(got)
	}
}

// Lead decision 2026-11-10: a rule with no strings prints "name: test"; a rule with strings keeps its bare name.
func TestBrief_RuleTestAndEmptyVoiceValues(t *testing.T) {
	s := synthSet(t)
	for i := range s.Rules {
		switch s.Rules[i].ID {
		case "K29":
			s.Rules[i].Test = "Does it explain the idiom away?"
		case "K40":
			s.Rules[i].Test = "not printed, K40 has strings"
		}
	}
	s.voice = append(s.voice, KV{"empty_list", json.RawMessage(`[]`)}, KV{"empty_text", json.RawMessage(`""`)})
	got := Brief(s, "", "", "1.2.3", fixed)
	if !strings.Contains(got, "\nAlso banned: idiom flattened into its explanation: Does it explain the idiom away?; the picture instead of the work.\n") ||
		strings.Contains(got, "Empty list") || strings.Contains(got, "Empty text") {
		t.Fatal(got)
	}
}

// M1: an untouched my-stance.md (the template's own instructions, with no
// "## " heading) must add nothing to the brief.
func TestReadStanceFile_NoHeadingAddsNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "my-stance.md")
	os.WriteFile(path, []byte("# My stance rules\n\nYour own rules about positions, terms and relationships go here.\n"), 0o600)
	got, err := readStanceFile(path)
	if err != nil || got != "" {
		t.Fatalf("got %q, err %v, want \"\", nil", got, err)
	}
}

// M1: a stance file the person actually filled in (a "## " heading) is
// still carried verbatim.
func TestReadStanceFile_KeepsContentWithHeading(t *testing.T) {
	path := filepath.Join(t.TempDir(), "my-stance.md")
	content := "## S1. Mine\n\n- BAD: x\n- GOOD: y\n"
	os.WriteFile(path, []byte(content), 0o600)
	got, err := readStanceFile(path)
	if err != nil || got != content {
		t.Fatalf("got %q, err %v, want %q, nil", got, err, content)
	}
}

// M7 (gate-p12-p5-review.md): a rule test that itself ends in a period
// must not double it up against the "Also banned: ...." sentence period
// brief.go adds.
func TestBrief_RuleTestTrailingPeriodNotDoubled(t *testing.T) {
	s := synthSet(t)
	for i := range s.Rules {
		if s.Rules[i].ID == "K29" {
			s.Rules[i].Test = "A sentence that restates the one before it."
		}
	}
	got := Brief(s, "", "", "1.2.3", fixed)
	if !strings.Contains(got, "idiom flattened into its explanation: A sentence that restates the one before it;") {
		t.Fatal(got)
	}
	if strings.Contains(got, "before it..") {
		t.Fatal("doubled period: " + got)
	}
}

func TestGenerate_WritesStampAndReturnsPath(t *testing.T) {
	t.Setenv("BYLINE_HOME", t.TempDir())
	p, err := generate(synthFS, "1.2.3", fixed)
	b, _ := os.ReadFile(paths.BriefPath())
	if err != nil || p != paths.BriefPath() || !strings.HasPrefix(string(b), "<!-- byline 1.2.3 | core rules 0.9.0 | personal rules saved never |") {
		t.Fatal(err, p)
	}
	os.MkdirAll(paths.StateDir(), 0o700)
	os.WriteFile(paths.RulesSavedPath(), []byte("2026-11-09T12:00:00Z\n"), 0o600)
	generate(synthFS, "1.2.3", fixed)
	b, _ = os.ReadFile(paths.BriefPath())
	if !strings.Contains(string(b), "| personal rules saved 2026-11-09T12:00:00Z |") {
		t.Fatal(string(b)[:120])
	}
}

func TestGenerate_InvalidPersonalKeepsExistingBrief(t *testing.T) {
	t.Setenv("BYLINE_HOME", t.TempDir())
	os.WriteFile(paths.BriefPath(), []byte("old brief\n"), 0o600)
	os.WriteFile(paths.MyRulesPath(), []byte("{"), 0o600)
	p, err := generate(synthFS, "1.2.3", fixed)
	b, _ := os.ReadFile(paths.BriefPath())
	if err == nil || p != "" || string(b) != "old brief\n" {
		t.Fatal(err, p, string(b))
	}
}

func TestGenerate_InvalidPersonalWithNoBriefWritesCoreOnly(t *testing.T) {
	t.Setenv("BYLINE_HOME", t.TempDir())
	os.WriteFile(paths.MyRulesPath(), []byte("{"), 0o600)
	p, err := generate(synthFS, "1.2.3", fixed)
	b, _ := os.ReadFile(paths.BriefPath())
	if err == nil || !strings.Contains(err.Error(), "personal rules not applied") || p != paths.BriefPath() ||
		!strings.Contains(string(b), `Banned: "to be honest", "honestly,".`) {
		t.Fatal(err, p)
	}

	// M2 (gate-p12-p5-review.md): a broken my-rules.json's error, as
	// Generate's caller would print it to stderr, is one plain sentence --
	// never raw Go JSON error text like "cannot unmarshal array into Go
	// struct field".
	if strings.Contains(err.Error(), "cannot unmarshal") || strings.Contains(err.Error(), "looking for beginning of value") {
		t.Fatalf("leaked raw Go JSON error text: %v", err)
	}
	if !strings.Contains(err.Error(), "my-rules.json: isn't valid JSON") {
		t.Fatalf("not a plain sentence naming the file: %v", err)
	}
}

// Fix round 1 (2026-11-10): VOICE_ORDER has 15 keys (generate.mjs:94-96), not
// 4. These four are deliberately out of VOICE_ORDER's relative order, plus
// one key VOICE_ORDER doesn't know about, so a 4-key voiceOrder reproduces
// file order unchanged instead of reordering them.
func TestBrief_FullVoiceOrderAndUnknownKeyAppended(t *testing.T) {
	s := synthSet(t)
	s.voice = Object{
		{Key: "punctuation", Value: json.RawMessage(`{"em_dash":"never"}`)},
		{Key: "phrases_he_uses", Value: json.RawMessage(`["Go"]`)},
		{Key: "zeta_custom", Value: json.RawMessage(`"Z."`)},
		{Key: "asks", Value: json.RawMessage(`"Ride it."`)},
	}
	got := Brief(s, "", "", "1.2.3", fixed)
	want := "**Asks:** Ride it.\n**Phrases this person uses:** Go.\n**Punctuation:** em_dash: never.\n**Zeta custom:** Z.\n"
	if !strings.Contains(got, want) {
		t.Fatalf("got:\n%s\nwant substring:\n%s", got, want)
	}
}

// M7 (gate-p12-p5-review.md): a voice_target key starting with a
// non-ASCII letter must get a valid, properly upper-cased label, not an
// invalid UTF-8 byte from upper-casing only the key's first byte.
func TestBrief_NonASCIIVoiceKeyLabel(t *testing.T) {
	s := synthSet(t)
	s.voice = append(s.voice, KV{"élan_rule", json.RawMessage(`"Keep it."`)})
	got := Brief(s, "", "", "1.2.3", fixed)
	if !strings.Contains(got, "**Élan rule:** Keep it.\n") {
		t.Fatalf("got:\n%s\nwant \"**Élan rule:** Keep it.\\n\"", got)
	}
}

func TestLoad_EmbeddedCore(t *testing.T) {
	if s, err := Load(nil); err != nil || len(s.Families) != 6 {
		t.Fatal(err)
	}
}
