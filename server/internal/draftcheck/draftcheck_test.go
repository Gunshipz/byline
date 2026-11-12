package draftcheck

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"byline/internal/paths"
	"byline/internal/rules"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "byline-draftcheck")
	if err != nil {
		panic(err)
	}
	os.Setenv("BYLINE_HOME", dir)
	os.Exit(m.Run())
}

func caseByName(name string) SelftestCase {
	for _, c := range SelftestCases() {
		if c.Name == name {
			return c
		}
	}
	panic(name)
}
func specJSON(c SelftestCase) []byte { b, _ := json.Marshal(c.Spec); return b }
func readReceipt(t *testing.T, p string) (Receipt, string) {
	b, err := os.ReadFile(p)
	var r Receipt
	if err != nil || json.Unmarshal(b, &r) != nil {
		t.Fatal(p, err)
	}
	return r, string(b)
}

func TestContentWords(t *testing.T) {
	got := ContentWords("The budget sheet Priya revised is already finalized, and I'd say it's 12,000 dollars under.")
	if !reflect.DeepEqual(got, []string{"budget", "sheet", "priya", "revised", "already", "finalized", "i'd", "say", "it's", "000", "dollars"}) {
		t.Fatal(got)
	}
}

func TestSelftest_ElevenCasesPass(t *testing.T) {
	var b bytes.Buffer
	if code := Selftest(&b); code != 0 || !strings.HasSuffix(b.String(), "\nselftest: 11 passed, 0 failed\n\n") || len(SelftestCases()) != 11 {
		t.Fatal(b.String())
	}
}

// Values from today's draftcheck.mjs on the same fixtures, 2026-11-10.
func TestCheck_CleanAndRevived(t *testing.T) {
	c := caseByName("clean draft passes")
	r := check(c.Draft, c.Spec)
	names := []string{"cut content did not return", "within word budget (350)", "reader questions declared (2-4)",
		"sections map 1:1 to reader questions", "no persuasion architecture (intent: inform)"}
	for i, ch := range r.Checks {
		if ch.Name != names[i] || !ch.Pass {
			t.Fatal(i, ch)
		}
	}
	if !r.Pass || r.Words != 64 || r.Checks[3].Detail[0] != "3 section(s), all declared: Why it's realistic / Planning time / What it costs the team" {
		t.Fatal(r.Words, r.Checks[3].Detail)
	}
	rv := caseByName("catches reworded killed content")
	k := check(rv.Draft, rv.Spec).Checks[0]
	if k.Pass || k.Detail[0] != `para 7 revives "the venue discount Priya already locked in for the team offsite" (matched venue, discount, priya, already, locked, team, offsite)`+
		"\n      > Some of it we get for nothing. The venue discount Priya already locked in for the team offsite is already conf" {
		t.Fatal(k.Detail)
	}
}

func TestCheck_SpecEdges(t *testing.T) {
	c := caseByName("clean draft passes")
	s := c.Spec
	s.SectionMap = nil
	if d := check(c.Draft, s).Checks[3].Detail[0]; d != "spec has no sectionMap; declare which question each section answers, or set it to {} for a short unsectioned message" {
		t.Fatal(d)
	}
	s = c.Spec
	s.Budget = 0
	if ch := check(c.Draft, s).Checks[1]; ch.Name != "within word budget (Infinity)" || !ch.Pass {
		t.Fatal(ch)
	}
}

// Review fix round 1: Node's answered Set matches exact sectionMap values,
// so a non-integer index like 2.5 answers no declared question (1, 2 or
// 3), not question 2 by truncation.
func TestCheck_SectionMapNonIntegerIndexNotTruncated(t *testing.T) {
	spec := Spec{
		Questions:  []string{"q1", "q2", "q3"},
		SectionMap: &rules.Object{{Key: "Effort", Value: json.RawMessage("2.5")}},
	}
	text := "**Effort**\n\nSome body text here that is long enough to read as a paragraph.\n"
	got := check(text, spec).Checks[3].Detail
	want := []string{
		`question 1 ("q1") has no section answering it`,
		`question 2 ("q2") has no section answering it`,
		`question 3 ("q3") has no section answering it`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
}

func TestRun_WritesReceiptOnPassOnly(t *testing.T) {
	t.Setenv("BYLINE_HOME", t.TempDir())
	c := caseByName("clean draft passes")
	r, err := Run(c.Draft, specJSON(c))
	rec, raw := readReceipt(t, r.Receipt)
	if err != nil || !r.Pass || filepath.Dir(r.Receipt) != paths.ReceiptsDir() || !reflect.DeepEqual(rec.Words, ContentWords(r.Text)) ||
		rec.Draft != "" || !regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{3}Z$`).MatchString(rec.At) || !strings.HasPrefix(raw, `{"draft":`) {
		t.Fatal(err, raw)
	}
	fat := caseByName("catches over budget")
	if r, err := Run(fat.Draft, specJSON(fat)); err != nil || r.Pass || r.Receipt != "" {
		t.Fatal(err, r.Receipt)
	}
	if files, _ := os.ReadDir(paths.ReceiptsDir()); len(files) != 1 {
		t.Fatal(len(files))
	}
	if _, err := Run(c.Draft, []byte("{")); err == nil {
		t.Fatal("bad spec accepted")
	}
}

// Review Focus 3 in the master plan.
func TestDraftcheck_CRLFDraftMatchesLF(t *testing.T) {
	t.Setenv("BYLINE_HOME", t.TempDir())
	c := caseByName("clean draft passes")
	lf, err1 := Run(c.Draft, specJSON(c))
	crlf, err2 := Run(strings.ReplaceAll(c.Draft, "\n", "\r\n"), specJSON(c))
	if err1 != nil || err2 != nil || !lf.Pass || !crlf.Pass {
		t.Fatal(err1, err2)
	}
	r1, _ := readReceipt(t, lf.Receipt)
	r2, _ := readReceipt(t, crlf.Receipt)
	lf.Receipt, crlf.Receipt = "", ""
	if !reflect.DeepEqual(lf, crlf) || !reflect.DeepEqual(r1.Words, r2.Words) || len(r1.Words) == 0 {
		t.Fatalf("CRLF differs:\n%+v\n%+v", lf, crlf)
	}
}

// Gate finding M2: a UTF-8 BOM in spec.json (PowerShell's -Encoding UTF8
// writes one) must not be treated as a broken file.
func TestDraftcheckCLI_SpecWithLeadingBOMStillWorks(t *testing.T) {
	t.Setenv("BYLINE_HOME", t.TempDir())
	dir := t.TempDir()
	draft := filepath.Join(dir, "draft.txt")
	spec := filepath.Join(dir, "spec.json")
	os.WriteFile(draft, []byte("Just a short note.\n"), 0o600)
	bom := append([]byte("\xef\xbb\xbf"), []byte(`{"reader":"Jordan","intent":"inform","questions":[],"budget":350,"sectionMap":{},"killList":[]}`)...)
	os.WriteFile(spec, bom, 0o600)

	var o, e bytes.Buffer
	if code := RunCLI([]string{draft, spec}, &o, &e); code == 2 {
		t.Fatalf("exit 2 on a spec with a leading BOM: %s / %s", o.String(), e.String())
	}
}

// Gate finding M3: Node's loosely typed JS accepted a numeric string for
// "budget" and treated an empty array "sectionMap" like {}; Go's strict
// struct unmarshal rejected both. A real type error on either field, or any
// other field, now gets a plain message naming the field instead of Go's
// raw JSON error.
func TestSpecUnmarshalJSON_LooseTypes(t *testing.T) {
	var s Spec
	if err := json.Unmarshal([]byte(`{"budget":"350","questions":[],"killList":[]}`), &s); err != nil || s.Budget != 350 {
		t.Fatalf("numeric string budget: %v %v", err, s.Budget)
	}

	s = Spec{}
	if err := json.Unmarshal([]byte(`{"sectionMap":[],"questions":[],"killList":[]}`), &s); err != nil || s.SectionMap == nil || len(*s.SectionMap) != 0 {
		t.Fatalf("empty-array sectionMap: %v %v", err, s.SectionMap)
	}

	s = Spec{}
	err := json.Unmarshal([]byte(`{"budget":"not a number","questions":[],"killList":[]}`), &s)
	if err == nil || !strings.Contains(err.Error(), "budget") || strings.Contains(err.Error(), "json:") || strings.Contains(err.Error(), "cannot unmarshal") {
		t.Fatalf("bad budget string should give a plain message naming the field: %v", err)
	}

	s = Spec{}
	err = json.Unmarshal([]byte(`{"sectionMap":[1,2],"questions":[],"killList":[]}`), &s)
	if err == nil || !strings.Contains(err.Error(), "sectionMap") || strings.Contains(err.Error(), "json:") {
		t.Fatalf("non-empty array sectionMap should give a plain message naming the field: %v", err)
	}

	s = Spec{}
	err = json.Unmarshal([]byte(`{"reader":5,"questions":[],"killList":[]}`), &s)
	if err == nil || !strings.Contains(err.Error(), "reader") || strings.Contains(err.Error(), "json:") || strings.Contains(err.Error(), "cannot unmarshal") {
		t.Fatalf("a bad reader type should give a plain message naming the field, not a raw Go error: %v", err)
	}
}

// Same end-to-end case as the review's own proof: Node accepted both, Go
// exited 2.
func TestDraftcheckCLI_LooseSpecTypesMatchNode(t *testing.T) {
	t.Setenv("BYLINE_HOME", t.TempDir())
	dir := t.TempDir()
	draft := filepath.Join(dir, "draft.txt")
	os.WriteFile(draft, []byte("Just a short note.\n"), 0o600)

	budgetSpec := filepath.Join(dir, "budget.json")
	os.WriteFile(budgetSpec, []byte(`{"reader":"Jordan","intent":"inform","questions":[],"budget":"350","sectionMap":{},"killList":[]}`), 0o600)
	var o, e bytes.Buffer
	if code := RunCLI([]string{draft, budgetSpec}, &o, &e); code == 2 {
		t.Fatalf("exit 2 on a numeric-string budget: %s / %s", o.String(), e.String())
	}

	arraySpec := filepath.Join(dir, "array.json")
	os.WriteFile(arraySpec, []byte(`{"reader":"Jordan","intent":"inform","questions":[],"budget":350,"sectionMap":[],"killList":[]}`), 0o600)
	o.Reset()
	e.Reset()
	if code := RunCLI([]string{draft, arraySpec}, &o, &e); code == 2 {
		t.Fatalf("exit 2 on sectionMap:[]: %s / %s", o.String(), e.String())
	}
}

// Gate finding M10: "--selftest" only triggers selftest mode when it's the
// sole argument; with a trailing argument it's treated as a draft path
// (and fails plainly, since no file named "--selftest" exists), not run as
// selftest.
func TestDraftcheckCLI_SelftestOnlyWhenSoleArg(t *testing.T) {
	t.Setenv("BYLINE_HOME", t.TempDir())
	var o, e bytes.Buffer
	code := RunCLI([]string{"--selftest", "x"}, &o, &e)
	if strings.Contains(o.String(), "selftest:") {
		t.Fatalf("treated --selftest as selftest mode with a trailing arg: %s", o.String())
	}
	if code != 2 {
		t.Fatalf("exit %d, want 2 (no file named --selftest): %s / %s", code, o.String(), e.String())
	}
}

func TestDraftcheckCLI(t *testing.T) {
	t.Setenv("BYLINE_HOME", t.TempDir())
	d, s := "../../testdata/draftcheck/pass_draft.txt", "../../testdata/draftcheck/pass_spec.json"
	var o, e bytes.Buffer
	if code := RunCLI([]string{d, s}, &o, &e); code != 0 || !regexp.MustCompile(`\nall checks passed — receipt \d+-\d{6}\.json\n\n$`).MatchString(o.String()) {
		t.Fatal(code, o.String())
	}
	files, _ := os.ReadDir(paths.ReceiptsDir())
	if rec, _ := readReceipt(t, filepath.Join(paths.ReceiptsDir(), files[0].Name())); rec.Draft != d || rec.Spec != s {
		t.Fatal(rec)
	}
	o.Reset()
	if code := RunCLI([]string{d, "../../testdata/draftcheck/fail_spec.json"}, &o, &e); code != 1 ||
		!strings.HasSuffix(o.String(), "\n1 FAILED — do not show this to the user yet\n\n") || !strings.Contains(o.String(), "within word budget (20)") {
		t.Fatal(code, o.String())
	}
	if code := RunCLI([]string{d}, &o, &e); code != 2 {
		t.Fatal(code)
	}
}
