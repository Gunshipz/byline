package content

import (
	"reflect"
	"testing"
)

// installAndHostsHeadings are the "## " headings references/install-and-hosts.md
// must have, in order.
var installAndHostsHeadings = []string{
	"## How Byline's tools are reached",
	"## Claude Code",
	"## Reading sent mail and Slack for setup",
	"## Updating",
	"## Supported machines",
}

// checkAllowedTools validates this copy's own allowed-tools entries in
// SKILL.md's frontmatter: the update mode's own command, exact and
// host-specific, is the one thing this copy pre-approves.
func checkAllowedTools(t *testing.T, entries []string) {
	t.Helper()
	want := []string{`  - "Bash(claude plugin update byline@byline)"`}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("allowed-tools entries = %q, want %q", entries, want)
	}
}

// ownRulePrefix is this copy's id prefix for its own last-22-rule block in
// rules/core-rules.json. A rule id is never changed or reused, so the two
// copies keep different prefixes for good.
const ownRulePrefix = "W"

// ownRuleJudgmentOnly lists this copy's own rules, by id, that carry no
// banned-string match and are judged in context instead.
var ownRuleJudgmentOnly = map[string]bool{"W1": true, "W2": true, "W4": true, "W13": true, "W19": true, "W20": true, "W21": true, "W22": true}

// ownRuleBannedPhrases lists the banned-phrase examples for this copy's own
// rules that aren't judgment-only.
var ownRuleBannedPhrases = map[string][]string{
	"W3":  {"an error was introduced", "it was agreed", "concerns were raised", "the deadline was missed", "a call was made", "it was decided", "mistakes were made"},
	"W5":  {"What happened next?", "The catch?", "So what's the fix?", "The result?", "The answer?"},
	"W6":  {"functions as", "operates as", "flaunts", "serves as", "stands as", "boasts"},
	"W7":  {"spotlighting the importance", "driving home the point", "reinforcing the case", "highlighting the need", "underscoring the"},
	"W8":  {"counted, not guessed", "checked, not assumed", "recorded rather than recalled"},
	"W9":  {"time for the work that counts", "focus on bigger things"},
	"W10": {"opens the door to", "clears the way for", "will unlock"},
	"W11": {"No exceptions.", "That's the whole idea.", "Nothing stays the same after that."},
	"W12": {"not merely", "not simply", "not only"},
	"W14": {"To wrap up,", "All told,", "Taking stock,", "Putting it together,", "Bottom line,", "In summary,", "In conclusion,", "To sum up,"},
	"W15": {"without delay", "right away", "ASAP"},
	"W16": {"ecstatic", "elated", "overjoyed", "stoked to announce", "thrilled", "excited to announce"},
	"W17": {"watershed moment", "paradigm-shifting", "monumental shift", "paves the way for", "lays the groundwork for", "game-changer", "game-changing", "transformative", "setting the stage for"},
	"W18": {"industry-leading", "next-generation", "top-tier", "pioneering", "first-of-its-kind", "world-class", "cutting-edge", "groundbreaking", "state-of-the-art"},
}
