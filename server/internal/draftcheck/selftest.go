package draftcheck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"byline/internal/rules"
)

// SelftestCase is one of Selftest's in-process cases.
type SelftestCase struct {
	Name  string
	Draft string
	Spec  Spec
	Want  func(pass bool, out string) bool
}

// SelftestCases returns today's 11 selftest cases (draftcheck.mjs:218-309),
// with the real names and project terms replaced by invented ones, per the
// task brief's replacement table.
func SelftestCases() []SelftestCase {
	section := func(pairs ...any) *rules.Object {
		o := &rules.Object{}
		for i := 0; i < len(pairs); i += 2 {
			n := pairs[i+1].(int)
			*o = append(*o, rules.KV{Key: pairs[i].(string), Value: json.RawMessage(fmt.Sprintf("%d", n))})
		}
		return o
	}

	spec := Spec{
		Reader: "Sam", Intent: "inform",
		Questions:  []string{"is this realistic and why", "how much planning time", "what does it cost the team"},
		Budget:     350,
		SectionMap: section("Why it's realistic", 1, "Planning time", 2, "What it costs the team", 3),
		KillList: []string{
			"the venue discount Priya already locked in for the team offsite",
			"we can already book the whole team for one flat day rate",
		},
	}

	body := "Sam, here's the short version.\n\n**Why it's realistic**\n\nFinance already pre-approved a per-person cap and two vendors sent quotes that fit inside it.\n\n**Planning time**\n\nI don't know yet. Across the 6 venues we're comparing there are three different catering menus, so there's no single plan to lock in.\n\n**What it costs the team**\n\nIt pulls my focus off the roadmap work.\n"
	last := "It pulls my focus off the roadmap work."

	revived := strings.Replace(body, last,
		"Some of it we get for nothing. The venue discount Priya already locked in for the team offsite is already confirmed, and we can already book the whole team for one flat day rate at that same venue.", 1)
	legit := strings.Replace(body, last,
		"The venue already confirmed our headcount for the room block, catering is holding a minimum we haven't signed, and finance still needs to sign off.", 1)

	specLegit := spec
	specLegit.KillList = append(append([]string{}, spec.KillList...), "confirm the catering minimum and headcount guarantee with the venue")

	fat := body + "\n" + strings.Repeat("padding ", 400)
	noq := Spec{Reader: "Sam", Intent: "inform", Budget: 350, Questions: []string{}, SectionMap: &rules.Object{}}

	extra := body + "\n**What I recommend**\n\nConfirm the room block by Friday.\n"
	missing := strings.Replace(body, "**What it costs the team**\n\nIt pulls my focus off the roadmap work.\n", "", 1)

	shortSpec := Spec{
		Reader: "Dana", Intent: "inform", Budget: 70, SectionMap: &rules.Object{},
		Questions: []string{"send the booth artwork", "is the badge printing manual or automated"},
		KillList:  []string{"it is the step we would need most for check-in"},
	}
	shortOk := "Hi Dana, just a reminder to send over the booth artwork when you get a chance. Rough is fine, don't spend any time polishing it.\n\nOne question while I'm digging. Is the badge printing done by hand at the registration desk, or is there a script behind it? That's the one piece I can't find anywhere.\n"
	shortBad := "Hi Dana, send the booth artwork.\n\nIs the badge printing manual or automated? That's the one piece I can't find, and it's the step we'd need most for check-in.\n"

	mismatched := Spec{
		Reader: "Dana", Intent: "inform", Budget: 70,
		Questions:  []string{"send the artwork", "manual or automated"},
		SectionMap: section("Signage", 1),
		KillList:   []string{},
	}

	fails := func(sub string) func(bool, string) bool {
		return func(pass bool, out string) bool { return !pass && strings.Contains(out, sub) }
	}
	passesAnd := func(sub string) func(bool, string) bool {
		return func(pass bool, out string) bool { return pass && strings.Contains(out, sub) }
	}

	return []SelftestCase{
		{"catches reworded killed content", revived, spec, fails("revives")},
		{`flags persuasion marker "some of it we get for"`, revived, spec,
			func(pass bool, out string) bool {
				return strings.Contains(strings.ToLower(out), "some of it we get for")
			}},
		{"clean draft passes", body, spec, func(pass bool, out string) bool { return pass }},
		{"does NOT fire on legitimate prose sharing kill-list words", legit, specLegit,
			func(pass bool, out string) bool { return !strings.Contains(out, "revives") }},
		{"catches over budget", fat, spec, fails("over by")},
		{"refuses when no reader questions declared", body, noq, fails("NONE DECLARED")},
		{"catches an undeclared extra section", extra, spec, fails("not declared in sectionMap")},
		{"catches a declared section missing from the draft", missing, spec, fails("is missing from the draft")},
		{"short unsectioned message passes", shortOk, shortSpec, passesAnd("short form")},
		{"short form still catches killed content", shortBad, shortSpec, fails("revives")},
		{"catches sections declared but absent from an unsectioned draft", shortOk, mismatched, fails("no headers")},
	}
}

// Selftest runs every SelftestCase in process (unlike draftcheck.mjs's
// --selftest, which spawns a subprocess per case) and writes no receipts.
// It prints "  ok   <name>" or "  FAIL <name>" per case, then
// "\nselftest: <p> passed, <f> failed\n\n". It returns 1 on any failure.
func Selftest(w io.Writer) int {
	pass, fail := 0, 0
	for _, c := range SelftestCases() {
		r := check(c.Draft, c.Spec)
		var buf bytes.Buffer
		Render(&buf, "", r)
		if c.Want(r.Pass, buf.String()) {
			pass++
			fmt.Fprintf(w, "  ok   %s\n", c.Name)
		} else {
			fail++
			fmt.Fprintf(w, "  FAIL %s\n", c.Name)
		}
	}
	fmt.Fprintf(w, "\nselftest: %d passed, %d failed\n\n", pass, fail)
	if fail > 0 {
		return 1
	}
	return 0
}
