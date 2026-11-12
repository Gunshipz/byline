package content

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// seedFile is the shape of a person's my-rules.json, the output of
// `byline-mcp dev split`.
type seedFile struct {
	Format          int                         `json:"format"`
	AddRules        []map[string]any            `json:"add_rules"`
	AddStrings      map[string][]string         `json:"add_strings"`
	SwitchOff       map[string][]map[string]any `json:"switch_off"`
	ReplaceExamples map[string]map[string]any   `json:"replace_examples"`
	VoiceTarget     map[string]any              `json:"voice_target"`
}

// loadSeed reads and parses a my-rules.json at path.
func loadSeed(t *testing.T, path string) seedFile {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("loadSeed: read %s: %v", path, err)
	}
	var sf seedFile
	if err := json.Unmarshal(data, &sf); err != nil {
		t.Fatalf("loadSeed: parse %s: %v", path, err)
	}
	return sf
}

// ruleByID indexes a []map[string]any rule list by its "id" field.
func ruleByID(rules []map[string]any) map[string]map[string]any {
	out := make(map[string]map[string]any, len(rules))
	for _, r := range rules {
		out[str(r, "id")] = r
	}
	return out
}

// stringSlice returns a []string copy of r[k] (a JSON array of strings),
// tolerating a missing or non-array key.
func stringSlice(r map[string]any, k string) []string {
	list, _ := r[k].([]any)
	out := make([]string, 0, len(list))
	for _, v := range list {
		s, _ := v.(string)
		out = append(out, s)
	}
	return out
}

// asSet turns a string slice into a sorted, de-duplicated slice, for
// set-equality comparisons.
func asSet(ss []string) []string {
	m := map[string]bool{}
	for _, s := range ss {
		m[s] = true
	}
	out := make([]string, 0, len(m))
	for s := range m {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// dropNulls returns a shallow copy of m with every key whose value is a
// JSON null removed. Legacy's export wrote some now-unused fields (such
// as an inactive "flagged") as explicit null where this phase's rule
// schema simply omits them; the two mean the same thing.
func dropNulls(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if v == nil {
			continue
		}
		out[k] = v
	}
	return out
}

// hasString reports whether s is in list.
func hasString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// TestPrivateSeed_MatchesLegacy checks that the private seed
// `byline-mcp dev split` writes from today's legacy rules.json, merged
// with this phase's core rules, reproduces every one of the legacy
// rules.json's 54 rules: every id lands in exactly one of core or the
// seed's add_rules, and every rule's merged strings, examples and
// removed list equal the legacy rule's own.
func TestPrivateSeed_MatchesLegacy(t *testing.T) {
	dir := privateDir(t)
	seedDir := filepath.Join(dir, "owner-voice-seed")
	seedPath := filepath.Join(seedDir, "my-rules.json")
	if _, err := os.Stat(seedPath); err != nil {
		t.Fatalf("seed missing: %s (run byline-mcp dev split first): %v", seedPath, err)
	}

	legacy := loadRules(t, filepath.Join(dir, "legacy", "rules.json"))
	core := coreRules(t)
	seed := loadSeed(t, seedPath)

	if seed.Format != 1 {
		t.Errorf("seed format = %d, want 1", seed.Format)
	}

	if len(legacy.Rules) != 54 {
		t.Fatalf("legacy has %d rules, want 54", len(legacy.Rules))
	}

	coreByID := ruleByID(core.Rules)
	addByID := ruleByID(seed.AddRules)
	legacyByID := ruleByID(legacy.Rules)

	personal := map[string]bool{"K8": true, "K9": true, "K11": true, "K34": true, "K40": true}

	// Every legacy id is in exactly one of core rules or the seed's
	// add_rules.
	for id := range legacyByID {
		_, inCore := coreByID[id]
		_, inAdd := addByID[id]
		switch {
		case inCore && inAdd:
			t.Errorf("rule %q is in both core and the seed's add_rules", id)
		case !inCore && !inAdd:
			t.Errorf("rule %q is in neither core nor the seed's add_rules", id)
		case inAdd != personal[id]:
			t.Errorf("rule %q is in add_rules = %v, want %v", id, inAdd, personal[id])
		}
	}
	if len(seed.AddRules) != len(personal) {
		t.Errorf("seed add_rules has %d rules, want %d", len(seed.AddRules), len(personal))
	}

	// Each add_rules entry deep-equals its legacy object.
	for id := range personal {
		want, ok := legacyByID[id]
		if !ok {
			t.Errorf("legacy has no rule %q", id)
			continue
		}
		got, ok := addByID[id]
		if !ok {
			t.Errorf("add_rules missing personal rule %q", id)
			continue
		}
		if !reflect.DeepEqual(dropNulls(got), dropNulls(want)) {
			t.Errorf("add_rules %q = %#v, want %#v (legacy's own object)", id, got, want)
		}
	}

	// For every legacy id, the merged strings, as a set, equal the legacy
	// strings — except M5's G7 narrowing, where core also ships four new
	// figurative phrases ("navigate the complexities", "navigate this",
	// "unpack this", "unpack that") the legacy rule never had reason to
	// list; the owner's own bare-word ban comes back through add_strings
	// alongside them, so the merged set is legacy's own plus those four.
	g7NewPhrases := []string{"navigate the complexities", "navigate this", "unpack this", "unpack that"}
	for id, legacyRule := range legacyByID {
		var merged []string
		if personal[id] {
			merged = stringSlice(addByID[id], "strings")
		} else {
			c, ok := coreByID[id]
			if !ok {
				continue // already reported above
			}
			merged = append(merged, stringSlice(c, "strings")...)
			merged = append(merged, seed.AddStrings[id]...)
			off := map[string]bool{}
			for _, o := range seed.SwitchOff[id] {
				off[str(o, "string")] = true
			}
			var kept []string
			for _, s := range merged {
				if !off[s] {
					kept = append(kept, s)
				}
			}
			merged = kept
		}
		want := stringSlice(legacyRule, "strings")
		if id == "G7" {
			want = append(append([]string{}, want...), g7NewPhrases...)
		}
		if got, want := asSet(merged), asSet(want); !reflect.DeepEqual(got, want) {
			t.Errorf("rule %q: merged strings %v, want %v", id, got, want)
		}
	}

	// The merged bad and good (replace_examples[id] when present,
	// otherwise core; a personal rule's own) equal the legacy pair.
	for id, legacyRule := range legacyByID {
		var bad, good string
		if personal[id] {
			bad, good = str(addByID[id], "bad"), str(addByID[id], "good")
		} else {
			c, ok := coreByID[id]
			if !ok {
				continue
			}
			bad, good = str(c, "bad"), str(c, "good")
			if ex, ok := seed.ReplaceExamples[id]; ok {
				if v := str(ex, "bad"); v != "" {
					bad = v
				}
				if v := str(ex, "good"); v != "" {
					good = v
				}
			}
		}
		wantBad, wantGood := str(legacyRule, "bad"), str(legacyRule, "good")
		if bad != wantBad || good != wantGood {
			t.Errorf("rule %q: merged bad/good = %q / %q, want %q / %q", id, bad, good, wantBad, wantGood)
		}
	}

	// The merged removed equals the legacy removed.
	for id, legacyRule := range legacyByID {
		var removed []string
		if personal[id] {
			removed = stringSlice(addByID[id], "removed")
		} else if c, ok := coreByID[id]; ok {
			removed = stringSlice(c, "removed")
		}
		if want := stringSlice(legacyRule, "removed"); !reflect.DeepEqual(removed, want) {
			t.Errorf("rule %q: merged removed = %v, want %v", id, removed, want)
		}
	}

	// replace_examples has exactly the 23 ids from the rule table (K19 and
	// K28 joined this list once the declined-list fix gave their core
	// bad/good new wording the owner's legacy text no longer matches), and
	// switch_off is empty.
	wantReplace := strings.Fields("K3 K6 K12 K15 K16 K18 K19 K20 K23 K24 K25 K26 K27 K28 K29 K30 K31 K32 K33 K35 K36 G27 V1")
	if len(seed.ReplaceExamples) != len(wantReplace) {
		t.Errorf("replace_examples has %d ids, want %d: %v", len(seed.ReplaceExamples), len(wantReplace), wantReplace)
	}
	for _, id := range wantReplace {
		if _, ok := seed.ReplaceExamples[id]; !ok {
			t.Errorf("replace_examples missing %q", id)
		}
	}
	for id := range seed.ReplaceExamples {
		if !hasString(wantReplace, id) {
			t.Errorf("replace_examples has unexpected id %q", id)
		}
	}
	if len(seed.SwitchOff) != 0 {
		t.Errorf("switch_off = %v, want empty", seed.SwitchOff)
	}

	// The seed's voice_target deep-equals the legacy voice_target, apart
	// from an added baseline key. signature_lines and known_names, if
	// present, aren't checked here.
	wantVT := map[string]any{}
	for k, v := range legacy.VoiceTarget {
		if k == "note" {
			continue
		}
		wantVT[k] = v
	}
	gotVT := map[string]any{}
	for k, v := range seed.VoiceTarget {
		if k == "baseline" {
			continue
		}
		gotVT[k] = v
	}
	if !reflect.DeepEqual(gotVT, wantVT) {
		t.Errorf("seed voice_target (minus baseline) does not equal legacy voice_target (minus note)")
	}

	// The bytes of my-stance.md equal the bytes of legacy/stance.md.
	legacyStance, err := os.ReadFile(filepath.Join(dir, "legacy", "stance.md"))
	if err != nil {
		t.Fatalf("read legacy/stance.md: %v", err)
	}
	seedStance, err := os.ReadFile(filepath.Join(seedDir, "my-stance.md"))
	if err != nil {
		t.Fatalf("read owner-voice-seed/my-stance.md: %v", err)
	}
	if !bytes.Equal(legacyStance, seedStance) {
		t.Error("owner-voice-seed/my-stance.md differs from legacy/stance.md")
	}
}
