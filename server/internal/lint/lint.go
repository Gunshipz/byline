// Package lint ports lint.mjs: Byline's deterministic string and
// structural writing checks. No model calls; a clean lint is a regression
// check, not a quality gate.
package lint

import (
	"regexp"
	"sort"
	"strings"

	"byline/internal/rules"
	"byline/internal/textutil"
)

// Violation is one lint hit: a banned string or a structural pattern the
// linter found in the text.
type Violation struct {
	RuleID string `json:"rule_id"`
	Family int    `json:"family"`
	Name   string `json:"name"`
	Line   int    `json:"line"`
	Match  string `json:"match"`
	String string `json:"string"`
	Fix    string `json:"fix"`
}

// META_CUES (lint.mjs:35), verbatim: text around a match that's talking
// ABOUT the phrase, not using it, so it isn't a violation.
var metaCuesRe = regexp.MustCompile(`(?i)(say something like|says? something like|like\s*[-–:]|don'?t (want|say|use|do)|never (say|write|use|do)|stop (say|writ|us)|this pattern|you write|i flagged|banned|prohibit|rule (against|to)|violation|drop this|take (this|these) out|reeks|passed humanizer|instead of|BAD:|GOOD:)`)

// Header region: To:/Cc:/Subject:/... lines at the top of the text.
var headerRe = regexp.MustCompile(`(?m)^(\s*(To|Cc|CC|Bcc|Subject|From)\s*:.*\n?)+`)

// Structural check regexes, ported from lint.mjs:56-111.
var (
	boldLabelRe      = regexp.MustCompile(`^\s*[-*]\s+\*\*[^*]{2,40}\*\*\s*:`)
	titleCaseRe      = regexp.MustCompile(`^#{1,4}\s+([A-Z][a-z]+\s+){2,}[A-Z][a-z]+\s*$`)
	emojiRe          = regexp.MustCompile(`[\x{1F300}-\x{1FAFF}\x{2600}-\x{27BF}]`)
	noXCommaRe       = regexp.MustCompile(`(?i)\bno\s+\w+,`)
	noXEndRe         = regexp.MustCompile(`(?i)\bno\s+\w+\.?\s*$`)
	notButRe         = regexp.MustCompile(`(?i)\bnot\s+[\w\s]{3,30},?\s+but\s+(rather\s+)?[\w\s]{3,30}`)
	doubledRatherRe  = regexp.MustCompile(`(?i)\b(?:rather than|instead of)\b[^.!?]{5,90}?\b(?:i(?:'d| would)?\s+(?:rather|prefer)|instead|rather than)\b`)
	flattenNewlineRe = regexp.MustCompile(`\s*\n\s*`)
	listSkipRe       = regexp.MustCompile(`^\s*[-*\d|#>]`)
	colonPayoffRe    = regexp.MustCompile(`^([A-Z][^:.\n]{8,60}):\s+([a-z][^.\n]{15,})\.?$`)
	andOrRe          = regexp.MustCompile(`\b(?:and|or)\b`)
)

// curlyReplacer (I1) maps curly quotes to straight ones, the way Node's old
// "lint.mjs --fix" step did before linting (factlint's curlyReplacer is the
// same mapping).
var curlyReplacer = strings.NewReplacer("‘", "'", "’", "'", "“", "\"", "”", "\"")

// Run lints text against set's active rules. It is the master's lint.Run.
func Run(set *rules.Set, text string) []Violation {
	text = textutil.Normalize(text)
	text = curlyReplacer.Replace(text)
	regions := regions(text, set)
	sentences := textutil.SplitSentences(text)

	var hits []Violation
	for _, r := range set.Active() {
		if r.Autofix {
			continue
		}
		for _, s := range r.Strings {
			if strings.Contains(s, "\n") {
				continue // handled by newlineRuleHits
			}
			re := stringToRegex(s)
			for _, loc := range re.FindAllStringIndex(text, -1) {
				if inRegions(regions, loc[0]) || inMetaQuote(text, sentences, loc[0], loc[1]) {
					continue
				}
				hits = append(hits, Violation{
					RuleID: r.ID, Family: r.Family, Name: r.Name,
					Line: lineOf(text, loc[0]), Match: text[loc[0]:loc[1]],
					String: s, Fix: r.Fix,
				})
			}
		}
	}

	hits = append(hits, structuralHits(text, regions, set)...)
	hits = append(hits, newlineRuleHits(text, set)...)

	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Line != hits[j].Line {
			return hits[i].Line < hits[j].Line
		}
		return hits[i].RuleID < hits[j].RuleID
	})
	return hits
}

// lineOf is the 1-based line number of byte offset idx in text.
func lineOf(text string, idx int) int {
	return strings.Count(text[:idx], "\n") + 1
}

// stringToRegex (lint.mjs's stringToRegex, narrowed by I5), case-
// insensitive: a phrase gets \b at whichever edge (start, end, or both)
// has a word character there, so it can't match as a substring inside a
// longer word ("by design" no longer hits "reviewed by designers"). An
// edge whose character isn't a word character (punctuation, an em dash,
// a curly quote) needs no \b: the literal character already forces a
// break.
func stringToRegex(s string) *regexp.Regexp {
	e := regexp.QuoteMeta(s)
	prefix, suffix := "", ""
	if isWordByte(s[0]) {
		prefix = `\b`
	}
	if isWordByte(s[len(s)-1]) {
		suffix = `\b`
	}
	return regexp.MustCompile(`(?i)` + prefix + e + suffix)
}

// isWordByte matches Go regexp's own \w: [0-9A-Za-z_]. A multi-byte UTF-8
// rune's bytes are all >= 0x80, so checking the raw byte is safe even for
// non-ASCII phrase edges (an accented letter, an em dash, a curly quote):
// none of them are word bytes, so none gets a \b.
func isWordByte(b byte) bool {
	return b == '_' || ('0' <= b && b <= '9') || ('a' <= b && b <= 'z') || ('A' <= b && b <= 'Z')
}

// regions is signatureRegions(text) (lint.mjs:44-52): the signature block
// (from the first signature cue to the end) and the header block (To:/Cc:/
// Subject:/... lines at the top), generalized to a Set's own signature
// lines instead of one hard-coded name.
func regions(text string, set *rules.Set) [][2]int {
	var out [][2]int
	loc := firstSignatureStart(text, set)
	// Gate finding I4: nothing but `dev split` produces signature_lines for
	// anyone but the owner, so with none configured, also fall back to a
	// generic closing-line cut, the same as factlint. When signature_lines
	// is set, this is unchanged (owner parity).
	if len(set.SignatureLines) == 0 {
		if i := textutil.CutClosingSignature(text); i >= 0 && (loc < 0 || i < loc) {
			loc = i
		}
	}
	if loc >= 0 {
		out = append(out, [2]int{loc, len(text)})
	}
	if hloc := headerRe.FindStringIndex(text); hloc != nil {
		out = append(out, [2]int{hloc[0], hloc[1]})
	}
	return out
}

// dashOnlyRe recognizes signatureRegex's "--\s*$" alternative specifically,
// so firstSignatureStart can gate it (M4) without touching the other,
// unconditional cues.
var dashOnlyRe = regexp.MustCompile(`^\n\s*--\s*$`)

// firstSignatureStart is the earliest signature-region start in text: the
// earliest match of set's explicit signature lines or the Manager/Sr.
// Manager cues (unconditional), or a lone "--" separator line, gated by
// M4 — a real sign-off only when 4 or fewer short lines (6 words or
// fewer each) follow it, not a mid-document rule such as a markdown
// divider. Returns -1 when nothing qualifies.
func firstSignatureStart(text string, set *rules.Set) int {
	for _, loc := range signatureRegex(set).FindAllStringIndex(text, -1) {
		if !dashOnlyRe.MatchString(text[loc[0]:loc[1]]) || dashHasShortTail(text[loc[1]:]) {
			return loc[0]
		}
	}
	return -1
}

// dashHasShortTail (M4) reports whether tail — the text right after a "--"
// separator line — holds 4 or fewer lines, each 6 words or fewer: the
// shape of a real sign-off block, not ordinary prose that happens to
// follow a "--" divider.
func dashHasShortTail(tail string) bool {
	lines := strings.Split(strings.TrimSuffix(strings.TrimPrefix(tail, "\n"), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return true
	}
	if len(lines) > 4 {
		return false
	}
	for _, l := range lines {
		if len(strings.Fields(l)) > 6 {
			return false
		}
	}
	return true
}

// signatureRegex builds (?m)\n\s*(?:<SignatureLines>|Manager,\s|Sr\.? Manager,\s|--\s*$).
func signatureRegex(set *rules.Set) *regexp.Regexp {
	var alts []string
	for _, l := range set.SignatureLines {
		if l != "" {
			alts = append(alts, regexp.QuoteMeta(l))
		}
	}
	alts = append(alts, `Manager,\s`, `Sr\.? Manager,\s`, `--\s*$`)
	return regexp.MustCompile(`(?m)\n\s*(?:` + strings.Join(alts, "|") + `)`)
}

func inRegions(regions [][2]int, i int) bool {
	for _, r := range regions {
		if i >= r[0] && i < r[1] {
			return true
		}
	}
	return false
}

// inMetaQuote (M4) is true when the match at [start,end) sits inside a
// quoted span, or a META_CUES cue phrase appears, anywhere in the same
// sentence sentences carves text into. The earlier fixed 220/120-UTF-16-
// unit window suggested the surrounding text was talking ABOUT the
// matched phrase rather than using it, but that window was wide enough to
// span unrelated sentences: ordinary prose using a transition phrase like
// "instead of" silenced real hits on an unrelated sentence nearby.
func inMetaQuote(text string, sentences []textutil.Piece, start, _ int) bool {
	piece := sentenceContaining(text, sentences, start)
	if insideQuotedSpan(piece.Text, start-piece.Start) {
		return true
	}
	return metaCuesRe.MatchString(piece.Text)
}

// sentenceContaining returns whichever of sentences holds byte offset idx
// (the gap of separator whitespace between two pieces counts as part of
// the earlier one, which never matters: a match never lands in it).
func sentenceContaining(text string, sentences []textutil.Piece, idx int) textutil.Piece {
	for i, p := range sentences {
		end := len(text)
		if i+1 < len(sentences) {
			end = sentences[i+1].Start
		}
		if idx < end {
			return p
		}
	}
	if len(sentences) > 0 {
		return sentences[len(sentences)-1]
	}
	return textutil.Piece{Text: text, Start: 0}
}

// insideQuotedSpan (M4) reports whether position start in s sits inside an
// open straight double quote: an odd count of " before it, so it's after
// an opening quote and before that quote's close. A plain "before AND
// after" check would also catch text BETWEEN two separate quoted phrases
// in the same sentence ("X" — "Y"), which isn't quoted at all. Only
// straight quotes need checking — by the time Run calls this,
// curlyReplacer (I1) has already mapped curly quotes to straight ones.
//
// shortcut: a parity count can't tell a real quote from one half of an
// unbalanced pair (an unclosed " earlier in the sentence, from a typo or
// markup error) — it flips every later edge check for the rest of the
// sentence. Upgrade to real pair-matching (nearest unmatched open/close)
// if that starts misfiring on real drafts, not just malformed input.
func insideQuotedSpan(s string, start int) bool {
	if start < 0 || start > len(s) {
		return false
	}
	return strings.Count(s[:start], `"`)%2 == 1
}

// structuralHits ports lint.mjs's structural() (lines 56-111): per-line
// checks, then the per-sentence colon-payoff check.
func structuralHits(text string, regions [][2]int, set *rules.Set) []Violation {
	byStruct := map[string]rules.Rule{}
	for _, r := range set.Active() {
		if r.Structural != "" {
			byStruct[r.Structural] = r
		}
	}
	var hits []Violation
	push := func(key string, line int, match string) {
		r, ok := byStruct[key]
		if !ok {
			return
		}
		hits = append(hits, Violation{
			RuleID: r.ID, Family: r.Family, Name: r.Name,
			Line: line, Match: textutil.Slice16(match, 0, 70), Fix: r.Fix,
		})
	}

	lines := strings.Split(text, "\n")
	offset := 0
	for i, ln := range lines {
		lineStart := offset
		offset += len(ln) + 1
		if inRegions(regions, lineStart) {
			continue
		}
		trimmed := strings.TrimSpace(ln)
		if boldLabelRe.MatchString(ln) {
			push("bold_label_colon_list_item", i+1, trimmed)
		}
		if titleCaseRe.MatchString(ln) {
			push("title_case_heading", i+1, trimmed)
		}
		if emojiRe.MatchString(ln) {
			push("emoji", i+1, trimmed)
		}
		if len(noXCommaRe.FindAllString(ln, -1)) >= 2 && noXEndRe.MatchString(trimmed) {
			push("three_or_more_no_X", i+1, trimmed)
		}
		if m := notButRe.FindString(ln); m != "" {
			push("not_x_but_y", i+1, m)
		}
		if m := doubledRatherRe.FindString(ln); m != "" {
			push("doubled_rather", i+1, m)
		}
	}

	for _, piece := range textutil.SplitSentences(text) {
		s := strings.TrimSpace(flattenNewlineRe.ReplaceAllString(piece.Text, " "))
		if listSkipRe.MatchString(s) {
			continue
		}
		if inRegions(regions, piece.Start) {
			continue
		}
		m := colonPayoffRe.FindStringSubmatch(s)
		if m == nil {
			continue
		}
		if strings.Contains(m[2], ",") || andOrRe.MatchString(m[2]) {
			continue
		}
		push("short_clause_colon_lowercase_payoff", lineOf(text, piece.Start), s)
	}

	return hits
}

// newlineRuleHits is the generic replacement for lint.mjs:113-118's
// hard-coded "name above signature" check: for every active rule with a
// string containing "\n", build a regex per such string (its lines joined
// by \s*\n\s*, trailing commas optional, wrapped in \n\s* ... \s*\n), and
// take the earliest match across them, at most one hit per rule.
func newlineRuleHits(text string, set *rules.Set) []Violation {
	var hits []Violation
	for _, r := range set.Active() {
		var newlineStrings []string
		for _, s := range r.Strings {
			if strings.Contains(s, "\n") {
				newlineStrings = append(newlineStrings, s)
			}
		}
		if len(newlineStrings) == 0 {
			continue
		}
		bestIdx, bestString := -1, ""
		for _, s := range newlineStrings {
			loc := newlineRuleRegex(s).FindStringIndex(text)
			if loc == nil {
				continue
			}
			if bestIdx == -1 || loc[0] < bestIdx {
				bestIdx, bestString = loc[0], s
			}
		}
		if bestIdx == -1 {
			continue
		}
		hits = append(hits, Violation{
			RuleID: r.ID, Family: r.Family, Name: r.Name,
			Line:   lineOf(text, bestIdx),
			Match:  strings.ReplaceAll(newlineStrings[0], "\n", "/"),
			String: bestString, Fix: r.Fix,
		})
	}
	return hits
}

// newlineRuleRegex turns a rule string with embedded newlines into
// \n\s*<line1>\s*\n\s*<line2>...\s*\n, case-insensitive, with each line's
// trailing comma made optional.
func newlineRuleRegex(s string) *regexp.Regexp {
	lines := strings.Split(s, "\n")
	parts := make([]string, len(lines))
	for i, l := range lines {
		if strings.HasSuffix(l, ",") {
			parts[i] = regexp.QuoteMeta(strings.TrimSuffix(l, ",")) + ",?"
		} else {
			parts[i] = regexp.QuoteMeta(l)
		}
	}
	return regexp.MustCompile(`(?i)\n\s*` + strings.Join(parts, `\s*\n\s*`) + `\s*\n`)
}
