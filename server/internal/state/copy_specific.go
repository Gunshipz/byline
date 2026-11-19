package state

// DefaultSendTools are the tools Byline's send checks always cover; no
// settings edit can remove one. Each name is one a connector or server
// publishes: Claude's Microsoft 365 connector (Outlook and Teams), the
// most-used open-source Gmail and Google Workspace servers, and the
// most-used open-source Slack server, under the server names their own
// instructions register. Anything else, including connectors whose tool
// names aren't published, is added through settings.
var DefaultSendTools = []string{
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
}
