package content

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// coreRules loads the repo's rules/core-rules.json.
func coreRules(t *testing.T) ruleFile {
	t.Helper()
	return loadRules(t, filepath.Join(repoRoot(t), "rules", "core-rules.json"))
}

// str returns m[k] when it's a string, else "".
func str(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return s
}

// keysEqual fails the test unless m's keys are exactly want.
func keysEqual(t *testing.T, label string, m map[string]any, want ...string) {
	t.Helper()
	var got []string
	for k := range m {
		got = append(got, k)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("%s keys = %v, want %v", label, got, want)
	}
}

func TestCoreRules_Shape(t *testing.T) {
	rf := coreRules(t)
	if rf.Version != "1.0.0" {
		t.Errorf("version = %q, want 1.0.0", rf.Version)
	}
	if len(rf.Families) != 6 {
		t.Errorf("%d families, want 6", len(rf.Families))
	}
	for i, f := range rf.Families {
		if id, _ := f["id"].(float64); id != float64(i+1) {
			t.Errorf("family %d has id %v, want %d", i, f["id"], i+1)
		}
		for _, k := range []string{"name", "test", "principle"} {
			if str(f, k) == "" {
				t.Errorf("family %v: empty %s", f["id"], k)
			}
		}
	}

	reserved := map[string]bool{"K8": true, "K9": true, "K11": true, "K34": true, "K40": true}
	seen := map[string]bool{}
	var nonOwn []string
	for _, r := range rf.Rules {
		id := str(r, "id")
		for _, k := range []string{"id", "name", "ban", "fix", "bad", "good"} {
			if str(r, k) == "" {
				t.Errorf("rule %q: empty %s", id, k)
			}
		}
		if fam, _ := r["family"].(float64); fam < 1 || fam > 6 || fam != float64(int(fam)) {
			t.Errorf("rule %q: family %v, want 1 to 6", id, r["family"])
		}
		if seen[id] {
			t.Errorf("rule %q appears twice", id)
		}
		seen[id] = true
		for _, k := range []string{"flagged", "note"} {
			if _, ok := r[k]; ok {
				t.Errorf("rule %q has a %q key", id, k)
			}
		}
		if reserved[id] {
			t.Errorf("rule %q is reserved for the personal file", id)
		}
		if strings.HasPrefix(id, "U") {
			t.Errorf("rule %q: core ids never start with U", id)
		}
		if !strings.HasPrefix(id, ownRulePrefix) {
			nonOwn = append(nonOwn, id)
			if _, ok := r["test"]; ok {
				t.Errorf("rule %q: legacy rules carry no test", id)
			}
		}
	}
	want := "K1 K2 K3 K4 K5 K6 K7 K10 K12 K13a K13b K13c K13d K13e K14 K15 K16 K17 K18 K19 K20 K35 K36 K37 K21 K22 K23 K24 K25 K26 K27 K28 G7 G19 G16 G17 G18 G23 G27 G28 P5 V1 K29 K30 K31 K32 K33 K38 K39"
	if got := strings.Join(nonOwn, " "); got != want {
		t.Errorf("non-own ids:\n got %s\nwant %s", got, want)
	}
}

// collectStrings appends every string inside v (recursing into objects and
// arrays) to out, labelled by path.
func collectStrings(path string, v any, out map[string]string) {
	switch x := v.(type) {
	case string:
		out[path] = x
	case map[string]any:
		for k, e := range x {
			collectStrings(path+"."+k, e, out)
		}
	case []any:
		for i, e := range x {
			collectStrings(fmt.Sprintf("%s[%d]", path, i), e, out)
		}
	}
}

func TestCoreRules_NoThirdPersonWriter(t *testing.T) {
	rf := coreRules(t)
	re := regexp.MustCompile(`(?i)\b(he|his|him|himself)\b`)
	texts := map[string]string{}
	for _, r := range rf.Rules {
		for _, k := range []string{"name", "ban", "fix", "exception", "removed_reason"} {
			texts[str(r, "id")+"."+k] = str(r, k)
		}
	}
	for _, f := range rf.Families {
		for _, k := range []string{"test", "principle"} {
			texts[fmt.Sprintf("family %v.%s", f["id"], k)] = str(f, k)
		}
	}
	for k, v := range rf.VoiceTarget {
		collectStrings("voice_target."+k, v, texts)
	}
	for where, text := range texts {
		if m := re.FindString(text); m != "" {
			t.Errorf("%s: %q in %q", where, m, text)
		}
	}
}

func TestCoreRules_StringsAreExact(t *testing.T) {
	for _, r := range coreRules(t).Rules {
		id := str(r, "id")
		list, _ := r["strings"].([]any)
		seen := map[string]bool{}
		for _, v := range list {
			s, ok := v.(string)
			switch {
			case !ok || s == "":
				t.Errorf("rule %q: empty or non-text string %v", id, v)
			case s != strings.TrimSpace(s):
				t.Errorf("rule %q: string %q has leading or trailing space", id, s)
			case seen[s]:
				t.Errorf("rule %q: string %q appears twice", id, s)
			}
			seen[s] = true
		}
	}
}

func TestCoreRules_VoiceTargetKeys(t *testing.T) {
	vt := coreRules(t).VoiceTarget
	keysEqual(t, "voice_target", vt, strings.Fields("note register contractions person sentences asks authority stance praise texture uncertainty phrases_he_uses punctuation address_the_reader sentence_targets length_floor")...)
	c, _ := vt["contractions"].(map[string]any)
	keysEqual(t, "voice_target.contractions", c, "rule", "examples")
	p, _ := vt["punctuation"].(map[string]any)
	keysEqual(t, "voice_target.punctuation", p, strings.Fields("em_dash hyphen_as_connector exclamation ellipsis semicolon parentheses colon oxford_comma")...)
}

func TestCoreRules_Adopted(t *testing.T) {
	rs := coreRules(t).Rules
	if len(rs) != 71 {
		t.Fatalf("%d rules, want 71", len(rs))
	}
	families := []int{3, 6, 4, 3, 3, 4, 1, 1, 2, 4, 3, 3, 3, 1, 5, 2, 1, 2, 4, 3, 4, 2}
	// M5 (2026-11-10): this copy's own rule 13 moved from a string match on
	// "→" to a judgment rule, since the arrow is also correct inside a menu
	// path.
	judgment := ownRuleJudgmentOnly
	want := ownRuleBannedPhrases
	for i, r := range rs[len(rs)-22:] {
		id := fmt.Sprintf("%s%d", ownRulePrefix, i+1)
		if got := str(r, "id"); got != id {
			t.Errorf("rule %d from the end is %q, want %q", 22-i, got, id)
			continue
		}
		if fam, _ := r["family"].(float64); fam != float64(families[i]) {
			t.Errorf("%s: family %v, want %d", id, r["family"], families[i])
		}
		var got []string
		list, _ := r["strings"].([]any)
		for _, v := range list {
			s, _ := v.(string)
			got = append(got, s)
		}
		_, hasTest := r["test"]
		if judgment[id] {
			if len(got) != 0 || str(r, "structural") != "judgment_only" || str(r, "test") != str(r, "ban") {
				t.Errorf("%s: want no strings, structural judgment_only and test equal to ban; got strings %q, structural %q, test %q", id, got, str(r, "structural"), str(r, "test"))
			}
			continue
		}
		if hasTest {
			t.Errorf("%s: a rule with strings carries no test", id)
		}
		if strings.Join(got, "\x00") != strings.Join(want[id], "\x00") {
			t.Errorf("%s: strings %q, want %q", id, got, want[id])
		}
	}
}

// TestCoreRules_K32FixHasNoPrivateQuote is the regression for I7: K32's
// fix field quoted the owner's real Slack post ("shooting from the hip").
// The fix field is a worked example, not sourced from anyone's real text.
func TestCoreRules_K32FixHasNoPrivateQuote(t *testing.T) {
	for _, r := range coreRules(t).Rules {
		if str(r, "id") != "K32" {
			continue
		}
		if fix := str(r, "fix"); strings.Contains(fix, "shooting from the hip") || strings.Contains(fix, "give this time back") {
			t.Errorf("K32 fix %q quotes the owner's real Slack post", fix)
		}
		return
	}
	t.Fatal("K32 not found")
}

// TestCoreRules_K19GoodHasNoPrivateExample is the lead's ruling on the
// review's declined list: K19's good example traced to a private AI-draft
// fixture. It must read as an invented, generic worked example instead.
func TestCoreRules_K19GoodHasNoPrivateExample(t *testing.T) {
	for _, r := range coreRules(t).Rules {
		if str(r, "id") != "K19" {
			continue
		}
		if good := str(r, "good"); strings.Contains(good, "hours people were actually saving") {
			t.Errorf("K19 good %q traces to the private fixture", good)
		}
		return
	}
	t.Fatal("K19 not found")
}

// TestCoreRules_K23PairHasNoPrivateExample is the lead's ruling on the
// review's declined list: K23's bad/good pair traced to a private task
// fixture. Both must read as an invented, generic worked example instead.
func TestCoreRules_K23PairHasNoPrivateExample(t *testing.T) {
	for _, r := range coreRules(t).Rules {
		if str(r, "id") != "K23" {
			continue
		}
		if strings.Contains(str(r, "bad"), "37,000") || strings.Contains(str(r, "good"), "hours math was done") {
			t.Errorf("K23 bad %q / good %q traces to the private fixture", str(r, "bad"), str(r, "good"))
		}
		return
	}
	t.Fatal("K23 not found")
}

// TestCoreRules_K28PairHasNoPrivateExample is the lead's ruling on the
// review's declined list: K28's bad/good pair traced to a private failure
// log. Both must read as an invented, generic worked example instead.
func TestCoreRules_K28PairHasNoPrivateExample(t *testing.T) {
	for _, r := range coreRules(t).Rules {
		if str(r, "id") != "K28" {
			continue
		}
		if strings.Contains(str(r, "good"), "next frontier") {
			t.Errorf("K28 good %q traces to the private fixture", str(r, "good"))
		}
		return
	}
	t.Fatal("K28 not found")
}

// ruleStrings returns rule r's "strings" field as []string.
func ruleStrings(r map[string]any) []string {
	list, _ := r["strings"].([]any)
	var got []string
	for _, v := range list {
		s, _ := v.(string)
		got = append(got, s)
	}
	return got
}

// TestCoreRules_M5ArrowIsJudgmentOnly is M5: "→" flagged prose by design
// even in a menu path, where this copy's own rule 13's fix says arrows
// belong in menu paths. Drop the string match and judge it like the
// family's other context-dependent rules.
func TestCoreRules_M5ArrowIsJudgmentOnly(t *testing.T) {
	id := ownRulePrefix + "13"
	for _, r := range coreRules(t).Rules {
		if str(r, "id") != id {
			continue
		}
		if got := ruleStrings(r); len(got) != 0 {
			t.Errorf("%s strings = %q, want none", id, got)
		}
		if str(r, "structural") != "judgment_only" {
			t.Errorf("%s structural = %q, want judgment_only", id, str(r, "structural"))
		}
		if str(r, "test") != str(r, "ban") {
			t.Errorf("%s test %q != ban %q", id, str(r, "test"), str(r, "ban"))
		}
		return
	}
	t.Fatalf("%s not found", id)
}

// TestCoreRules_M5NavigateAndUnpackAreFigurative is M5: bare "navigate" and
// "unpack" flagged ordinary technical prose ("navigate to Settings",
// "unpack the archive"). Narrow the strings to their figurative phrases.
func TestCoreRules_M5NavigateAndUnpackAreFigurative(t *testing.T) {
	for _, r := range coreRules(t).Rules {
		if str(r, "id") != "G7" {
			continue
		}
		got := ruleStrings(r)
		for _, bare := range []string{"navigate", "unpack"} {
			for _, s := range got {
				if s == bare {
					t.Errorf("G7 strings still has the bare word %q", bare)
				}
			}
		}
		for _, want := range []string{"navigate the complexities", "navigate this", "unpack this", "unpack that"} {
			found := false
			for _, s := range got {
				if s == want {
					found = true
				}
			}
			if !found {
				t.Errorf("G7 strings doesn't have the figurative phrase %q", want)
			}
		}
		return
	}
	t.Fatal("G7 not found")
}

func TestCoreStance_Shape(t *testing.T) {
	text := readFile(t, "rules/core-stance.md")
	if n := len(strings.Fields(text)); n > 600 {
		t.Errorf("%d words, want 600 or fewer", n)
	}
	if m := regexp.MustCompile(`\bS(10|[1-9])\b`).FindString(text); m != "" {
		t.Errorf("has the personal stance id %q", m)
	}
	if m := regexp.MustCompile(`\b20\d\d-\d\d-\d\d\b`).FindString(text); m != "" {
		t.Errorf("has the date %q", m)
	}
	var heads []string
	var bad, good []int
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(line, "## "):
			heads = append(heads, line)
			bad, good = append(bad, 0), append(good, 0)
		case len(heads) == 0:
		case strings.HasPrefix(line, "- BAD:"):
			bad[len(heads)-1]++
		case strings.HasPrefix(line, "- GOOD:"):
			good[len(heads)-1]++
		}
	}
	if len(heads) != 8 {
		t.Fatalf("%d sections, want CS1 to CS8: %q", len(heads), heads)
	}
	for i, h := range heads {
		if want := fmt.Sprintf("## CS%d. ", i+1); !strings.HasPrefix(h, want) {
			t.Errorf("section %d heading %q, want it to start %q", i+1, h, want)
		}
		if bad[i] != 1 || good[i] != 1 {
			t.Errorf("%q: %d BAD and %d GOOD lines, want one of each", h, bad[i], good[i])
		}
	}
}
