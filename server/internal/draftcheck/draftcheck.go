// Package draftcheck is the structural pre-send check for a drafted comm:
// cut content returning (reworded or relocated), sections that answer no
// reader question, drafts over the agreed word budget, and persuasion
// architecture in a comm whose job is to inform. It is a port of today's
// draftcheck.mjs; see the task brief for the ported-verbatim decisions.
package draftcheck

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"byline/internal/rules"
	"byline/internal/textutil"
)

// Spec is the JSON spec a draft is checked against.
type Spec struct {
	Reader     string        `json:"reader"`
	Intent     string        `json:"intent"`
	Questions  []string      `json:"questions"`
	Budget     float64       `json:"budget"`
	SectionMap *rules.Object `json:"sectionMap,omitempty"`
	KillList   []string      `json:"killList"`
}

// specAux mirrors Spec's JSON shape, keeping "budget" and "sectionMap" as
// raw JSON so UnmarshalJSON can accept the looser types Node's untyped JS
// let through: a numeric string for budget, and an empty array for
// sectionMap (JS's Object.entries treats {} and [] alike).
type specAux struct {
	Reader     string          `json:"reader"`
	Intent     string          `json:"intent"`
	Questions  []string        `json:"questions"`
	Budget     json.RawMessage `json:"budget"`
	SectionMap json.RawMessage `json:"sectionMap"`
	KillList   []string        `json:"killList"`
}

// UnmarshalJSON decodes a draftcheck spec. Gate finding M3: Node accepted a
// numeric string for "budget" and an empty array for "sectionMap"; this
// does too. Any other type mismatch, on these fields or any other, gives a
// plain message naming the field instead of Go's raw JSON error.
func (s *Spec) UnmarshalJSON(data []byte) error {
	var aux specAux
	if err := json.Unmarshal(data, &aux); err != nil {
		return plainSpecError(err)
	}
	s.Reader, s.Intent, s.Questions, s.KillList = aux.Reader, aux.Intent, aux.Questions, aux.KillList

	if len(aux.Budget) > 0 && string(aux.Budget) != "null" {
		budget, err := parseBudget(aux.Budget)
		if err != nil {
			return err
		}
		s.Budget = budget
	}

	if len(aux.SectionMap) > 0 && string(aux.SectionMap) != "null" {
		sm, err := parseSectionMap(aux.SectionMap)
		if err != nil {
			return err
		}
		s.SectionMap = sm
	}
	return nil
}

// plainSpecError turns a json.Unmarshal error into a plain sentence naming
// spec.json and, where Go's error identifies one, the field — never Go's
// raw "cannot unmarshal ... into Go struct field" text.
func plainSpecError(err error) error {
	var te *json.UnmarshalTypeError
	if errors.As(err, &te) && te.Field != "" {
		return fmt.Errorf("spec.json's %q field has the wrong type", te.Field)
	}
	return fmt.Errorf("spec.json isn't valid JSON: %v", err)
}

// parseBudget decodes a spec's "budget" field: a JSON number, or — Node's
// untyped `spec.budget || Infinity` passed one through unchanged — a
// numeric string. Anything else gives a plain message naming the field.
func parseBudget(raw json.RawMessage) (float64, error) {
	var n float64
	if json.Unmarshal(raw, &n) == nil {
		return n, nil
	}
	var str string
	if json.Unmarshal(raw, &str) == nil {
		if n, err := strconv.ParseFloat(strings.TrimSpace(str), 64); err == nil {
			return n, nil
		}
	}
	return 0, fmt.Errorf(`spec.json's "budget" field must be a number`)
}

// parseSectionMap decodes a spec's "sectionMap" field: a JSON object (the
// ordinary case), or — Node's untyped Object.entries treats {} and []
// alike — an empty array, which becomes an empty Object. Anything else
// gives a plain message naming the field.
func parseSectionMap(raw json.RawMessage) (*rules.Object, error) {
	if bytes.HasPrefix(bytes.TrimSpace(raw), []byte("[")) {
		var arr []json.RawMessage
		if json.Unmarshal(raw, &arr) == nil && len(arr) == 0 {
			empty := rules.Object{}
			return &empty, nil
		}
		return nil, fmt.Errorf(`spec.json's "sectionMap" field must be an object`)
	}
	obj, err := rules.DecodeObject(raw)
	if err != nil {
		return nil, fmt.Errorf(`spec.json's "sectionMap" field must be an object`)
	}
	return &obj, nil
}

// Check is one of the five structural checks.
type Check struct {
	Name   string
	Pass   bool
	Detail []string
}

// Result is the outcome of checking one draft against one spec.
type Result struct {
	Pass    bool
	Reader  string
	Text    string // the draft after Normalize and the Subject strip
	Words   int
	Checks  []Check
	Receipt string // the receipt path, or "" when the draft failed
}

// utf8BOM is the UTF-8 byte-order mark PowerShell's `-Encoding UTF8` writes
// at the start of a file.
var utf8BOM = []byte("\xef\xbb\xbf")

// stripBOM drops a leading UTF-8 BOM from b, so a spec.json PowerShell wrote
// still decodes.
func stripBOM(b []byte) []byte {
	return bytes.TrimPrefix(b, utf8BOM)
}

// Run parses spec, checks draft and, on a pass, writes the receipt through
// WriteReceipt("", "", result.Text, time.Now()). It errors on bad spec JSON
// or a failed receipt write.
func Run(draft string, spec []byte) (Result, error) {
	var s Spec
	if err := json.Unmarshal(stripBOM(spec), &s); err != nil {
		return Result{}, err
	}
	r := check(draft, s)
	if r.Pass {
		p, err := WriteReceipt("", "", r.Text, time.Now())
		if err != nil {
			return r, err
		}
		r.Receipt = p
	}
	return r, nil
}

// Render prints a draftcheck run's header and every check, the way
// draftcheck.mjs's run() does from "\ndraftcheck: " through the last
// detail line. The caller (the CLI) prints the pass/fail summary after.
func Render(w io.Writer, draftPath string, r Result) {
	fmt.Fprintf(w, "\ndraftcheck: %s\n", draftPath)
	reader := r.Reader
	if reader == "" {
		reader = "(unspecified)"
	}
	fmt.Fprintf(w, "reader: %s\n\n", reader)
	for _, c := range r.Checks {
		status := "FAIL"
		if c.Pass {
			status = "PASS"
		}
		fmt.Fprintf(w, "  %s  %s\n", status, c.Name)
		for _, d := range c.Detail {
			fmt.Fprintf(w, "        %s\n", d)
		}
	}
}

// stopWords mirrors draftcheck.mjs's STOP set verbatim.
var stopWords = map[string]bool{}

func init() {
	for _, w := range []string{"the", "a", "an", "and", "or", "but", "of", "to", "in", "on", "at", "for", "with", "is", "are", "was",
		"were", "be", "been", "it", "its", "this", "that", "these", "those", "we", "our", "i", "my", "you", "your", "he", "she", "they",
		"as", "by", "from", "not", "no", "do", "does", "did", "has", "have", "had", "will", "would", "can", "could", "so", "if", "then",
		"than", "there", "their", "what", "which", "who", "when", "how", "all", "any", "also", "about", "into", "out", "up", "down",
		"over", "under", "more", "most", "some", "one", "two"} {
		stopWords[w] = true
	}
}

var wordRe = regexp.MustCompile(`[a-z0-9']+`)

// words mirrors draftcheck.mjs's words(): every run of ASCII letters,
// digits or apostrophes, lowercased, including stop words.
func words(s string) []string {
	return wordRe.FindAllString(strings.ToLower(s), -1)
}

// ContentWords mirrors draftcheck.mjs's content(): words(), minus stop
// words and anything two characters or shorter.
func ContentWords(s string) []string {
	ws := words(s)
	out := make([]string, 0, len(ws))
	for _, w := range ws {
		if !stopWords[w] && len(w) > 2 {
			out = append(out, w)
		}
	}
	return out
}

// persuasion mirrors draftcheck.mjs's PERSUASION list verbatim, copied into
// RE2 syntax (none of them need a lookaround).
var persuasion = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bwhat convinced\b`),
	regexp.MustCompile(`(?i)\bi'?m convinced\b`),
	regexp.MustCompile(`(?i)\bwhat sold me\b`),
	regexp.MustCompile(`(?i)\bthe case for\b`),
	regexp.MustCompile(`(?i)\bwhy this matters\b`),
	regexp.MustCompile(`(?i)\bmakes the case\b`),
	regexp.MustCompile(`(?i)\bi'?d argue\b`),
	regexp.MustCompile(`(?i)\bthe upside\b`),
	regexp.MustCompile(`(?i)\bworth noting\b`),
	regexp.MustCompile(`(?i)\bon the one hand\b`),
	regexp.MustCompile(`(?i)\bin some (places|ways|respects) and\b`),
	regexp.MustCompile(`(?i)\bfurther along than\b`),
	regexp.MustCompile(`(?i)\bhonest read is that this is (further|more|less)\b`),
	regexp.MustCompile(`(?i)\bsome of it we get for\b`),
	regexp.MustCompile(`(?i)\bthe good news\b`),
	regexp.MustCompile(`(?i)\bthe bad news\b`),
}

var paraSplitRe = regexp.MustCompile(`\n\s*\n`)

// paragraphs mirrors draftcheck.mjs's paragraphs().
func paragraphs(text string) []string {
	var out []string
	for _, p := range paraSplitRe.Split(text, -1) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

var subjectLineRe = regexp.MustCompile(`(?im)^\s*Subject:.*$`)

// stripMeta mirrors draftcheck.mjs's stripMeta(): drop the first leading
// "Subject:" line (only the first match, as the old script's non-global
// regex did), then trim the whole result.
func stripMeta(text string) string {
	if loc := subjectLineRe.FindStringIndex(text); loc != nil {
		text = text[:loc[0]] + text[loc[1]:]
	}
	return strings.TrimSpace(text)
}

type killFail struct {
	item      string
	paragraph int
	matched   []string
	excerpt   string
}

// checkKillList mirrors draftcheck.mjs's checkKillList(): a windowSize of
// 30, needing 80% of an entry's content words (at least 3) inside one
// 30-word sliding window of a paragraph.
func checkKillList(paras []string, killList []string) []killFail {
	const windowSize = 30
	var fails []killFail
	for _, item := range killList {
		terms := uniqueStrings(ContentWords(item))
		if len(terms) < 3 {
			continue
		}
		need := int(math.Ceil(float64(len(terms)) * 0.8))
		if need < 3 {
			need = 3
		}
		for i, p := range paras {
			pw := ContentWords(p)
			limit := len(pw) - windowSize + 1
			if limit < 1 {
				limit = 1
			}
			var best []string
			for s := 0; s < limit; s++ {
				end := s + windowSize
				if end > len(pw) {
					end = len(pw)
				}
				win := map[string]bool{}
				for _, w := range pw[s:end] {
					win[w] = true
				}
				var hits []string
				for _, t := range terms {
					if win[t] {
						hits = append(hits, t)
					}
				}
				if len(hits) > len(best) {
					best = hits
				}
			}
			if len(best) >= need {
				fails = append(fails, killFail{item: item, paragraph: i + 1, matched: best, excerpt: textutil.Slice16(p, 0, 110)})
			}
		}
	}
	return fails
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

var headerRe = regexp.MustCompile(`(?m)^\s*(?:\*\*(.+?)\*\*|\*(.+?)\*|#{1,4}\s*(.+?))\s*$`)
var subjectHeaderRe = regexp.MustCompile(`(?i)^subject:`)

type sectionResult struct {
	headers   []string
	problems  []string
	shortForm bool
}

func sectionMapValue(sm *rules.Object, key string) (json.RawMessage, bool) {
	for _, kv := range *sm {
		if kv.Key == key {
			return kv.Value, true
		}
	}
	return nil, false
}

func sectionMapNumber(sm *rules.Object, key string) (float64, bool) {
	raw, ok := sectionMapValue(sm, key)
	if !ok {
		return 0, false
	}
	var n float64
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, false
	}
	return n, true
}

// sectionMapDisplay renders a sectionMap value the way a JS template
// literal would, for the one message that prints it when it is not a
// number.
// shortcut: an array or object value prints as Go's %v, not JS's
// comma-join or "[object Object]"; upgrade if a real spec ever maps a
// section to a non-number, non-string, non-bool value.
func sectionMapDisplay(sm *rules.Object, key string) string {
	raw, ok := sectionMapValue(sm, key)
	if !ok {
		return "undefined"
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return string(raw)
	}
	switch t := v.(type) {
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		return textutil.FormatNum(t)
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%v", t)
	}
}

// checkSectionMap mirrors draftcheck.mjs's checkSectionMap() verbatim,
// including every message.
func checkSectionMap(text string, questions []string, sectionMap *rules.Object) sectionResult {
	var headers []string
	for _, m := range headerRe.FindAllStringSubmatch(text, -1) {
		h := m[1]
		if h == "" {
			h = m[2]
		}
		if h == "" {
			h = m[3]
		}
		h = strings.TrimSpace(h)
		if h == "" || subjectHeaderRe.MatchString(h) {
			continue
		}
		headers = append(headers, h)
	}

	if sectionMap == nil {
		return sectionResult{headers: headers,
			problems: []string{"spec has no sectionMap; declare which question each section answers, or set it to {} for a short unsectioned message"}}
	}
	var mapped []string
	for _, kv := range *sectionMap {
		mapped = append(mapped, kv.Key)
	}

	if len(headers) == 0 && len(mapped) == 0 {
		return sectionResult{headers: headers, shortForm: true}
	}
	if len(headers) == 0 && len(mapped) > 0 {
		return sectionResult{headers: headers, problems: []string{fmt.Sprintf("spec declares %d section(s) but the draft has no headers", len(mapped))}}
	}

	var problems []string
	for _, h := range headers {
		found := false
		for _, m := range mapped {
			if strings.EqualFold(m, h) {
				found = true
				break
			}
		}
		if !found {
			problems = append(problems, fmt.Sprintf(`section "%s" is in the draft but not declared in sectionMap — it may be content the user did not ask for`, h))
		}
	}
	for _, m := range mapped {
		found := false
		for _, h := range headers {
			if strings.EqualFold(h, m) {
				found = true
				break
			}
		}
		if !found {
			problems = append(problems, fmt.Sprintf(`declared section "%s" is missing from the draft`, m))
		}
		qi, ok := sectionMapNumber(sectionMap, m)
		if !ok || qi < 1 || qi > float64(len(questions)) {
			problems = append(problems, fmt.Sprintf("section \"%s\" maps to question %s, which is not one of the %d declared", m, sectionMapDisplay(sectionMap, m), len(questions)))
		}
	}
	// Keyed on the exact float, not int(qi): Node's Set holds the raw
	// values from Object.values(sectionMap) and answered.has(i+1) is an
	// exact match, so a non-integer index like 2.5 answers no question.
	answered := map[float64]bool{}
	for _, m := range mapped {
		if qi, ok := sectionMapNumber(sectionMap, m); ok {
			answered[qi] = true
		}
	}
	for i, q := range questions {
		if !answered[float64(i+1)] {
			problems = append(problems, fmt.Sprintf(`question %d ("%s") has no section answering it`, i+1, q))
		}
	}
	return sectionResult{headers: headers, problems: problems}
}

// check is the pure, unexported port of draftcheck.mjs's run(), minus the
// file I/O, printing and receipt write. Run, the CLI and Selftest all use
// it.
func check(text string, spec Spec) Result {
	text = textutil.Normalize(text)
	text = stripMeta(text)
	paras := paragraphs(text)
	wc := len(words(text))

	var checks []Check

	// 1. kill list
	fails := checkKillList(paras, spec.KillList)
	var killDetail []string
	if len(fails) == 0 {
		killDetail = []string{"no killed content found"}
	} else {
		for _, f := range fails {
			killDetail = append(killDetail, fmt.Sprintf("para %d revives \"%s\" (matched %s)\n      > %s",
				f.paragraph, f.item, strings.Join(f.matched, ", "), f.excerpt))
		}
	}
	checks = append(checks, Check{Name: "cut content did not return", Pass: len(fails) == 0, Detail: killDetail})

	// 2. budget
	budget := spec.Budget
	if budget == 0 {
		budget = math.Inf(1)
	}
	within := float64(wc) <= budget
	budgetDetail := fmt.Sprintf("%d words", wc)
	if !within {
		budgetDetail += fmt.Sprintf(", over by %s", textutil.FormatNum(float64(wc)-budget))
	}
	budgetLabel := "Infinity"
	if spec.Budget != 0 {
		budgetLabel = textutil.FormatNum(spec.Budget)
	}
	checks = append(checks, Check{Name: fmt.Sprintf("within word budget (%s)", budgetLabel), Pass: within, Detail: []string{budgetDetail}})

	// 3. reader questions declared
	qs := spec.Questions
	qPass := len(qs) >= 2 && len(qs) <= 4
	var qDetail []string
	if len(qs) == 0 {
		qDetail = []string{"NONE DECLARED — do not draft without these"}
	} else {
		for i, q := range qs {
			qDetail = append(qDetail, fmt.Sprintf("%d. %s", i+1, q))
		}
	}
	checks = append(checks, Check{Name: "reader questions declared (2-4)", Pass: qPass, Detail: qDetail})

	// 4. every section maps to a declared reader question, and vice versa
	sres := checkSectionMap(text, qs, spec.SectionMap)
	var sDetail []string
	switch {
	case len(sres.problems) > 0:
		sDetail = sres.problems
	case sres.shortForm:
		sDetail = []string{"short form, no sections — structure check not applicable"}
	default:
		sDetail = []string{fmt.Sprintf("%d section(s), all declared: %s", len(sres.headers), strings.Join(sres.headers, " / "))}
	}
	checks = append(checks, Check{Name: "sections map 1:1 to reader questions", Pass: len(sres.problems) == 0, Detail: sDetail})

	// 5. persuasion architecture when informing
	intent := spec.Intent
	if intent == "" {
		intent = "inform"
	}
	var pHits []string
	if intent == "inform" {
		for _, re := range persuasion {
			if m := re.FindString(text); m != "" {
				pHits = append(pHits, m)
			}
		}
	}
	var pDetail []string
	if len(pHits) == 0 {
		pDetail = []string{"reads as a report"}
	} else {
		for _, h := range pHits {
			pDetail = append(pDetail, fmt.Sprintf("\"%s\" argues instead of reporting", h))
		}
	}
	checks = append(checks, Check{Name: fmt.Sprintf("no persuasion architecture (intent: %s)", intent), Pass: len(pHits) == 0, Detail: pDetail})

	failed := 0
	for _, c := range checks {
		if !c.Pass {
			failed++
		}
	}
	return Result{Pass: failed == 0, Reader: spec.Reader, Text: text, Words: wc, Checks: checks}
}
