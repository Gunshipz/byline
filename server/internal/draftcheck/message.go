package draftcheck

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"

	"byline/internal/paths"
	"byline/internal/textutil"
)

// The send gate compares an outgoing message with the copy a structure
// check passed on. Both sides go through MessageText, so the comparison
// sees only the message: no HTML markup or style block, no link syntax,
// and no signature block at the end.

var (
	htmlAnchorRe    = regexp.MustCompile(`(?is)<a\s[^>]*href=["']([^"']*)["'][^>]*>(.*?)</a>`)
	slackLabeledRe  = regexp.MustCompile(`<(https?://[^|>\s]+)\|([^>]*)>`)
	slackBareLinkRe = regexp.MustCompile(`<(https?://[^>\s]+)>`)
	hiddenBlockRe   = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style>|<script\b[^>]*>.*?</script>|<!--.*?-->`)
	lineTagRe       = regexp.MustCompile(`(?i)<(?:br|/?(?:p|div|li|tr|h[1-6]))\b[^>]*>`)
	anyTagRe        = regexp.MustCompile(`<[^>]+>`)
	entityRe        = regexp.MustCompile(`(?i)&[a-z]+;|&#\d+;`)
)

// RewriteLinks turns Slack's <url|label> and an HTML <a href="url">label</a>
// into "url label" before tags are stripped, so a link's label words count
// on both sides.
func RewriteLinks(text string) string {
	text = htmlAnchorRe.ReplaceAllString(text, "$1 $2")
	text = slackLabeledRe.ReplaceAllString(text, "$1 $2")
	return slackBareLinkRe.ReplaceAllString(text, "$1")
}

// MessageText is text as the send gate compares it: links rewritten, a
// line break for every HTML line or paragraph break, style blocks,
// comments, other tags and entities dropped, and the sign-off block cut
// from the end.
func MessageText(text string) string {
	t := RewriteLinks(textutil.Normalize(text))
	t = hiddenBlockRe.ReplaceAllString(t, " ")
	t = lineTagRe.ReplaceAllString(t, "\n")
	t = anyTagRe.ReplaceAllString(t, " ")
	t = entityRe.ReplaceAllString(t, " ")
	return cutSignature(t, savedSignatureLines())
}

// savedSignatureLines reads the person's signature_lines from their own
// rules file, or none when it can't be read.
func savedSignatureLines() []string {
	var my struct {
		SignatureLines []string `json:"signature_lines"`
	}
	if b, err := os.ReadFile(paths.MyRulesPath()); err == nil {
		_ = json.Unmarshal(b, &my)
	}
	return my.SignatureLines
}

// cutSignature drops the sign-off block from the end of text: from the
// first line that starts with one of the person's own signature lines, or
// from a closing line such as "Thanks,", provided what's left is shaped
// like a signature (see signatureShaped). Only the person's own lines
// start a block: a generic cue such as "Manager," also starts ordinary
// sentences, and would let new lines after one go out unchecked.
// shortcut: a sign-off with neither cue still counts as new words; setup
// saving the person's signature_lines covers that.
func cutSignature(text string, sigLines []string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		if startsSignature(squash(l), sigLines) && signatureShaped(lines[i:]) {
			return strings.Join(lines[:i], "\n")
		}
	}
	if i := textutil.CutClosingSignature(text); i >= 0 {
		return text[:i]
	}
	return text
}

func startsSignature(line string, sigLines []string) bool {
	for _, s := range sigLines {
		if s = squash(s); s != "" && strings.HasPrefix(line, s) {
			return true
		}
	}
	return false
}

// signatureShaped reports whether lines are a sign-off block: 5 or fewer
// non-blank lines of 12 words or fewer each, 30 words in all, so new
// content can't go out unchecked behind a line that starts with the
// person's name.
func signatureShaped(lines []string) bool {
	n, total := 0, 0
	for _, l := range lines {
		f := len(strings.Fields(l))
		if f == 0 {
			continue
		}
		n++
		total += f
		if n > 5 || f > 12 || total > 30 {
			return false
		}
	}
	return true
}

// squash trims s and collapses each run of spaces to one, so a saved
// signature line matches the same line after HTML has spread it out.
func squash(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
