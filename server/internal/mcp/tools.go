package mcp

// Tools returns every tool Byline exposes over MCP, in order: byline_status,
// byline_brief, byline_lint, byline_factlint, byline_draftcheck,
// byline_voicecheck, byline_flag, byline_calibrate, byline_setup_save,
// byline_settings_read, byline_settings_save, plus any extra tools a
// copy's own file adds. internal/mcp/classify_test.go classifies every
// name this returns as read-only or writing, exactly once.
func Tools() []Tool { return buildTools(&startState{}) }

// extraTools, if set, returns tools a copy's own file adds beyond the
// ones built in here. nil by default; a copy's own file sets it,
// typically from init().
var extraTools func() []Tool

// buildTools is Tools()'s testable core: ss carries one server's start
// state into byline_status, so it can report what EnsureVoiceFolder found
// at session start.
func buildTools(ss *startState) []Tool {
	tools := []Tool{
		statusTool(ss),
		briefTool(),
		lintTool(),
		factlintTool(),
		draftcheckTool(),
		voicecheckTool(),
		flagTool(),
		calibrateTool(),
		setupSaveTool(),
		settingsReadTool(),
		settingsSaveTool(),
	}
	if extraTools != nil {
		tools = append(tools, extraTools()...)
	}
	return tools
}
