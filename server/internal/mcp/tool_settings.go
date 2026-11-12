package mcp

import (
	"errors"
	"fmt"

	"byline/internal/paths"
	"byline/internal/state"
)

func settingsReadTool() Tool {
	return Tool{
		Name:        "byline_settings_read",
		Description: "Reads Byline's saved settings: which tools its send checks cover, exempt channels, and extra draft phrases.",
		Handler: func(args map[string]any) (any, error) {
			s, err := state.LoadSettings()
			if err != nil {
				return nil, friendlySettingsError(err)
			}
			return map[string]any{"path": paths.SettingsPath(), "settings": s}, nil
		},
	}
}

func settingsSaveTool() Tool {
	return Tool{
		Name:        "byline_settings_save",
		Description: "Saves one or more settings: send tools, exempt channels, or extra draft phrases.",
		Args: []Arg{
			{"send_tools", "array", false},
			{"exempt_channels", "array", false},
			{"extra_draft_phrases", "array", false},
		},
		Handler: func(args map[string]any) (any, error) {
			if len(args) == 0 {
				return nil, errors.New("byline_settings_save needs at least one of: send_tools, exempt_channels, extra_draft_phrases.")
			}
			patch := map[string]any{}
			for _, key := range []string{"send_tools", "exempt_channels", "extra_draft_phrases"} {
				v, ok := args[key]
				if !ok {
					continue
				}
				if !isStringList(v) {
					return nil, fmt.Errorf("%q must be a list of strings.", key)
				}
				patch[key] = v
			}
			// SaveSettings always expands send_tools back out to include
			// every default (state/settings.go's expandSendTools), which
			// would otherwise silently mask a request that drops one; so
			// check the person's own list, before expansion, here.
			if v, ok := patch["send_tools"]; ok {
				if err := state.ValidateSettings(state.Settings{SendTools: stringsFromAny(v)}); err != nil {
					return nil, err
				}
			}
			s, backup, err := state.SaveSettings(patch)
			if err != nil {
				return nil, friendlySettingsError(err)
			}
			return map[string]any{"path": paths.SettingsPath(), "settings": s, "backup": backup}, nil
		},
	}
}

// isStringList reports whether v decoded as a JSON array of only strings.
func isStringList(v any) bool {
	list, ok := v.([]any)
	if !ok {
		return false
	}
	for _, item := range list {
		if _, ok := item.(string); !ok {
			return false
		}
	}
	return true
}

// stringsFromAny converts a []any already confirmed by isStringList into
// []string.
func stringsFromAny(v any) []string {
	list, _ := v.([]any)
	out := make([]string, len(list))
	for i, item := range list {
		out[i], _ = item.(string)
	}
	return out
}
