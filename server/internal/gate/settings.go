package gate

import (
	"strings"

	"byline/internal/state"
)

// gateSettings is how the gate reads settings.json: through state, which
// owns defaults, format upgrades and validation. On any error (a missing
// or unparsable file, or a newer format) the gate falls back to the bare
// defaults, with no bot chats and no extra phrases — never an escape
// hatch, since DefaultSendTools is never empty.
func gateSettings() state.Settings {
	s, err := state.LoadSettings()
	if err != nil {
		return state.Settings{SendTools: state.DefaultSendTools}
	}
	return s
}

// matchSendTool reports whether name is one of sendTools, either exactly
// or as that tool loaded through a plugin: a plugin-wrapped MCP tool name
// looks like mcp__plugin_<PluginName>_<server>__<tool>, so it ends with
// "_" + the tool name's own server__tool suffix.
func matchSendTool(name string, sendTools []string) (string, bool) {
	for _, t := range sendTools {
		if name == t {
			return t, true
		}
		if strings.HasPrefix(name, "mcp__plugin_") && strings.HasSuffix(name, "_"+strings.TrimPrefix(t, "mcp__")) {
			return t, true
		}
	}
	return "", false
}
