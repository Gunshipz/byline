package mcp

// Tools returns every tool Byline exposes over MCP, in order: byline_status,
// byline_brief, byline_lint, byline_factlint, byline_draftcheck,
// byline_voicecheck, byline_flag, byline_calibrate, byline_setup_save,
// byline_settings_read, byline_settings_save. internal/mcp/classify_test.go
// classifies every name this returns as read-only or writing, exactly once.
func Tools() []Tool { return buildTools(&startState{}) }

// buildTools is Tools()'s testable core: ss carries one server's start
// state into byline_status, so it can report what EnsureVoiceFolder found
// at session start.
func buildTools(ss *startState) []Tool {
	return []Tool{
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
}
