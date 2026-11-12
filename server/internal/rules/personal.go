package rules

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"byline/internal/paths"
)

// SwitchOff is one string a person wants a core rule to stop flagging, with
// their reason for turning it off.
type SwitchOff struct {
	String string `json:"string"`
	Reason string `json:"reason"`
}

// Examples replaces a rule's bad and/or good example.
type Examples struct {
	Bad  string `json:"bad,omitempty"`
	Good string `json:"good,omitempty"`
}

// MyRules is a person's my-rules.json: what they add, silence or replace on
// top of the core rules Byline ships.
type MyRules struct {
	Format          int                    `json:"format"`
	AddRules        []Rule                 `json:"add_rules,omitempty"`
	AddStrings      map[string][]string    `json:"add_strings,omitempty"`
	SwitchOff       map[string][]SwitchOff `json:"switch_off,omitempty"`
	ReplaceExamples map[string]Examples    `json:"replace_examples,omitempty"`
	VoiceTarget     Object                 `json:"voice_target,omitempty"`
	SignatureLines  []string               `json:"signature_lines,omitempty"`
	KnownNames      []string               `json:"known_names,omitempty"`
}

// utf8BOM is the UTF-8 byte order mark PowerShell 5.1 writes with
// `-Encoding UTF8` (M2, gate-p12-p5-review.md).
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// ParseMyRules decodes data as a MyRules. Formats 0 and 1 read the same
// way, and unknown keys are ignored; a format this version doesn't
// understand is refused. A leading UTF-8 BOM is stripped first, and a
// decoding error comes back as one plain sentence (plainJSONError), never
// raw Go JSON error text.
func ParseMyRules(data []byte) (*MyRules, error) {
	data = bytes.TrimPrefix(data, utf8BOM)
	var m MyRules
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, plainJSONError("my-rules.json", err)
	}
	if m.Format > 1 {
		return nil, fmt.Errorf("my-rules.json format %d is newer than this version of Byline understands (1); update Byline", m.Format)
	}
	return &m, nil
}

// LoadPersonalFile reads paths.MyRulesPath() (a missing or unreadable
// file means no personal rules, same as nil) and returns the merged Set:
// the one entry point the CLI (byline-mcp's factlint/lint/voicecheck
// modes) and every MCP tool share, so they behave the same way (gate
// finding, server/internal/mcp/checkers.go's currentRules duplicated
// server/cmd/byline-mcp/main.go's loadSet). A leading UTF-8 BOM is
// stripped (ParseMyRules already does this; stripped again here is a
// no-op). If the personal file doesn't parse, this degrades to core
// rules only, the way Generate already does, and returns a plain
// one-line warning naming the file instead of stopping the caller
// outright; the caller decides where that warning goes (a CLI's stderr,
// or an MCP tool's own result).
func LoadPersonalFile() (set *Set, warning string, err error) {
	b, _ := os.ReadFile(paths.MyRulesPath()) // missing or unreadable: same as no personal file
	b = bytes.TrimPrefix(b, utf8BOM)
	if s, lerr := Load(b); lerr == nil {
		return s, "", nil
	} else if len(b) > 0 {
		warning = "my-rules.json isn't valid; using core rules only"
	}
	set, err = Load(nil)
	return set, warning, err
}

// plainJSONError turns a json package decoding error into one plain
// sentence naming file and, when the decoder knows it, the offending
// field -- never the raw Go error text (M2).
func plainJSONError(file string, err error) error {
	var te *json.UnmarshalTypeError
	if errors.As(err, &te) {
		return fmt.Errorf("%s: field %q can't be a JSON %s", file, te.Field, te.Value)
	}
	return fmt.Errorf("%s: isn't valid JSON", file)
}

// familyIDs returns the set of family ids core defines, for validating
// rules a person adds.
func familyIDs(core *Core) map[int]bool {
	m := make(map[int]bool, len(core.Families))
	for _, f := range core.Families {
		m[f.ID] = true
	}
	return m
}

// ValidateMyRules checks my against core for every problem it can find,
// joining them all with errors.Join.
func ValidateMyRules(core *Core, my *MyRules) error {
	var errs []error
	families := familyIDs(core)

	known := map[string]bool{}
	coreIDs := map[string]bool{}
	for _, r := range core.Rules {
		known[r.ID] = true
		coreIDs[r.ID] = true
	}

	seen := map[string]bool{}
	for _, r := range my.AddRules {
		if coreIDs[r.ID] {
			errs = append(errs, fmt.Errorf("add_rules: rule %q is already a core rule", r.ID))
		}
		if seen[r.ID] {
			errs = append(errs, fmt.Errorf("add_rules: rule %q appears twice", r.ID))
		}
		seen[r.ID] = true
		known[r.ID] = true
		for _, e := range validateRule(r, families) {
			errs = append(errs, e)
		}
	}

	for id, strs := range my.AddStrings {
		if !known[id] {
			errs = append(errs, fmt.Errorf("add_strings: no rule %q", id))
			continue
		}
		for _, s := range strs {
			if s == "" {
				errs = append(errs, fmt.Errorf("add_strings %q: empty string", id))
			}
		}
	}

	for id, offs := range my.SwitchOff {
		if !known[id] {
			errs = append(errs, fmt.Errorf("switch_off: no rule %q", id))
			continue
		}
		for _, o := range offs {
			if o.Reason == "" {
				errs = append(errs, fmt.Errorf("switch_off %q %q: reason is required", id, o.String))
			}
		}
	}

	for id, ex := range my.ReplaceExamples {
		if !known[id] {
			errs = append(errs, fmt.Errorf("replace_examples: no rule %q", id))
			continue
		}
		if ex.Bad == "" && ex.Good == "" {
			errs = append(errs, fmt.Errorf("replace_examples %q: bad or good is required", id))
		}
	}

	// M6 (gate-p12-p5-review.md): an empty or whitespace-only entry cuts
	// cutSignature's draft at the wrong place (a sign-off block often has
	// a blank line), so it is rejected here rather than silently kept.
	for _, s := range my.SignatureLines {
		if strings.TrimSpace(s) == "" {
			errs = append(errs, errors.New("signature_lines: an entry is empty or blank"))
		}
	}
	for _, s := range my.KnownNames {
		if strings.TrimSpace(s) == "" {
			errs = append(errs, errors.New("known_names: an entry is empty or blank"))
		}
	}

	if err := validateVoiceTarget(my.VoiceTarget); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}
