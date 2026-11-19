// copy_specific.go is this copy's own data for the gate: the chat tools
// that count as a post to a person, the draft triggers that name this
// copy's own writing skills, and the formal document types left to their
// own writing process. The shared gate files read these by name.
package gate

import "regexp"

// slackPostTools are the canonical names the ask and exemption checks
// treat as a chat post to a person: Slack, and Teams through the
// Microsoft 365 connector.
var slackPostTools = map[string]bool{
	"mcp__slack__conversations_add_message":                     true,
	"mcp__claude_ai_Microsoft_365__teams_send_chat_message":     true,
	"mcp__claude_ai_Microsoft_365__teams_send_channel_message":  true,
	"mcp__claude_ai_Microsoft_365__teams_reply_channel_message": true,
}

// copyDraftPatterns are the draft triggers that name this copy's own
// writing skills, tried after the shared draftPatterns. This copy has
// none.
var copyDraftPatterns []*regexp.Regexp

// formalDocStyle names long formal document types: a draft request naming
// one of these skips the gate, because those documents get their own
// writing process, not Byline's.
var formalDocStyle = regexp.MustCompile(`(?i)\b(6[- ]?pager|six[- ]?pager|pr[- ]?faq|narrative|post[- ]?mortem|tech design|design doc|hld|lld)\b`)
