package rules

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Split computes the personal MyRules that, merged with core, reproduces
// old: a person's current rules.json split into what core already covers
// and what is still only theirs. It also returns, in file order, one note per
// difference no operation can restore (core's own new rules, strings or
// wording), so the person knows what changed.
func Split(old, core *Core) (*MyRules, []string) {
	m := &MyRules{Format: 1}
	var notes []string

	coreByID := map[string]Rule{}
	for _, r := range core.Rules {
		coreByID[r.ID] = r
	}
	oldFamilyByID := map[int]Family{}
	for _, f := range old.Families {
		oldFamilyByID[f.ID] = f
	}

	// Family notes come before rule notes.
	for _, f := range core.Families {
		if of, ok := oldFamilyByID[f.ID]; ok && of.Test != f.Test {
			notes = append(notes, fmt.Sprintf("family %d: core test wording kept", f.ID))
		}
	}

	matched := map[string]bool{}
	for _, o := range old.Rules {
		c, ok := coreByID[o.ID]
		if !ok {
			m.AddRules = append(m.AddRules, o)
			continue
		}
		matched[o.ID] = true

		coreStrings := map[string]bool{}
		for _, s := range c.Strings {
			coreStrings[strings.ToLower(s)] = true
		}
		oldStrings := map[string]bool{}
		for _, s := range o.Strings {
			oldStrings[strings.ToLower(s)] = true
		}
		oldRemoved := map[string]bool{}
		for _, s := range o.Removed {
			oldRemoved[strings.ToLower(s)] = true
		}

		// add_strings: old strings missing from core (case-insensitive),
		// in old order. A retired core rule loses every old string, so
		// all of them go here.
		var add []string
		for _, s := range o.Strings {
			if c.Retired || !coreStrings[strings.ToLower(s)] {
				add = append(add, s)
			}
		}
		if len(add) > 0 {
			if m.AddStrings == nil {
				m.AddStrings = map[string][]string{}
			}
			m.AddStrings[o.ID] = add
		}

		// switch_off: a core string old's removed list names.
		var off []SwitchOff
		for _, s := range c.Strings {
			if oldRemoved[strings.ToLower(s)] {
				off = append(off, SwitchOff{String: s, Reason: o.RemovedReason})
			}
		}
		if len(off) > 0 {
			if m.SwitchOff == nil {
				m.SwitchOff = map[string][]SwitchOff{}
			}
			m.SwitchOff[o.ID] = off
		}

		// replace_examples: old's bad/good, together, whenever either
		// differs from core's.
		if o.Bad != c.Bad || o.Good != c.Good {
			if m.ReplaceExamples == nil {
				m.ReplaceExamples = map[string]Examples{}
			}
			m.ReplaceExamples[o.ID] = Examples{Bad: o.Bad, Good: o.Good}
		}

		// A core string in neither old's strings nor its removed list is
		// new; no operation restores it, so it only gets a note.
		for _, s := range c.Strings {
			ls := strings.ToLower(s)
			if !oldStrings[ls] && !oldRemoved[ls] {
				notes = append(notes, fmt.Sprintf(`rule %s: new core string %q kept`, o.ID, s))
			}
		}

		// Differences in wording no operation restores.
		var fields []string
		if o.Name != c.Name {
			fields = append(fields, "name")
		}
		if o.Ban != c.Ban {
			fields = append(fields, "ban")
		}
		if o.Fix != c.Fix {
			fields = append(fields, "fix")
		}
		if o.Note != c.Note {
			fields = append(fields, "note")
		}
		if o.Exception != c.Exception {
			fields = append(fields, "exception")
		}
		if o.Test != c.Test {
			fields = append(fields, "test")
		}
		if len(fields) > 0 {
			notes = append(notes, fmt.Sprintf("rule %s: core %s wording kept", o.ID, strings.Join(fields, ", ")))
		}
	}

	// A core rule with no old rule of that id is new; note it, in core
	// order.
	for _, c := range core.Rules {
		if !matched[c.ID] {
			notes = append(notes, fmt.Sprintf("rule %s: new core rule kept", c.ID))
		}
	}

	m.VoiceTarget = splitVoiceTarget(old.VoiceTarget, core.VoiceTarget)

	return m, notes
}

// splitVoiceTarget returns, in old's order, every old voice_target key
// (other than "note") that core lacks, or whose compacted JSON differs
// from core's.
func splitVoiceTarget(old, core Object) Object {
	coreCompact := map[string][]byte{}
	for _, kv := range core {
		coreCompact[kv.Key] = compactJSON(kv.Value)
	}

	var out Object
	for _, kv := range old {
		if kv.Key == "note" {
			continue
		}
		cv, ok := coreCompact[kv.Key]
		if ok && bytes.Equal(compactJSON(kv.Value), cv) {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// compactJSON compacts raw JSON for byte-for-byte comparison, ignoring
// insignificant whitespace; raw that doesn't parse is returned unchanged.
func compactJSON(raw json.RawMessage) []byte {
	var buf bytes.Buffer
	if json.Compact(&buf, raw) != nil {
		return raw
	}
	return buf.Bytes()
}
