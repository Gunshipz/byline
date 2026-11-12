package rules

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"byline/internal/paths"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "byline-rules")
	if err != nil {
		panic(err)
	}
	os.Setenv("BYLINE_HOME", dir)
	os.Exit(m.Run())
}

func my(t *testing.T, js string) *MyRules {
	t.Helper()
	m, err := ParseMyRules([]byte(js))
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func ruleOf(s *Set, id string) Rule {
	for _, r := range s.Rules {
		if r.ID == id {
			return r
		}
	}
	return Rule{}
}
func merged(t *testing.T, js string) *Set {
	t.Helper()
	s, err := Merge(synth(t), my(t, js))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestMerge_AddRulesAppendAfterCore(t *testing.T) {
	a := merged(t, `{"format":1,"add_rules":[{"id":"U1","family":2,"name":"Mine","ban":"b","strings":["per my last"]}]}`).Active()
	if len(a) != 15 || a[14].ID != "U1" {
		t.Fatalf("%d active, last %q", len(a), a[len(a)-1].ID)
	}
}

func TestMerge_AddStringsAppendAndDedupe(t *testing.T) {
	s := merged(t, `{"add_strings":{"K1":["To Be Honest","frankly"]}}`)
	if got := ruleOf(s, "K1").Strings; !reflect.DeepEqual(got, []string{"to be honest", "honestly,", "frankly"}) {
		t.Fatal(got)
	}
	if got := s.Core()[0].Strings; !reflect.DeepEqual(got, []string{"to be honest", "honestly,"}) || len(s.Core()) != 15 || !s.Core()[14].Retired {
		t.Fatal("Core() must be the unmerged core", got)
	}
}

func TestMerge_SwitchOff(t *testing.T) {
	s := merged(t, `{"switch_off":{"K1":[{"string":"HONESTLY,","reason":"I write it"}]}}`)
	if got := ruleOf(s, "K1").Strings; !reflect.DeepEqual(got, []string{"to be honest"}) || len(s.Warnings) != 0 {
		t.Fatal(got, s.Warnings)
	}
	s = merged(t, `{"switch_off":{"K1":[{"string":"never there","reason":"r"}]}}`)
	if !reflect.DeepEqual(s.Warnings, []string{`switch_off: rule "K1" has no string "never there"`}) {
		t.Fatal(s.Warnings)
	}
	s = merged(t, `{"add_strings":{"K1":["frankly"]},"switch_off":{"K1":[{"string":"frankly","reason":"r"}]}}`)
	if got := ruleOf(s, "K1").Strings; !reflect.DeepEqual(got, []string{"to be honest", "honestly,"}) {
		t.Fatal(got)
	}
}

func TestMerge_ReplaceExamplesPerField(t *testing.T) {
	r := ruleOf(merged(t, `{"replace_examples":{"K1":{"bad":"My real bad line."}}}`), "K1")
	if r.Bad != "My real bad line." || r.Good != "y" {
		t.Fatal(r.Bad, r.Good)
	}
}

func TestMerge_VoiceTargetPerKey(t *testing.T) {
	s := merged(t, `{"voice_target":{"person":"I for me.","extra":"New."}}`)
	var keys []string
	for _, kv := range s.voice {
		keys = append(keys, kv.Key)
	}
	want := []string{"note", "register", "contractions", "person", "asks", "phrases_he_uses", "punctuation", "zeta_custom", "extra"}
	vt := s.VoiceTarget()
	if !reflect.DeepEqual(keys, want) || vt["person"] != "I for me." || vt["register"] != "Casual professional." ||
		vt["contractions"].(map[string]any)["rule"] != "Use them." {
		t.Fatal(keys, vt)
	}
}

func TestMerge_RetiredRuleKeepsOnlyPersonalStrings(t *testing.T) {
	for _, r := range merged(t, `{}`).Active() {
		if r.ID == "R1" {
			t.Fatal("retired R1 is active")
		}
	}
	s := merged(t, `{"add_strings":{"R1":["touch base"]}}`)
	r := ruleOf(s, "R1")
	if r.Retired || !reflect.DeepEqual(r.Strings, []string{"touch base"}) ||
		!reflect.DeepEqual(s.Warnings, []string{`rule "R1" is retired in core; only your own strings for it still apply`}) {
		t.Fatal(r, s.Warnings)
	}
}

func TestValidateMyRules_Errors(t *testing.T) {
	for js, want := range map[string]string{
		`{"add_rules":[{"id":"K1","family":1,"name":"N","ban":"b"}]}`:       `add_rules: rule "K1" is already a core rule`,
		`{"add_rules":[{"id":"U1","family":9,"name":"N","ban":"b"}]}`:       `rule "U1": family 9 does not exist`,
		`{"add_strings":{"K99":["x"]}}`:                                     `add_strings: no rule "K99"`,
		`{"switch_off":{"K1":[{"string":"to be honest","reason":""}]}}`:     `switch_off "K1" "to be honest": reason is required`,
		`{"replace_examples":{"K1":{}}}`:                                    `replace_examples "K1": bad or good is required`,
		`{"voice_target":{"baseline":{"sent_len_mean":{"lo":30,"hi":20}}}}`: `voice_target baseline "sent_len_mean": lo is above hi`,
		`{"signature_lines":["Jordan Sample",""]}`:                          `signature_lines: an entry is empty or blank`,
		`{"signature_lines":["Jordan Sample","   "]}`:                       `signature_lines: an entry is empty or blank`,
		`{"known_names":["Jordan",""]}`:                                     `known_names: an entry is empty or blank`,
	} {
		err := ValidateMyRules(synth(t), my(t, js))
		_, loadErr := load(os.DirFS("testdata/synthetic"), []byte(js))
		if err == nil || !strings.Contains(err.Error(), want) || loadErr == nil || !strings.Contains(loadErr.Error(), want) {
			t.Errorf("%s: want %q, got %v / %v", js, want, err, loadErr)
		}
	}

	// A duplicate add_rules id that isn't itself a core rule must be
	// reported only once, as "appears twice" -- not also as "is already
	// a core rule".
	dup := `{"add_rules":[{"id":"U1","family":1,"name":"N","ban":"b"},{"id":"U1","family":1,"name":"N","ban":"b"}]}`
	want := `add_rules: rule "U1" appears twice`
	err := ValidateMyRules(synth(t), my(t, dup))
	_, loadErr := load(os.DirFS("testdata/synthetic"), []byte(dup))
	if err == nil || err.Error() != want || loadErr == nil || loadErr.Error() != want {
		t.Fatalf("duplicate add_rules: got %v / %v, want exactly %q", err, loadErr, want)
	}
}

func TestParseMyRules_FormatsAndUnknownKeys(t *testing.T) {
	if m, err := ParseMyRules([]byte(`{"add_strings":{}}`)); err != nil || m.Format != 0 {
		t.Fatal(err)
	}
	if _, err := ParseMyRules([]byte(`{"format":1,"future_key":{"a":1}}`)); err != nil {
		t.Fatal(err)
	}
	_, err := ParseMyRules([]byte(`{"format":2}`))
	if err == nil || err.Error() != "my-rules.json format 2 is newer than this version of Byline understands (1); update Byline" {
		t.Fatal(err)
	}
	if _, err := ParseMyRules([]byte(`{`)); err == nil || !strings.HasPrefix(err.Error(), "my-rules.json: ") {
		t.Fatal(err)
	}
}

// M2 (gate-p12-p5-review.md): a decoding error is one plain sentence
// naming the file and, when the decoder knows it, the field -- never raw
// Go JSON error text like "cannot unmarshal array into Go struct field".
func TestParseMyRules_PlainErrors(t *testing.T) {
	_, err := ParseMyRules([]byte(`{"add_strings":[1,2]}`))
	if err == nil || err.Error() != `my-rules.json: field "add_strings" can't be a JSON array` {
		t.Fatalf("got %v", err)
	}
	if _, err := ParseMyRules([]byte(`not json`)); err == nil || err.Error() != "my-rules.json: isn't valid JSON" {
		t.Fatalf("got %v", err)
	}
}

// M2: a UTF-8 BOM (PowerShell 5.1's "-Encoding UTF8" writes one) must not
// make an otherwise valid my-rules.json unreadable.
func TestParseMyRules_StripsLeadingBOM(t *testing.T) {
	if _, err := ParseMyRules(append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"format":1}`)...)); err != nil {
		t.Fatalf("got %v", err)
	}
}

func TestLoad_NilPersonalIsCoreOnly(t *testing.T) {
	s, err := load(os.DirFS("testdata/synthetic"), nil)
	if err != nil || len(s.Active()) != 14 || s.CoreVersion != "0.9.0" {
		t.Fatal(err)
	}
}

// TestLoadPersonalFile_MissingIsCoreOnly covers the shared loader the CLI
// (loadSet) and every MCP tool (currentRules) now both call: a missing
// personal file is quietly core-only, with no warning.
func TestLoadPersonalFile_MissingIsCoreOnly(t *testing.T) {
	t.Setenv("BYLINE_HOME", t.TempDir())
	set, warning, err := LoadPersonalFile()
	if err != nil || set == nil || warning != "" {
		t.Fatal(set, warning, err)
	}
}

// TestLoadPersonalFile_BrokenFallsBackWithWarning: a my-rules.json that
// doesn't parse degrades to core rules only, with a plain one-line
// warning naming the file -- never a raw Go JSON error, and never a hard
// failure that would stop a checker outright (gate finding M2).
func TestLoadPersonalFile_BrokenFallsBackWithWarning(t *testing.T) {
	t.Setenv("BYLINE_HOME", t.TempDir())
	if err := os.WriteFile(paths.MyRulesPath(), []byte(`{"format": "not-a-number"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	set, warning, err := LoadPersonalFile()
	if err != nil || set == nil {
		t.Fatal(err, set)
	}
	if warning != "my-rules.json isn't valid; using core rules only" {
		t.Fatalf("warning = %q", warning)
	}
}

// TestLoadPersonalFile_StripsLeadingBOM: a UTF-8 BOM (PowerShell's
// "-Encoding UTF8" writes one) must not be treated as a broken file.
func TestLoadPersonalFile_StripsLeadingBOM(t *testing.T) {
	t.Setenv("BYLINE_HOME", t.TempDir())
	bom := append([]byte("\xef\xbb\xbf"), []byte(`{"format": 1, "known_names": ["Jordan"]}`)...)
	if err := os.WriteFile(paths.MyRulesPath(), bom, 0o600); err != nil {
		t.Fatal(err)
	}
	set, warning, err := LoadPersonalFile()
	if err != nil || set == nil || warning != "" {
		t.Fatal(set, warning, err)
	}
	if len(set.KnownNames) != 1 || set.KnownNames[0] != "Jordan" {
		t.Fatalf("BOM blocked the personal file from loading: %+v", set.KnownNames)
	}
}
