package rules

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// Set is the merged rule set: the core rules Byline ships plus a person's
// own my-rules.json on top.
type Set struct {
	CoreVersion    string
	PersonalSaved  string
	Families       []Family
	Rules          []Rule
	SignatureLines []string
	KnownNames     []string
	Warnings       []string
	core           []Rule
	voice          Object
}

// Active returns the merged rules with Retired false, in order.
func (s *Set) Active() []Rule {
	var out []Rule
	for _, r := range s.Rules {
		if !r.Retired {
			out = append(out, r)
		}
	}
	return out
}

// Core returns the core rules exactly as loaded, retired rules included.
func (s *Set) Core() []Rule {
	return s.core
}

// VoiceTarget returns the merged voice target, decoded; an empty map
// (never nil) for a zero Set.
func (s *Set) VoiceTarget() map[string]any {
	out := map[string]any{}
	for _, kv := range s.voice {
		var v any
		if err := json.Unmarshal(kv.Value, &v); err != nil {
			continue
		}
		out[kv.Key] = v
	}
	return out
}

// Merge builds the merged Set from core and a person's my-rules.json. It
// validates my, not core; a nil my means empty.
func Merge(core *Core, my *MyRules) (*Set, error) {
	if my == nil {
		my = &MyRules{}
	}
	if err := ValidateMyRules(core, my); err != nil {
		return nil, err
	}

	// 1. Copy the core rules in core order, each with its own Strings
	// slice so later steps never mutate core's.
	rules := make([]Rule, len(core.Rules))
	for i, r := range core.Rules {
		rules[i] = r
		rules[i].Strings = append([]string(nil), r.Strings...)
	}

	// 2. add_rules: append in file order.
	for _, r := range my.AddRules {
		r.Strings = append([]string(nil), r.Strings...)
		rules = append(rules, r)
	}

	var warnings []string

	// 3. add_strings: append to the target rule, skipping strings
	// already there (case-insensitive). A retired core rule loses its
	// core strings and is no longer retired.
	for i := range rules {
		strs, ok := my.AddStrings[rules[i].ID]
		if !ok {
			continue
		}
		wasRetired := rules[i].Retired
		if wasRetired {
			rules[i].Strings = nil
			rules[i].Retired = false
		}
		for _, s := range strs {
			if indexFold(rules[i].Strings, s) < 0 {
				rules[i].Strings = append(rules[i].Strings, s)
			}
		}
		if wasRetired {
			warnings = append(warnings, fmt.Sprintf(`rule %q is retired in core; only your own strings for it still apply`, rules[i].ID))
		}
	}

	// 4. switch_off: remove the string from the target rule
	// (case-insensitive); a no-op on a retired rule.
	for i := range rules {
		offs, ok := my.SwitchOff[rules[i].ID]
		if !ok || rules[i].Retired {
			continue
		}
		for _, o := range offs {
			j := indexFold(rules[i].Strings, o.String)
			if j < 0 {
				warnings = append(warnings, fmt.Sprintf(`switch_off: rule %q has no string %q`, rules[i].ID, o.String))
				continue
			}
			rules[i].Strings = append(rules[i].Strings[:j], rules[i].Strings[j+1:]...)
		}
	}

	// 5. replace_examples: replace bad and/or good, each only when
	// non-empty.
	for i := range rules {
		ex, ok := my.ReplaceExamples[rules[i].ID]
		if !ok {
			continue
		}
		if ex.Bad != "" {
			rules[i].Bad = ex.Bad
		}
		if ex.Good != "" {
			rules[i].Good = ex.Good
		}
	}

	sort.Strings(warnings)

	s := &Set{
		CoreVersion:    core.Version,
		Families:       core.Families,
		Rules:          rules,
		SignatureLines: my.SignatureLines,
		KnownNames:     my.KnownNames,
		Warnings:       warnings,
		core:           core.Rules,
		voice:          mergeVoiceTarget(core.VoiceTarget, my.VoiceTarget),
	}
	return s, nil
}

// mergeVoiceTarget merges my over core, per key: core keys keep core
// order, with the person's value where given; the person's new keys
// follow in file order.
func mergeVoiceTarget(core, my Object) Object {
	override := map[string]json.RawMessage{}
	for _, kv := range my {
		override[kv.Key] = kv.Value
	}

	seen := map[string]bool{}
	var out Object
	for _, kv := range core {
		v := kv.Value
		if ov, ok := override[kv.Key]; ok {
			v = ov
		}
		out = append(out, KV{Key: kv.Key, Value: v})
		seen[kv.Key] = true
	}
	for _, kv := range my {
		if seen[kv.Key] {
			continue
		}
		out = append(out, kv)
		seen[kv.Key] = true
	}
	return out
}

// indexFold returns the index of the first string in list that equals s
// case-insensitively, or -1.
func indexFold(list []string, s string) int {
	for i, v := range list {
		if strings.EqualFold(v, s) {
			return i
		}
	}
	return -1
}

// load reads core rules from fsys and personal (nil meaning empty) and
// returns the merged Set. Task 1.4's Load wraps this.
func load(core fs.FS, personal []byte) (*Set, error) {
	c, err := LoadCore(core)
	if err != nil {
		return nil, err
	}
	var my *MyRules
	if personal != nil {
		my, err = ParseMyRules(personal)
		if err != nil {
			return nil, err
		}
	}
	return Merge(c, my)
}
