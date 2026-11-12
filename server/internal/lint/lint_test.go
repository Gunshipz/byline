package lint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"byline/internal/rules"
)

// Measured 2026-11-10 with today's lint.mjs on the synthetic rules (K11 written with today's hard-coded name).
const draftA = "To be honest, we should delve into this.\nWe leverage it daily and the deleveraged plan is robust.\n" +
	"## This Is Title Case\n- **Owner**: Sam\nShipped it 🎉 today\nno meetings, no slides, no status\n" +
	"It is not about the money, but about the time we lose.\nRather than waiting for the review, I would rather ship it now.\n" +
	"The plan is simple: we ship the first version on Friday.\nIt ran long — again.\n\nThanks,\nSam\n"

func synthSet(t *testing.T) *rules.Set {
	c, err := rules.LoadCore(os.DirFS("../rules/testdata/synthetic"))
	if err != nil {
		t.Fatal(err)
	}
	s, _ := rules.Merge(c, nil)
	return s
}
func short(vs []Violation) (out []string) {
	for _, v := range vs {
		out = append(out, fmt.Sprintf("%d %s %s", v.Line, v.RuleID, v.Match))
	}
	return
}

func TestLint_SyntheticDraft(t *testing.T) {
	vs := Run(synthSet(t), draftA)
	want := []string{"1 G7 delve", "1 K1 To be honest", "2 G7 leverage", "3 G17 ## This Is Title Case",
		"4 G16 - **Owner**: Sam", "5 G18 Shipped it 🎉 today", "6 K13b no meetings, no slides, no status",
		"7 K5 not about the money, but about the time we lose", "8 K32 Rather than waiting for the review, I would rather",
		"9 K13a The plan is simple: we ship the first version on Friday.", "10 K11 Thanks,/Sam", "10 K4 —"}
	if !reflect.DeepEqual(short(vs), want) || vs[0].Family != 4 || vs[0].Name != "AI vocabulary" || vs[0].Fix != "f" ||
		vs[1].String != "to be honest" || vs[3].String != "" || vs[10].String != "Thanks,\nSam" {
		t.Fatal(short(vs))
	}
}

// I1: a banned phrase written with curly apostrophes/quotes must still
// hit, the way Node's old "lint.mjs --fix" step (silent curly-quote
// autofix, run before linting) made it do.
func TestLint_CurlyQuotesNormalizedBeforeMatching(t *testing.T) {
	core := &rules.Core{Rules: []rules.Rule{
		{ID: "T1", Family: 1, Name: "Contraction", Strings: []string{"don't worry about it"}, Fix: "f"},
	}}
	s, err := rules.Merge(core, nil)
	if err != nil {
		t.Fatal(err)
	}
	vs := Run(s, "Please don’t worry about it today.\n")
	if len(vs) != 1 || vs[0].Match != "don't worry about it" {
		t.Fatal(vs)
	}
}

// I5: a multi-word banned phrase must not match inside a longer word.
// testdata/lint/good_prose.txt is invented, ordinary prose that embeds the
// review's four examples ("by design"/"designers", "In addition"/
// "additional", "is the path"/"pathway", "full stop"/"stops") inside
// longer words; against the real core rules it must give 0 hits.
func TestLint_WordBoundaryAtPhraseEdges(t *testing.T) {
	core, err := rules.LoadCore(repoRulesFS(t))
	if err != nil {
		t.Fatal(err)
	}
	s, err := rules.Merge(core, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile("../../testdata/lint/good_prose.txt")
	if err != nil {
		t.Fatal(err)
	}
	if vs := Run(s, string(b)); len(vs) != 0 {
		t.Fatalf("%d false-positive hits on ordinary prose: %+v", len(vs), vs)
	}
}

func TestLint_SkipsMetaQuotesSignaturesHeadersAndInactive(t *testing.T) {
	s := synthSet(t)
	for in, want := range map[string][]string{
		"I flagged \"to be honest\" in your draft last week.\n":   nil,
		"We should delve into it.\n--\nSam\ndelve again\n":        {"1 G7 delve"},
		"We should delve into it.\n\nManager, Ops\ndelve again\n": {"1 G7 delve"},
		"To: sam@example.com\nSubject: delve\n\nWe delve.\n":      {"4 G7 delve"},
		"It is robust. Let's circle back. “Quoted.”\n":            nil,
	} {
		if got := short(Run(s, in)); !reflect.DeepEqual(got, want) {
			t.Errorf("%q: %v", in, got)
		}
	}
	s.SignatureLines = []string{"Jordan Sample"}
	if got := short(Run(s, "We should delve into it.\nJordan Sample\ndelve again\n")); !reflect.DeepEqual(got, []string{"1 G7 delve"}) {
		t.Fatal(got)
	}
}

// I4, wired into lint the same way factlint already does it (textutil.
// CutClosingSignature): with no signature_lines configured, a generic
// closing line ("Thanks,", "Best,", ...) also starts the signature
// region, so a non-owner's own invented sign-off isn't linted either.
// When signature_lines IS set, nothing changes (owner parity).
func TestLint_NoSignatureLinesCutsGenericClosing(t *testing.T) {
	s := synthSet(t)
	text := "We should delve into it.\n\nThanks,\ndelve again\nSenior Program Manager\n"
	if got := short(Run(s, text)); !reflect.DeepEqual(got, []string{"1 G7 delve"}) {
		t.Fatal(got)
	}

	s.SignatureLines = []string{"Jordan Sample"}
	if got := short(Run(s, text)); !reflect.DeepEqual(got, []string{"1 G7 delve", "4 G7 delve"}) {
		t.Fatal(got)
	}
}

// M4: a lone "--" line is a real sign-off only when 4 or fewer short
// lines (6 words or fewer each) follow it, not a mid-document divider.
func TestLint_DashIsSignatureOnlyWithShortTail(t *testing.T) {
	s := synthSet(t)
	for in, want := range map[string][]string{
		// More than 4 lines follow "--": not a sign-off, the later hit counts.
		"We should delve into it.\n--\n" +
			"Here is a longer section that keeps going on.\n" +
			"It has several more lines of real prose in it.\n" +
			"And it keeps talking about things at length.\n" +
			"Even more lines follow this one for good measure.\n" +
			"Still going after all of these lines right here.\n" +
			"We should delve into it again right here.\n": {"1 G7 delve", "8 G7 delve"},
		// 4 short lines follow "--": still a sign-off.
		"We should delve into it.\n--\nSam\nSenior Program Manager\nOperations\ndelve again\n": {"1 G7 delve"},
		// Only 2 lines follow "--", but one has more than 6 words: not a sign-off.
		"We should delve into it.\n--\n" +
			"This single line alone has more than six words in it.\n" +
			"delve again\n": {"1 G7 delve", "4 G7 delve"},
	} {
		if got := short(Run(s, in)); !reflect.DeepEqual(got, want) {
			t.Errorf("%q: got %v want %v", in, got, want)
		}
	}
}

// Node: 182 x's puts "delve" at UTF-16 index 220 (window still holds "I flagged"); 183 puts it at 221.
// M4: a meta-quote cue only silences a hit in its OWN sentence, not
// everything within a fixed character window around it. "instead of" on
// one sentence must not silence a real hit in the next, unrelated one.
func TestLint_MetaCueScopedToOwnSentence(t *testing.T) {
	s := synthSet(t)
	text := "We moved intake to the form instead of email last week.\n" +
		"To be honest, this is crucial, and we should delve into it ASAP.\n"
	if got, want := short(Run(s, text)), []string{"2 G7 delve", "2 K1 To be honest"}; !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
}

// M4 regression guard: a cue in the SAME sentence as the match still
// silences it (the behavior the window was trying to give).
func TestLint_MetaCueInSameSentenceStillSuppresses(t *testing.T) {
	s := synthSet(t)
	if got := short(Run(s, "I flagged delve already in your last draft.\n")); got != nil {
		t.Fatal(got)
	}
}

// M4: a match quoted in the text is silenced even with no cue phrase
// nearby — it's being quoted, not used.
func TestLint_QuotedMatchSuppressesWithNoCue(t *testing.T) {
	s := synthSet(t)
	if got := short(Run(s, "She calls everything \"to be honest\" in meetings.\n")); got != nil {
		t.Fatal(got)
	}
}

func TestLint_MatchCutAt70UTF16Units(t *testing.T) {
	// Filler is "x", not a curly quote (I1 now maps those to straight
	// quotes before this truncation runs); this test is about the UTF-16
	// cut length, not quote mapping.
	vs := Run(synthSet(t), "- **Owner**: "+strings.Repeat("x", 80)+"\n")
	if len(vs) != 1 || vs[0].Match != "- **Owner**: "+strings.Repeat("x", 57) {
		t.Fatal(vs)
	}
}

func TestLint_CRLFMatchesLF(t *testing.T) {
	s := synthSet(t)
	if !reflect.DeepEqual(Run(s, strings.ReplaceAll(draftA, "\n", "\r\n")), Run(s, draftA)) {
		t.Fatal("CRLF differs")
	}
}

func TestLintCLI(t *testing.T) {
	dir := t.TempDir()
	p, empty := filepath.Join(dir, "a.txt"), filepath.Join(dir, "e.txt")
	os.WriteFile(p, []byte(draftA), 0o600)
	os.WriteFile(empty, nil, 0o600)
	var o, e bytes.Buffer
	code := RunCLI([]string{p}, synthSet(t), &o, &e)
	lines := strings.Split(strings.TrimRight(o.String(), "\n"), "\n")
	if code != 1 || lines[0] != p+": 12 violations" || lines[1] != `  L1  [G7] fam4 AI vocabulary: "delve"` ||
		lines[len(lines)-1] != `  families: {"Abstraction dodging the plain statement":2,"Announcing instead of saying":1,"Engineered rhythm":8,"Manufactured work":1}` {
		t.Fatal(code, o.String())
	}
	o.Reset()
	var j struct {
		File  string
		Count int
		Hits  []Violation
	}
	RunCLI([]string{p, "--json"}, synthSet(t), &o, &e)
	if json.Unmarshal(o.Bytes(), &j) != nil || j.Count != 12 || j.File != p || j.Hits[0].RuleID != "G7" ||
		!strings.Contains(o.String(), `"rule_id": "G7"`) {
		t.Fatal(o.String())
	}
	o.Reset()
	if code := RunCLI([]string{empty}, synthSet(t), &o, &e); code != 0 || o.String() != empty+": 0 violations\n" {
		t.Fatal(code, o.String())
	}
	if code := RunCLI(nil, synthSet(t), &o, &e); code != 2 {
		t.Fatal(code)
	}
}

// M10: --json is accepted in any position, and an unknown flag is a
// usage error (exit 2 with a plain message), not silently ignored.
func TestLintCLI_FlagsInAnyPositionUnknownFlagRejected(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	os.WriteFile(p, []byte(draftA), 0o600)

	var o, e bytes.Buffer
	if code := RunCLI([]string{"--json", p}, synthSet(t), &o, &e); code != 1 {
		t.Fatal("--json before the file:", code, o.String(), e.String())
	}
	var j struct{ File string }
	if json.Unmarshal(o.Bytes(), &j) != nil || j.File != p {
		t.Fatal("--json before the file did not parse the draft:", o.String())
	}

	o.Reset()
	e.Reset()
	if code := RunCLI([]string{p, "--jsn"}, synthSet(t), &o, &e); code != 2 || e.String() == "" {
		t.Fatal("unknown flag:", code, o.String(), e.String())
	}
}

// repoRulesFS is os.DirFS of the repo's rules/ folder, found by walking up
// from the working directory.
func repoRulesFS(t testing.TB) fs.FS {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		candidate := filepath.Join(dir, "rules", "core-rules.json")
		if _, err := os.Stat(candidate); err == nil {
			return os.DirFS(filepath.Join(dir, "rules"))
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("rules/core-rules.json not found walking up from " + dir)
		}
		dir = parent
	}
}
