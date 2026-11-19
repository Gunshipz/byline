package state

import (
	"slices"
	"testing"
)

// TestDefaultSendTools_C1_IncludesSlackPostingSuite reproduces C1: the
// defaults covered only one connector's send tool, so a connector's other
// posting, scheduling, editing and draft tools got no ask and no
// structure check. Each registered connector's full send suite must be
// covered, not just one tool from it.
func TestDefaultSendTools_C1_IncludesSlackPostingSuite(t *testing.T) {
	for _, want := range []string{
		"mcp__claude_ai_Microsoft_365__outlook_send_mail",
		"mcp__claude_ai_Microsoft_365__outlook_forward_mail",
		"mcp__claude_ai_Microsoft_365__outlook_send_draft",
		"mcp__claude_ai_Microsoft_365__outlook_create_draft",
		"mcp__claude_ai_Microsoft_365__outlook_create_reply_draft",
		"mcp__claude_ai_Microsoft_365__outlook_create_reply_all_draft",
		"mcp__claude_ai_Microsoft_365__outlook_update_draft",
		"mcp__claude_ai_Microsoft_365__teams_send_chat_message",
		"mcp__claude_ai_Microsoft_365__teams_send_channel_message",
		"mcp__claude_ai_Microsoft_365__teams_reply_channel_message",
		"mcp__gmail__send_email",
		"mcp__gmail__draft_email",
		"mcp__workspace-mcp__send_gmail_message",
		"mcp__workspace-mcp__draft_gmail_message",
		"mcp__slack__conversations_add_message",
	} {
		if !slices.Contains(DefaultSendTools, want) {
			t.Errorf("DefaultSendTools is missing %q", want)
		}
	}
}
