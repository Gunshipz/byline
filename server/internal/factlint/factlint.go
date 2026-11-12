// Package factlint is a deterministic source-tracing linter: it extracts
// checkable claims from a draft and verifies each appears in the source
// brief. It catches invented numbers, counts, quoted strings, proper
// nouns, and attributed opinions with zero model calls. It is a port of
// factlint.mjs:26-74.
//
// Scope: this catches only claims with a literal signature. Paraphrase-
// level fabrication is invisible to it.
package factlint

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"byline/internal/rules"
	"byline/internal/textutil"
)

// Flag is one source-fidelity finding: a claim in the draft that factlint
// could not verify against the source.
type Flag struct {
	Type string `json:"type"`
	Val  string `json:"val"`
}

// headerRe strips the first email-plumbing header block (To:/Cc:/Subject:
// etc.) only; factlint.mjs:29.
var headerRe = regexp.MustCompile(`(?m)^(?:[ \t]*(?:To|Cc|CC|Bcc|Subject|From)[ \t]*:.*(?:\r?\n)?)+`)

// numberRe finds numbers, percentages and money figures; factlint.mjs:40.
var numberRe = regexp.MustCompile(`\b\d+(?:\.\d+)?\s?%|\$\s?\d[\d,.]*[KMB]?|\b\d{2,}(?:,\d{3})*\b`)

// srcCountsRe finds enumeration counts asserted by the source;
// factlint.mjs:46.
var srcCountsRe = regexp.MustCompile(`(?:only\s+)?(one|two|three|four|five|six|seven|eight|nine|ten)\s+(?:actually\s+)?(?:land|of)`)

// wrongCountRe finds "<n> of <m>" counts in the draft; factlint.mjs:47.
var wrongCountRe = regexp.MustCompile(`(?i)\b(one|two|three|four|five|six|seven|eight|nine|ten)\s+of\s+(?:your\s+|the\s+)?(?:one|two|three|four|five|six|seven|eight|nine|ten|\d+)\b`)

// quoteRe finds quoted strings 6-80 characters long; factlint.mjs:55.
var quoteRe = regexp.MustCompile(`"([^"]{6,80})"`)

// properRe finds capitalized multiword phrases; factlint.mjs:62.
var properRe = regexp.MustCompile(`\b([A-Z][a-zA-Z]+(?:[ \t]+[A-Z][a-zA-Z]+){1,4})\b`)

// reactionRe finds attributed feeling/opinion/preference phrases;
// factlint.mjs:68.
var reactionRe = regexp.MustCompile(`(?i)\b(?:you|he|she|they)\s+(?:leaned toward|preferred|favored|felt|feel|may be feeling|is likely to|will likely|probably (?:thinks|feels)|would rather|seemed?)\b`)

var (
	curlyReplacer   = strings.NewReplacer("‘", "'", "’", "'", "“", "\"", "”", "\"")
	wsRe            = regexp.MustCompile(`\s+`)
	loosePunctRe    = regexp.MustCompile(`[.,;:!?]`)
	bareStripRe     = regexp.MustCompile(`[\s,$]`)
	percentSuffixRe = regexp.MustCompile(`%$`)
	trailingPunctRe = regexp.MustCompile(`[.,;:!?]+$`)
	splitWSRe       = regexp.MustCompile(`[ \t]+`)
)

// wordNum is factlint.mjs:37's W: spelled-out counts one through ten.
var wordNum = map[string]int{"one": 1, "two": 2, "three": 3, "four": 4, "five": 5,
	"six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10}

// baseStop is factlint.mjs:61's stop set, minus its two hard-coded people's
// names (the person's own known_names replace them).
var baseStop = []string{"I", "The", "This", "That", "We", "You", "If", "On", "In", "At", "For", "And", "But", "So",
	"It", "There", "Here", "What", "When", "Below", "Hi", "Hey", "Team", "Thanks", "My", "One", "Two", "Three",
	"Four", "No", "None", "Their", "His", "Her", "Our", "Both", "Those", "These", "Cc", "CC", "To", "Bcc",
	"Subject", "From", "Re", "Fwd", "Either"}

// norm is factlint.mjs:32's norm: lowercase, curly quotes to straight,
// whitespace runs to a single space.
func norm(s string) string {
	s = strings.ToLower(s)
	s = curlyReplacer.Replace(s)
	return wsRe.ReplaceAllString(s, " ")
}

// sigPattern builds the signature-region regexp for this person's
// signature lines, plus the two generic "Manager," lines factlint.mjs:30
// always recognizes.
func sigPattern(signatureLines []string) *regexp.Regexp {
	alts := make([]string, 0, len(signatureLines)+2)
	for _, l := range signatureLines {
		if l != "" {
			alts = append(alts, regexp.QuoteMeta(l))
		}
	}
	alts = append(alts, `Manager,[ \t]`, `Sr\.? Manager,[ \t]`)
	return regexp.MustCompile(`\n[ \t]*(?:` + strings.Join(alts, "|") + `)`)
}

// stopSet is baseStop plus the person's known_names.
func stopSet(knownNames []string) map[string]bool {
	m := make(map[string]bool, len(baseStop)+len(knownNames))
	for _, w := range baseStop {
		m[w] = true
	}
	for _, w := range knownNames {
		m[w] = true
	}
	return m
}

func containsInt(list []int, n int) bool {
	for _, v := range list {
		if v == n {
			return true
		}
	}
	return false
}

// joinUniqueInts formats nums as Node's [...new Set(nums)].join("/") does:
// each unique value once, in first-occurrence order.
func joinUniqueInts(nums []int) string {
	seen := map[int]bool{}
	var parts []string
	for _, n := range nums {
		if seen[n] {
			continue
		}
		seen[n] = true
		parts = append(parts, strconv.Itoa(n))
	}
	return strings.Join(parts, "/")
}

// Run checks draft against source and returns every source-fidelity flag
// it finds, in the order factlint.mjs's five checks run. set supplies this
// person's signature_lines (where the signature region starts) and
// known_names (added to the proper-noun stop list); nothing here is
// hard-coded to one person.
func Run(set *rules.Set, draft, source string) []Flag {
	d := textutil.Normalize(draft)
	src := textutil.Normalize(source)

	// Strip email plumbing, then the signature block to the end.
	if loc := headerRe.FindStringIndex(d); loc != nil {
		d = d[:loc[0]] + d[loc[1]:]
	}
	cut := len(d)
	if loc := sigPattern(set.SignatureLines).FindStringIndex(d); loc != nil {
		cut = loc[0]
	}
	// Gate finding I4: nothing but `dev split` produces signature_lines for
	// anyone but the owner, so with none configured, also fall back to a
	// generic closing-line cut. When signature_lines is set, this is
	// unchanged (owner parity).
	if len(set.SignatureLines) == 0 {
		if i := textutil.CutClosingSignature(d); i >= 0 && i < cut {
			cut = i
		}
	}
	d = d[:cut]

	S := norm(src)
	Sloose := wsRe.ReplaceAllString(loosePunctRe.ReplaceAllString(S, " "), " ")

	var findings []Flag

	// 1. Numbers, percentages, money figures must appear in the source.
	seenNum := map[string]bool{}
	for _, n := range numberRe.FindAllString(d, -1) {
		if seenNum[n] {
			continue
		}
		seenNum[n] = true
		bare := strings.ToLower(percentSuffixRe.ReplaceAllString(bareStripRe.ReplaceAllString(n, ""), ""))
		if !strings.Contains(S, bare) && !strings.Contains(S, norm(n)) {
			findings = append(findings, Flag{"UNSOURCED_NUMBER", n})
		}
	}

	// 2. Counting words tied to enumerations - flag when they contradict the source.
	var srcCounts []int
	for _, m := range srcCountsRe.FindAllStringSubmatch(S, -1) {
		srcCounts = append(srcCounts, wordNum[m[1]])
	}
	for _, m := range wrongCountRe.FindAllStringSubmatch(d, -1) {
		n := wordNum[strings.ToLower(m[1])]
		if len(srcCounts) > 0 && !containsInt(srcCounts, n) {
			findings = append(findings, Flag{"WRONG_COUNT", fmt.Sprintf("%s (source asserts %s)", m[0], joinUniqueInts(srcCounts))})
		}
	}

	// 3. Quoted strings must appear in the source (punctuation-tolerant).
	for _, m := range quoteRe.FindAllStringSubmatch(d, -1) {
		capture := m[1]
		q := strings.TrimSpace(trailingPunctRe.ReplaceAllString(norm(capture), ""))
		if !strings.Contains(Sloose, q) {
			findings = append(findings, Flag{"UNSOURCED_QUOTE", textutil.Slice16(capture, 0, 50)})
		}
	}

	// 4. Capitalized multiword proper nouns (orgs, studies, people) must appear in the source.
	stop := stopSet(set.KnownNames)
	seenProper := map[string]bool{}
	for _, p := range properRe.FindAllString(d, -1) {
		if seenProper[p] {
			continue
		}
		seenProper[p] = true
		first := splitWSRe.Split(p, 2)[0]
		if stop[first] {
			continue
		}
		if !strings.Contains(S, norm(p)) {
			findings = append(findings, Flag{"UNSOURCED_PROPER_NOUN", p})
		}
	}

	// 5. Attributing feeling/opinion/preference to another person = assumed reaction.
	for _, m := range reactionRe.FindAllString(d, -1) {
		if !strings.Contains(S, norm(m)) {
			findings = append(findings, Flag{"ASSUMED_REACTION", m})
		}
	}

	return findings
}
