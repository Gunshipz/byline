// send.go is the pre-tool check that stops a send without a fresh,
// matching structure-check receipt, and asks before every Slack post to a
// person (an exempt channel passes without asking).
package gate

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"byline/internal/draftcheck"
	"byline/internal/paths"
	"byline/internal/state"
)

// Gate numbers, unchanged from today's hooks.
const (
	gateRawWords           = 20
	gateMultiSentenceWords = 12
	minMatchWords          = 6
	coverageNeeded         = 0.9
	receiptLifetime        = 60 * time.Minute
)

// isSlackPost reports whether canonical is one of slackPostTools, this
// copy's list of chat tools that post to a person (copy_specific.go).
func isSlackPost(canonical string) bool {
	return slackPostTools[canonical]
}

// channelFields returns every channel identifier a Slack call names:
// channelId, channel_id and channel when present, and every entry of a
// channels array (C1: the bulk tools' field).
func channelFields(input map[string]any) []string {
	var out []string
	if s, ok := input["channelId"].(string); ok {
		out = append(out, s)
	}
	if s, ok := input["channel_id"].(string); ok {
		out = append(out, s)
	}
	if s, ok := input["channel"].(string); ok {
		out = append(out, s)
	}
	if arr, ok := input["channels"].([]any); ok {
		for _, e := range arr {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

// exemptChannel reports whether every channel field a Slack call names is
// on exempt. Finding M2: channelId alone used to decide the exemption,
// so a call naming a real person in channel still passed when channelId
// happened to be the bot chat. A call naming no channel at all is never
// exempt.
func exemptChannel(input map[string]any, exempt []string) bool {
	fields := channelFields(input)
	if len(fields) == 0 {
		return false
	}
	for _, f := range fields {
		if !slices.Contains(exempt, f) {
			return false
		}
	}
	return true
}

// needsStructureCheck reports whether canonical's call needs a fresh,
// matching structure-check receipt before it can go out. A call to a
// draft tool named ..._email_draft, whose operation field says what it
// does, only counts when it's actually creating or updating a body (or
// sending it outright); any other send tool needs one, except a Slack
// post to an exempt channel.
func needsStructureCheck(canonical string, input map[string]any, s state.Settings) bool {
	if strings.HasSuffix(canonical, "__email_draft") {
		op := str(input["operation"])
		if op != "create" && op != "update" {
			return false
		}
		if _, ok := input["body"].(string); ok {
			return true
		}
		if _, ok := input["bodyFilePath"].(string); ok {
			return true
		}
		if send, ok := input["send"].(bool); ok && send {
			return true
		}
		return false
	}
	if isSlackPost(canonical) && exemptChannel(input, s.ExemptChannels) {
		return false
	}
	return true
}

// collectText recursively collects every string found under a "text" key
// anywhere inside v: a Block Kit blocks array nests "text" at several
// depths (a section's own "text", a text object's {"type":"mrkdwn",
// "text":"..."}, a field or button's "text"), and this finds all of them.
func collectText(v any) []string {
	var out []string
	switch t := v.(type) {
	case map[string]any:
		if s, ok := t["text"].(string); ok {
			out = append(out, s)
		}
		for _, vv := range t {
			out = append(out, collectText(vv)...)
		}
	case []any:
		for _, vv := range t {
			out = append(out, collectText(vv)...)
		}
	}
	return out
}

// outgoingText returns the text a send tool call would actually send: the
// first of text, body, message or payload that's a string, plus every
// text field inside blocks (C1: Slack's Block Kit tools), falling back to
// the contents of bodyFilePath when none of those is present. It returns
// "" when the call carries no text, such as sending a saved draft.
func outgoingText(input map[string]any) string {
	var parts []string
	for _, k := range []string{"text", "body", "message", "payload"} {
		if s, ok := input[k].(string); ok {
			parts = append(parts, s)
			break
		}
	}
	if blocks, ok := input["blocks"]; ok {
		parts = append(parts, collectText(blocks)...)
	}
	if len(parts) > 0 {
		if text := strings.Join(parts, " "); text != "" {
			return text
		}
	}
	fp, ok := input["bodyFilePath"].(string)
	if !ok {
		return ""
	}
	b, err := os.ReadFile(fp)
	if err != nil {
		return ""
	}
	return string(b)
}

var (
	tagRe         = regexp.MustCompile(`<[^>]+>`)
	commWordRe    = regexp.MustCompile(`[A-Za-z0-9']+`)
	sentenceEndRe = regexp.MustCompile(`[.!?](?:` + jsSpace + `|$)`)
)

// rewriteLinks turns links into "url label" (I2) before tags are
// stripped; it is draftcheck.RewriteLinks, shared with the receipts.
func rewriteLinks(text string) string { return draftcheck.RewriteLinks(text) }

// looksLikeComm reports whether text reads like an actual message rather
// than a short acknowledgment or an emoji reaction: gated at 20 or more
// words, or at 2 or more sentences and 12 or more words.
func looksLikeComm(text string) bool {
	plain := tagRe.ReplaceAllString(rewriteLinks(text), " ")
	words := len(commWordRe.FindAllString(plain, -1))
	sentences := len(sentenceEndRe.FindAllString(plain, -1))
	return words >= gateRawWords || (sentences >= 2 && words >= gateMultiSentenceWords)
}

// outgoingWords returns text's content words as a set, read through
// draftcheck.MessageText the same way the receipts are, for matching
// against a structure-check receipt.
func outgoingWords(text string) map[string]bool {
	out := make(map[string]bool)
	for _, w := range draftcheck.ContentWords(draftcheck.MessageText(text)) {
		out[w] = true
	}
	return out
}

// bestCoverage compares an outgoing message's content words against every
// fresh, qualifying structure-check receipt (too old, unparsable, or
// under minMatchWords unique words are skipped), and returns the highest
// coverage found -- the share of that receipt's own words present in the
// message -- along with that same receipt's unmatched count: the
// message's content words it never saw (I1). Both are 0 and len(words)
// when no receipt qualifies at all.
func bestCoverage(words map[string]bool, now time.Time) (coverage float64, unmatched int) {
	unmatched = len(words)
	budget := extraWordBudget(len(words))
	entries, err := os.ReadDir(paths.ReceiptsDir())
	if err != nil {
		return 0, unmatched
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil || now.Sub(info.ModTime()) > receiptLifetime {
			continue
		}
		data, err := os.ReadFile(filepath.Join(paths.ReceiptsDir(), e.Name()))
		if err != nil {
			continue
		}
		var rec draftcheck.Receipt
		if json.Unmarshal(data, &rec) != nil {
			continue
		}
		unique := make(map[string]bool)
		for _, w := range rec.Words {
			unique[w] = true
		}
		if len(unique) < minMatchWords {
			continue
		}
		var hit, miss int
		for w := range unique {
			if words[w] {
				hit++
			}
		}
		for w := range words {
			if !unique[w] {
				miss++
			}
		}
		// A receipt that passes beats one that doesn't, then higher
		// coverage, then fewer unmatched words: otherwise an older
		// receipt for a shorter version hides the one that checked this copy.
		cov := float64(hit) / float64(len(unique))
		if p, bp := passes(cov, miss, budget), passes(coverage, unmatched, budget); p && !bp || p == bp && (cov > coverage || cov == coverage && miss < unmatched) {
			coverage, unmatched = cov, miss
		}
	}
	return coverage, unmatched
}

// passes reports whether a receipt's match is good enough to send on.
func passes(coverage float64, unmatched, budget int) bool {
	return coverage >= coverageNeeded && unmatched <= budget
}

// extraWordBudget is how many of the message's content words I1 lets a
// structure check have never seen: at least 8, so a short message still
// has room for a signature, or 15% of its word count for a longer one.
func extraWordBudget(wordCount int) int {
	if b := int(math.Ceil(float64(wordCount) * 0.15)); b > 8 {
		return b
	}
	return 8
}

// receiptsDirUsable makes sure the receipts folder exists, the same way
// draftcheck.WriteReceipt does, and reports whether it could: a folder
// that can't be created or read (I3) is told apart from one that simply
// has no receipts in it yet.
func receiptsDirUsable() bool {
	dir := paths.ReceiptsDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false
	}
	_, err := os.ReadDir(dir)
	return err == nil
}

// bodyFileUnreadable reports the bodyFilePath input falls back to, when
// it's set but the hook can't read it (M6) -- the one case outgoingText
// quietly treats as an empty message, even though the mail server might
// still read the file.
func bodyFileUnreadable(input map[string]any) (path string, bad bool) {
	if outgoingText(input) != "" {
		return "", false
	}
	fp, ok := input["bodyFilePath"].(string)
	if !ok {
		return "", false
	}
	if _, err := os.ReadFile(fp); err != nil {
		return fp, true
	}
	return "", false
}

// sendChecks is the Task 3.5 half of checkTool: it stops a send without a
// fresh, matching structure-check receipt, and otherwise asks before every
// Slack post to a person (an exempt channel passes without asking).
func sendChecks(canonical string, input map[string]any, s state.Settings) *hookOutput {
	if path, bad := bodyFileUnreadable(input); bad {
		return decide(canonical, "ask", "body_file_unreadable", bodyFileAskReason(path))
	}
	text := outgoingText(input)
	needs := needsStructureCheck(canonical, input, s)
	// I3: sending a saved draft with no body sends text Byline never sees,
	// so the person approves it, the same as any other send it can't check.
	// Checked before Decision 3's blanket ask below so an email_draft send
	// still gets this more specific wording.
	if needs && strings.HasSuffix(canonical, "__email_draft") && input["send"] == true && strings.TrimSpace(text) == "" {
		return decide(canonical, "ask", "draft_unseen", draftSendAskReason)
	}
	// Decision 3, 18 Nov: any send -- a tool the person added, or one of
	// Byline's own defaults -- may carry its text in a field the gate
	// doesn't read (upload_file's initialComment, a bot's own post call,
	// a saved draft sent as is), so ask rather than pass it unseen.
	// needsStructureCheck already excludes a call that sends nothing at
	// all (email_draft's own get and delete operations, a Slack post to
	// an exempt bot chat), so this doesn't ask about those.
	if needs && strings.TrimSpace(text) == "" {
		return decide(canonical, "ask", "text_unseen", unseenTextAskReason)
	}
	if needs && looksLikeComm(text) {
		if words := outgoingWords(text); len(words) >= minMatchWords {
			// I3: a receipts folder that can't be saved to or read isn't
			// "no check has passed" -- trying again can never fix that.
			if !receiptsDirUsable() {
				return decide(canonical, "ask", "receipts_unusable", receiptsFolderAskReason(paths.ReceiptsDir()))
			}
			coverage, unmatched := bestCoverage(words, time.Now())
			// I1: a receipt only covering what's already in the message
			// isn't enough -- most of the message's own words must have
			// been in the receipt too, or content added after the check
			// (including a cut kill-list item) goes out unchecked.
			if !passes(coverage, unmatched, extraWordBudget(len(words))) {
				return decide(canonical, "deny", "structure_deny", structureDenyReason(coverage, unmatched, len(words)))
			}
		}
	}
	if isSlackPost(canonical) {
		if exemptChannel(input, s.ExemptChannels) {
			return decide(canonical, "allow", "slack_exempt", slackExemptReason)
		}
		return decide(canonical, "ask", "slack_ask", slackAskReason)
	}
	return nil
}
