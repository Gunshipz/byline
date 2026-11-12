// Package rules holds Byline's writing-rules schema: the core rules Byline
// ships, and (from Task 1.3) a person's own rules merged on top of them.
package rules

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"strings"
)

// StructuralChecks names every structural check a rule's "structural" field
// may name.
var StructuralChecks = []string{"not_x_but_y", "three_or_more_no_X", "short_clause_colon_lowercase_payoff", "doubled_rather", "bold_label_colon_list_item", "title_case_heading", "emoji", "judgment_only"}

// BaselineMetrics names every metric a voice_target "baseline" object may
// hold a range for.
var BaselineMetrics = []string{"contraction_rate", "first_person_rate", "sent_len_mean", "sent_len_cv", "second_person_rate", "comma_chain_rate"}

var idRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

// Family is one of the writing-problem families a rule belongs to.
type Family struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Test      string `json:"test"`
	Principle string `json:"principle,omitempty"`
}

// Rule is one banned pattern, structural check or vocabulary swap.
type Rule struct {
	ID               string   `json:"id"`
	Family           int      `json:"family"`
	Name             string   `json:"name"`
	Ban              string   `json:"ban"`
	Strings          []string `json:"strings,omitempty"`
	Fix              string   `json:"fix"`
	Bad              string   `json:"bad"`
	Good             string   `json:"good"`
	Structural       string   `json:"structural,omitempty"`
	Exception        string   `json:"exception,omitempty"`
	Note             string   `json:"note,omitempty"`
	Autofix          bool     `json:"autofix,omitempty"`
	ExcludeFromBrief bool     `json:"exclude_from_brief,omitempty"`
	Flagged          string   `json:"flagged,omitempty"`
	Removed          []string `json:"removed,omitempty"`
	RemovedReason    string   `json:"removed_reason,omitempty"`
	Retired          bool     `json:"retired,omitempty"`
	Test             string   `json:"test,omitempty"`
}

// Core is the rules file Byline ships: core-rules.json.
type Core struct {
	Format      int      `json:"format"`
	Version     string   `json:"version"`
	Families    []Family `json:"families"`
	Rules       []Rule   `json:"rules"`
	VoiceTarget Object   `json:"voice_target"`
}

// ParseCore decodes data as a Core. It is lenient: it accepts today's
// rules.json shape, ignores unknown keys, and does not validate.
func ParseCore(data []byte) (*Core, error) {
	var c Core
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// LoadCore reads core-rules.json at the root of fsys, then parses and
// validates it.
func LoadCore(fsys fs.FS) (*Core, error) {
	data, err := fs.ReadFile(fsys, "core-rules.json")
	if err != nil {
		return nil, err
	}
	c, err := ParseCore(data)
	if err != nil {
		return nil, err
	}
	if err := ValidateCore(c); err != nil {
		return nil, err
	}
	return c, nil
}

// ValidateCore checks c for every problem it can find, joining them all
// with errors.Join.
func ValidateCore(c *Core) error {
	var errs []error
	if c.Format != 1 {
		errs = append(errs, fmt.Errorf("core rules: format %d, want 1", c.Format))
	}
	if c.Version == "" {
		errs = append(errs, errors.New("core rules: no version"))
	}

	families := map[int]bool{}
	seenFamily := map[int]bool{}
	for _, f := range c.Families {
		if seenFamily[f.ID] {
			errs = append(errs, fmt.Errorf("core rules: family %d appears twice", f.ID))
		}
		seenFamily[f.ID] = true
		families[f.ID] = true
		if f.Name == "" {
			errs = append(errs, fmt.Errorf("core rules: family %d: no name", f.ID))
		}
		if f.Test == "" {
			errs = append(errs, fmt.Errorf("core rules: family %d: no test", f.ID))
		}
	}

	seenRule := map[string]bool{}
	for _, r := range c.Rules {
		if seenRule[r.ID] {
			errs = append(errs, fmt.Errorf("core rules: rule %q appears twice", r.ID))
		}
		seenRule[r.ID] = true
		for _, e := range validateRule(r, families) {
			errs = append(errs, e)
		}
		if strings.HasPrefix(r.ID, "U") {
			errs = append(errs, fmt.Errorf("core rules: rule %q: ids starting with U are kept for personal rules", r.ID))
		}
	}

	if err := validateVoiceTarget(c.VoiceTarget); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// validateRule checks the parts of a rule that are the same whether it came
// from core-rules.json or from a person's own my-rules.json; Task 1.3
// reuses it for personal rules.
func validateRule(r Rule, families map[int]bool) []error {
	var errs []error
	if !idRe.MatchString(r.ID) {
		errs = append(errs, fmt.Errorf("rule %q: id must be a letter followed by letters, digits, _ or -", r.ID))
	}
	if !families[r.Family] {
		errs = append(errs, fmt.Errorf("rule %q: family %d does not exist", r.ID, r.Family))
	}
	if r.Name == "" {
		errs = append(errs, fmt.Errorf("rule %q: no name", r.ID))
	}
	if r.Ban == "" {
		errs = append(errs, fmt.Errorf("rule %q: no ban", r.ID))
	}
	for _, s := range r.Strings {
		if s == "" {
			errs = append(errs, fmt.Errorf("rule %q: empty string", r.ID))
		}
	}
	if r.Structural != "" && !stringIn(StructuralChecks, r.Structural) {
		errs = append(errs, fmt.Errorf("rule %q: unknown structural check %q", r.ID, r.Structural))
	}
	return errs
}

// voiceStringKeys names every voice_target key brief.go's writeVoiceTarget
// writes as a bare string (M7, gate-p12-p5-review.md): "register": 5
// silently printed blank instead of being rejected here, because that
// code path drops anything that isn't a JSON string with no error.
var voiceStringKeys = []string{"register", "person", "sentences"}

// validateVoiceTarget checks the voice_target object: that "contractions",
// if present, decodes to {"rule": string, "examples": []string}; that
// "baseline", if present, is an object of known metrics each a {"lo","hi"}
// range with lo not above hi; that register/person/sentences, if present,
// are JSON strings; and that no top-level value is JSON null.
func validateVoiceTarget(vt Object) error {
	var errs []error
	for _, kv := range vt {
		if strings.TrimSpace(string(kv.Value)) == "null" {
			errs = append(errs, fmt.Errorf("voice_target %s: value is null", kv.Key))
		}
		if stringIn(voiceStringKeys, kv.Key) {
			var s string
			if json.Unmarshal(kv.Value, &s) != nil {
				errs = append(errs, fmt.Errorf("voice_target %s: must be a string", kv.Key))
			}
		}
		switch kv.Key {
		case "contractions":
			var c struct {
				Rule     string   `json:"rule"`
				Examples []string `json:"examples"`
			}
			if json.Unmarshal(kv.Value, &c) != nil || c.Rule == "" || c.Examples == nil {
				errs = append(errs, errors.New(`voice_target contractions: needs "rule" and "examples"`))
			}
		case "baseline":
			if err := validateBaseline("voice_target baseline", kv.Value); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// validateBaseline checks raw as a baseline object: each key one of
// BaselineMetrics, each value a {"lo","hi"} range with lo not above hi.
// prefix names it in every error message -- "voice_target baseline" for
// a voice_target's own key (above), "voice_baseline" for extras.json's
// top-level key split_cli.go accepts (ValidateBaseline).
func validateBaseline(prefix string, raw json.RawMessage) error {
	obj, err := DecodeObject(raw)
	if err != nil {
		return fmt.Errorf("%s: %w", prefix, err)
	}
	var errs []error
	for _, m := range obj {
		if !stringIn(BaselineMetrics, m.Key) {
			errs = append(errs, fmt.Errorf("%s: unknown metric %q", prefix, m.Key))
			continue
		}
		var r struct {
			Lo float64 `json:"lo"`
			Hi float64 `json:"hi"`
		}
		if json.Unmarshal(m.Value, &r) != nil {
			errs = append(errs, fmt.Errorf(`%s %q: needs "lo" and "hi"`, prefix, m.Key))
			continue
		}
		if r.Lo > r.Hi {
			errs = append(errs, fmt.Errorf("%s %q: lo is above hi", prefix, m.Key))
		}
	}
	return errors.Join(errs...)
}

// ValidateBaseline checks raw as extras.json's voice_baseline object, in
// the same shape voice_target's own "baseline" key uses above
// (split_cli.go).
func ValidateBaseline(raw json.RawMessage) error {
	return validateBaseline("voice_baseline", raw)
}

func stringIn(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
