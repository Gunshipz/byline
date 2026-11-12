// Package gate is the hook that arms Byline's send checks: it decides
// whether a user turn is a draft request, whether a turn came from a
// machine rather than the person, and later (Task 3.2 on) what to do
// about it.
package gate

import (
	"regexp"
	"strings"
)

// jsSpace matches whitespace the way JavaScript's regex \s does, which the
// old hooks relied on. Task 3.5 also uses it.
const jsSpace = `[\s\v\p{Z}\x{FEFF}]`

// draftPatterns are the phrasings that arm the gate, in the order they're
// tried. Pattern 6 ("the team") deliberately carries no personal or team
// names: a bare "tell <someone>" never arms by default; only
// settings.json's extra_draft_phrases can add a name.
var draftPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(draft|write|compose|put together|whip up|create|make|prepare|craft|build|come up with|help me (?:with|write|draft|compose|put together)|outline)\b[^.?!\n]{0,40}\b(message|e-?mail|reply|response|follow[- ]?up|post|note|memo|slack|dm|update|announcement|shoutout|shout[- ]?out|outreach|intro|introduction|blurb)\b`),
	regexp.MustCompile(`(?i)\b(reply|respond|follow[- ]?up)\s+(to|with)\b`),
	regexp.MustCompile(`(?i)\b(cold\s+)?(outreach|reach(?:ing)?\s+out)\b`),
	regexp.MustCompile(`(?i)\b(send|shoot|fire off|post|ping|shoot over)\b[^.?!\n]{0,20}\b(slack|message|dm|email|e-?mail|note|reply|memo|announcement)\b`),
	regexp.MustCompile(`(?i)\b(slack|message|dm|email|e-?mail|note|memo)\b\s+(to|@|for)\b`),
	regexp.MustCompile(`(?i)\b(send|post|tell|message|slack|email)\b\s+(the\s+team)\b`),
	regexp.MustCompile(`(?i)\b(post|publish)\s+(in|to|on)\b\s+#`),
	regexp.MustCompile(`(?i)\bweekly update\b`),
	regexp.MustCompile(`(?i)\bstandup\b`),
	regexp.MustCompile(`(?i)\b1[- ]?on[- ]?1\s+prep\b`),
	regexp.MustCompile(`(?i)\bprep\b[^.?!\n]{0,30}\b(meeting|1[- ]?on[- ]?1|1:1)\b`),
}

// formalDocStyle names long formal document types: a draft request naming
// one of these skips the gate, because those documents get their own
// writing process, not Byline's.
var formalDocStyle = regexp.MustCompile(`(?i)\b(6[- ]?pager|six[- ]?pager|pr[- ]?faq|narrative|post[- ]?mortem|tech design|design doc|hld|lld)\b`)

// pipelineMeta names Byline's own machinery: a prompt about the hook,
// settings or the gate itself isn't a request to draft something, so it
// skips the gate too.
var pipelineMeta = regexp.MustCompile(`(?i)\b(hook|hooks|settings\.json|pre-?tool-?use|post-?tool-?use|userpromptsubmit|gate|sentinel|matcher|enforce\w*|programmatic\w*)\b`)

// collapseSpace lowercases s and collapses every run of whitespace to a
// single space, so an extra phrase matches regardless of case or spacing.
func collapseSpace(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// detectDraftIntent reports whether prompt is a request Byline should gate:
// pipelineMeta or formalDocStyle skip it first, then draftPatterns, then
// extraPhrases (settings.json's extra_draft_phrases), checked as a
// whitespace- and case-insensitive substring.
func detectDraftIntent(prompt string, extraPhrases []string) bool {
	if prompt == "" {
		return false
	}
	if pipelineMeta.MatchString(prompt) || formalDocStyle.MatchString(prompt) {
		return false
	}
	for _, re := range draftPatterns {
		if re.MatchString(prompt) {
			return true
		}
	}
	collapsed := collapseSpace(prompt)
	for _, phrase := range extraPhrases {
		p := collapseSpace(phrase)
		if p == "" {
			continue
		}
		if strings.Contains(collapsed, p) {
			return true
		}
	}
	return false
}

var (
	systemReminderRe = regexp.MustCompile(`^` + jsSpace + `*<system-reminder>`)
	agentMessageRe   = regexp.MustCompile(`<agent-message\b`)
)

// isMachineTurn reports whether prompt looks like it came from Claude Code
// or another agent rather than the person at the keyboard, checked over
// only the first 400 characters (runes, not bytes).
func isMachineTurn(prompt string) bool {
	head := []rune(prompt)
	if len(head) > 400 {
		head = head[:400]
	}
	h := string(head)
	return systemReminderRe.MatchString(h) ||
		strings.Contains(h, "[SYSTEM NOTIFICATION") ||
		strings.Contains(h, "<task-notification>") ||
		agentMessageRe.MatchString(h) ||
		strings.Contains(h, "[Subagent hand-back]")
}
