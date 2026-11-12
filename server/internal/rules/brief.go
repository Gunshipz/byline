package rules

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	bembed "byline/embed"
	"byline/internal/paths"
	"byline/internal/version"
)

// briefIntro is generate.mjs:73-76's two-line intro, with the blank line
// that follows the title and the blank line that precedes the first family
// section.
const briefIntro = "\nApply these BEFORE writing, as composition constraints. Each family has a generative TEST that\ncatches novel phrasing, not just the banned strings. The strings are examples, not the whole rule.\n\n"

// voiceOrder is generate.mjs:94-96's VOICE_ORDER: the voice_target keys
// that come first, in this order, when they exist; every other key (except
// "note") follows in merged order.
var voiceOrder = []string{"register", "contractions", "person", "sentences", "address_the_reader",
	"sentence_targets", "length_floor", "asks", "authority", "stance", "praise", "texture",
	"uncertainty", "phrases_he_uses", "punctuation"}

// voiceLabels names the display label for voice_target keys generate.mjs
// spells out by hand; any other key is labelled from its own name.
// phrases_he_uses reads "Phrases this person uses" here, not generate.mjs's
// "Phrases he uses" (lead ruling, 2026-11-10: the brief names no one).
var voiceLabels = map[string]string{
	"address_the_reader": "Address the reader",
	"sentence_targets":   "Sentence targets",
	"length_floor":       "Don't under-write",
	"phrases_he_uses":    "Phrases this person uses",
}

// Brief renders the composition brief: the stamp, the title, today's intro,
// one section per active family (minus exclude_from_brief), the voice
// target, then each stance appended verbatim. It is a port of
// generate.mjs:69-115.
func Brief(set *Set, coreStance, myStance, bylineVersion string, now time.Time) string {
	saved := set.PersonalSaved
	if saved == "" {
		saved = "never"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "<!-- byline %s | core rules %s | personal rules saved %s | generated %s -->\n",
		bylineVersion, set.CoreVersion, saved, now.UTC().Format("2006-01-02T15:04:05Z"))
	b.WriteString("# Writing rules: composition brief\n")
	b.WriteString(briefIntro)

	active := set.Active()
	for _, f := range set.Families {
		var inFamily []Rule
		for _, r := range active {
			if r.Family == f.ID && !r.ExcludeFromBrief {
				inFamily = append(inFamily, r)
			}
		}
		writeFamilySection(&b, f, inFamily)
	}

	b.WriteString("## VOICE TARGET (what to write, not just what to avoid)\n")
	writeVoiceTarget(&b, set.voice)

	for _, st := range []string{coreStance, myStance} {
		if st == "" {
			continue
		}
		b.WriteString("\n")
		b.WriteString(strings.TrimRight(st, "\n"))
		b.WriteString("\n")
	}

	return b.String()
}

// writeFamilySection writes one family's header, test, and its Banned /
// Also banned lines (each left out when it would be empty), then a blank
// line.
func writeFamilySection(b *strings.Builder, f Family, rules []Rule) {
	fmt.Fprintf(b, "## %d. %s\n", f.ID, strings.ToUpper(f.Name))
	fmt.Fprintf(b, "**TEST:** %s\n", f.Test)

	var banned []string
	var alsoBanned []string
	for _, r := range rules {
		for _, s := range r.Strings {
			if !strings.Contains(s, "\n") {
				banned = append(banned, s)
			}
		}
		if r.Structural != "" || len(r.Strings) == 0 {
			name := strings.ToLower(r.Name)
			if len(r.Strings) == 0 && r.Test != "" {
				// M7 (gate-p12-p5-review.md): trim the test's own
				// trailing period first, or it doubles up against the
				// "Also banned: ....\n" sentence period below.
				name += ": " + strings.TrimSuffix(r.Test, ".")
			}
			alsoBanned = append(alsoBanned, name)
		}
	}

	if len(banned) > 0 {
		quoted := make([]string, len(banned))
		for i, s := range banned {
			quoted[i] = `"` + s + `"`
		}
		fmt.Fprintf(b, "Banned: %s.\n", strings.Join(quoted, ", "))
	}
	if len(alsoBanned) > 0 {
		fmt.Fprintf(b, "Also banned: %s.\n", strings.Join(alsoBanned, "; "))
	}
	b.WriteString("\n")
}

// writeVoiceTarget writes the VOICE TARGET body: voiceOrder's keys first
// (when present), then the rest in merged order, skipping "note",
// "baseline", and any key whose value is empty.
func writeVoiceTarget(b *strings.Builder, voice Object) {
	byKey := map[string]json.RawMessage{}
	for _, kv := range voice {
		byKey[kv.Key] = kv.Value
	}

	placed := map[string]bool{}
	var keys []string
	for _, k := range voiceOrder {
		if _, ok := byKey[k]; ok {
			keys = append(keys, k)
			placed[k] = true
		}
	}
	for _, kv := range voice {
		if placed[kv.Key] {
			continue
		}
		keys = append(keys, kv.Key)
		placed[kv.Key] = true
	}

	for _, key := range keys {
		if key == "note" || key == "baseline" {
			continue
		}
		raw := byKey[key]
		if isEmptyVoiceValue(raw) {
			continue
		}
		switch key {
		case "register":
			var s string
			json.Unmarshal(raw, &s)
			b.WriteString(s)
			b.WriteString(" ")
		case "contractions":
			writeContractions(b, raw)
		case "person", "sentences":
			var s string
			json.Unmarshal(raw, &s)
			b.WriteString(s)
			b.WriteString("\n")
		default:
			writeGenericVoiceItem(b, voiceLabel(key), raw)
		}
	}
}

// isEmptyVoiceValue reports whether raw decodes to "", [] or {}.
func isEmptyVoiceValue(raw json.RawMessage) bool {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return false
	}
	switch t := v.(type) {
	case string:
		return t == ""
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	}
	return false
}

// writeContractions writes the Contractions line: the rule, then the first
// eight examples joined with ", ".
func writeContractions(b *strings.Builder, raw json.RawMessage) {
	var c struct {
		Rule     string   `json:"rule"`
		Examples []string `json:"examples"`
	}
	json.Unmarshal(raw, &c)
	examples := c.Examples
	if len(examples) > 8 {
		examples = examples[:8]
	}
	fmt.Fprintf(b, "**Contractions:** %s Examples: %s.\n", c.Rule, strings.Join(examples, ", "))
}

// writeGenericVoiceItem writes any voice_target key that isn't one of the
// four generate.mjs handles by hand: an array, object, string or other
// scalar, each under its label.
func writeGenericVoiceItem(b *strings.Builder, label string, raw json.RawMessage) {
	var v any
	json.Unmarshal(raw, &v)
	switch t := v.(type) {
	case []any:
		items := make([]string, len(t))
		for i, it := range t {
			items[i] = scalarText(it)
		}
		fmt.Fprintf(b, "**%s:** %s.\n", label, strings.Join(items, ", "))
	case map[string]any:
		obj, _ := DecodeObject(raw)
		parts := make([]string, len(obj))
		for i, kv := range obj {
			parts[i] = kv.Key + ": " + rawText(kv.Value)
		}
		fmt.Fprintf(b, "**%s:** %s.\n", label, strings.Join(parts, "; "))
	case string:
		fmt.Fprintf(b, "**%s:** %s\n", label, t)
	default:
		fmt.Fprintf(b, "**%s:** %s\n", label, strings.TrimSpace(string(raw)))
	}
}

// scalarText renders one decoded JSON array element as text: a string value
// bare, anything else as its raw JSON.
func scalarText(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	bs, _ := json.Marshal(v)
	return string(bs)
}

// rawText renders one object value as text: a string value bare, anything
// else as its raw JSON.
func rawText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return strings.TrimSpace(string(raw))
}

// voiceLabel returns the display label for a voice_target key: the name
// generate.mjs spells out by hand, or the key with its first rune
// upper-cased and "_" turned into spaces. M7 (gate-p12-p5-review.md):
// upper-casing key[:1] mangled any key starting with a multi-byte rune
// (e.g. "élan" became an invalid UTF-8 label), so the first rune is
// upper-cased on its own instead of its first byte.
func voiceLabel(key string) string {
	if l, ok := voiceLabels[key]; ok {
		return l
	}
	if key == "" {
		return key
	}
	first, size := utf8.DecodeRuneInString(key)
	return string(unicode.ToUpper(first)) + strings.ReplaceAll(key[size:], "_", " ")
}

// coreFS is the embedded core rules, rooted so LoadCore can read
// "core-rules.json" from its top.
func coreFS() fs.FS {
	sub, err := fs.Sub(bembed.Files, "rules")
	if err != nil {
		panic(err) // baked into the binary at build time; cannot fail
	}
	return sub
}

// Load returns the merged Set from Byline's embedded core rules plus
// personal (nil meaning core only). Its errors name the bad field.
func Load(personal []byte) (*Set, error) {
	return load(coreFS(), personal)
}

// Generate writes the composition brief to paths.BriefPath() and returns
// its path.
func Generate() (string, error) {
	return generate(coreFS(), version.Version(), time.Now())
}

// generate is Generate's testable core: core is the core rules filesystem,
// so a test can hand it a synthetic one.
func generate(core fs.FS, bylineVersion string, now time.Time) (string, error) {
	c, err := LoadCore(core)
	if err != nil {
		return "", err // a core failure writes nothing
	}

	personalBytes, err := readOptionalFile(paths.MyRulesPath())
	if err != nil {
		return "", err
	}

	var my *MyRules
	var loadErr error
	if personalBytes != nil {
		my, loadErr = ParseMyRules(personalBytes)
	}
	var set *Set
	if loadErr == nil {
		set, loadErr = Merge(c, my)
	}

	coreStance, err := readCoreStance(core)
	if err != nil {
		return "", err
	}
	myStance, err := readStanceFile(paths.MyStancePath())
	if err != nil {
		return "", err
	}

	if loadErr != nil {
		if _, err := os.Stat(paths.BriefPath()); err == nil {
			// A brief already exists: write nothing.
			return "", loadErr
		}
		coreOnly, mergeErr := Merge(c, nil)
		if mergeErr != nil {
			return "", mergeErr
		}
		coreOnly.PersonalSaved = readPersonalSaved()
		if err := writeBrief(Brief(coreOnly, coreStance, myStance, bylineVersion, now)); err != nil {
			return "", err
		}
		return paths.BriefPath(), fmt.Errorf("personal rules not applied: %w", loadErr)
	}

	set.PersonalSaved = readPersonalSaved()
	if err := writeBrief(Brief(set, coreStance, myStance, bylineVersion, now)); err != nil {
		return "", err
	}
	return paths.BriefPath(), nil
}

// readOptionalFile reads path, treating a missing file as nil with no
// error.
func readOptionalFile(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return b, nil
}

// readStanceFile reads path, treating a missing file as "" with no error;
// any other read error is returned. A file with no "## " heading (such as
// the template's own instructions, never filled in) adds nothing to the
// brief (lead ruling M1, 2026-11-10).
func readStanceFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if !hasStanceHeading(string(b)) {
		return "", nil
	}
	return string(b), nil
}

// hasStanceHeading reports whether s has a line starting "## ".
func hasStanceHeading(s string) bool {
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "## ") {
			return true
		}
	}
	return false
}

// readCoreStance reads core-stance.md from core, treating a missing file
// as "" with no error; any other read error is returned.
func readCoreStance(core fs.FS) (string, error) {
	b, err := fs.ReadFile(core, "core-stance.md")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return string(b), nil
}

// readPersonalSaved reads paths.RulesSavedPath(), trimmed; a missing or
// empty file stays "".
func readPersonalSaved() string {
	b, err := os.ReadFile(paths.RulesSavedPath())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// writeBrief writes content to paths.BriefPath() atomically: a temp file in
// the same folder (created with mode 0700), then a rename.
func writeBrief(content string) error {
	path := paths.BriefPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".brief-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, werr := tmp.WriteString(content)
	cerr := tmp.Close()
	if werr != nil {
		os.Remove(name)
		return werr
	}
	if cerr != nil {
		os.Remove(name)
		return cerr
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}
