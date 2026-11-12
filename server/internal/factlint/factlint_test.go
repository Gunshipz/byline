package factlint

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"byline/internal/rules"
)

const src = "The pilot covers 26 offices and costs $4,500 a month. Only two of the five sites landed. " +
	"The vendor said: we need the permits first. Growth was 12% last year.\n"
const draft = "To: sam@example.com\nSubject: Pilot\n\nThe pilot covers 26 offices, 30 sites, and costs $4,500. Growth hit 12% and then 15%.\n" +
	"Three of the five sites landed.\n\"we need the permits first.\" was the line, and \"nobody asked for this\" too.\n" +
	"Acme Logistics Group runs it. The Review Board met. They felt rushed.\n\nThanks,\nSam\nManager, Ops\nWest Region Office\n"

var none = &rules.Set{}

// Measured 2026-11-10 with today's factlint.mjs. "$4,500." is flagged because the money pattern keeps the full stop.
func TestRun_Synthetic(t *testing.T) {
	want := []Flag{{"UNSOURCED_NUMBER", "30"}, {"UNSOURCED_NUMBER", "$4,500."}, {"UNSOURCED_NUMBER", "15%"},
		{"WRONG_COUNT", "Three of the five (source asserts 2)"}, {"UNSOURCED_QUOTE", "nobody asked for this"},
		{"UNSOURCED_PROPER_NOUN", "Acme Logistics Group"}, {"ASSUMED_REACTION", "They felt"}}
	if got := Run(none, draft, src); !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
}

const named = "Jordan Sample approved it.\nJordan Sample\nWest Region Office\n"

func TestRun_KnownNamesAndSignatureLines(t *testing.T) {
	want := []Flag{{"UNSOURCED_PROPER_NOUN", "Jordan Sample"}, {"UNSOURCED_PROPER_NOUN", "West Region Office"}}
	if got := Run(none, named, "nothing\n"); !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
	if got := Run(&rules.Set{KnownNames: []string{"Jordan"}, SignatureLines: []string{"Jordan Sample"}}, named, "nothing\n"); len(got) != 0 {
		t.Fatal(got)
	}
}

// Gate finding I4: with no signature_lines configured, factlint still must
// not flag a person's own closing sign-off.
func TestRun_NoSignatureLinesCutsGenericClosing(t *testing.T) {
	draft := "Thanks for reading.\n\nThanks,\nJordan Sample\nSenior Program Manager\nNorth America Operations\n"
	if got := Run(none, draft, "nothing here\n"); len(got) != 0 {
		t.Fatal(got)
	}
}

func TestRun_CRLFMatchesLF(t *testing.T) {
	if !reflect.DeepEqual(Run(none, strings.ReplaceAll(draft, "\n", "\r\n"), src), Run(none, draft, src)) {
		t.Fatal("CRLF differs")
	}
}

func TestFactlintCLI(t *testing.T) {
	dir := t.TempDir()
	d, s, one := filepath.Join(dir, "draft.txt"), filepath.Join(dir, "src.txt"), filepath.Join(dir, "one.txt")
	os.WriteFile(d, []byte(draft), 0o600)
	os.WriteFile(s, []byte(src), 0o600)
	os.WriteFile(one, []byte("The total is 30.\n"), 0o600)
	var o, e bytes.Buffer
	code := RunCLI([]string{d, s}, none, &o, &e)
	lines := strings.Split(o.String(), "\n")
	if code != 1 || lines[0] != d+" vs source: 7 source-fidelity flags" || lines[1] != `  [UNSOURCED_NUMBER] "30"` {
		t.Fatal(code, o.String())
	}
	o.Reset()
	if RunCLI([]string{one, s}, none, &o, &e); !strings.HasPrefix(o.String(), one+" vs source: 1 source-fidelity flag\n") {
		t.Fatal(o.String())
	}
	if code := RunCLI([]string{d}, none, &o, &e); code != 2 || !strings.Contains(e.String(), "usage: byline-mcp factlint <draft> <source>") {
		t.Fatal(code)
	}
}
