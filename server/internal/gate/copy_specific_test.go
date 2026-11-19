// copy_specific_test.go holds the tool names and prompts the shared gate
// tests use in this copy, so the tests themselves read the same in every
// copy.
package gate

const (
	toolEmailDraft  = "mcp__claude_ai_Microsoft_365__outlook_create_draft"
	toolEmailSend   = "mcp__claude_ai_Microsoft_365__outlook_send_mail"
	toolEmailReply  = "mcp__claude_ai_Microsoft_365__outlook_create_reply_draft"
	toolSlackPost   = "mcp__slack__conversations_add_message"
	toolSlackBlocks = "mcp__slack__conversations_add_message"
	toolSlackSearch = "mcp__slack__conversations_search_messages"
	// toolDraftOps is a draft tool whose calls carry an operation field
	// (create, update, get, delete) and a send flag. None of this copy's
	// default send tools works that way, so this stands in for one a
	// person adds.
	toolDraftOps = "mcp__mail__email_draft"
	// toolDraftAction is a draft tool whose calls carry an action field.
	toolDraftAction = "mcp__gmail__draft_email"
	// toolSendDraft, called with sendDraftInput, sends a saved draft as is.
	toolSendDraft = "mcp__claude_ai_Microsoft_365__outlook_send_draft"
	// toolAddedUpload is a send tool a person might add whose text sits in
	// a field the gate doesn't read.
	toolAddedUpload = "mcp__slack__files_upload"
	// bylineCommand is Byline's slash command as this copy's host names it.
	bylineCommand = "/byline:byline"
)

// sendDraftInput is toolSendDraft's input for sending a saved draft with
// no text of its own.
var sendDraftInput = map[string]any{"draftId": "d1"}

// silentDraftCall is a toolEmailDraft input the gate lets through
// without a word.
var silentDraftCall = map[string]any{"body": "Thanks, done."}

// readTools are MCP tools that read rather than send.
var readTools = []string{"mcp__slack__conversations_search_messages", "mcp__claude_ai_Microsoft_365__outlook_read_mail",
	"mcp__plugin_SomePlugin_slack__conversations_history"}

// copyDraftPrompts arm the gate through copyDraftPatterns. This copy has
// none.
var copyDraftPrompts []string

// copyFormalDocPrompts skip the gate through formalDocStyle.
var copyFormalDocPrompts = []string{"draft a post-mortem follow-up", "write the design doc for the importer"}
