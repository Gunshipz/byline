package mcp

import (
	"fmt"
	"path/filepath"
	"testing"
)

// requiredSendTool is a send tool byline_settings_save can never remove,
// because this copy's own default send_tools list always carries it
// (internal/state's DefaultSendTools).
const requiredSendTool = "mcp__slack__conversations_add_message"

// skillJSONPath is where this copy's own host records one installed
// copy of Byline's skill, numbered n, under home.
func skillJSONPath(home string, n int) string {
	return filepath.Join(home, ".claude", "plugins", "cache", "byline", "byline", fmt.Sprintf("1.0.%d", n), "skills", "byline", "skill.json")
}

// clearCopyEnv resets this copy's own environment knobs so a test starts
// from a clean state.
func clearCopyEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
}
