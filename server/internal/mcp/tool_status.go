package mcp

import (
	"fmt"
	"os"
	"strings"

	"byline/internal/paths"
	"byline/internal/state"
	"byline/internal/version"
)

// setupMarker is the template profile.md's first line before setup ever
// runs (embed/templates/profile.md).
const setupMarker = "<!-- byline template: setup replaces this file -->"

// exists reports whether path can be stat'd.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// firstLineOf returns path's first line (without its trailing newline),
// "" on any read error.
func firstLineOf(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	s := string(b)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// setupIsDone reports whether setup has ever run: profile.md's first line
// is no longer the template marker, and the real-voice fixture holds
// something.
func setupIsDone() bool {
	if firstLineOf(paths.ProfilePath()) == setupMarker {
		return false
	}
	b, err := os.ReadFile(paths.RealVoicePath())
	return err == nil && strings.TrimSpace(string(b)) != ""
}

// statusProblems reports a fresh EnsureVoiceFolder error -- never the
// server's start-time one, which would otherwise keep naming a problem
// forever after the person fixes it (gate finding M3) -- plus the
// sentence for a fresh LoadMyRules and LoadSettings error, without
// duplicates.
func statusProblems() []string {
	lines := []string{}
	seen := map[string]bool{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		lines = append(lines, s)
	}
	if err := state.EnsureVoiceFolder(); err != nil {
		for _, l := range strings.Split(err.Error(), "\n") {
			add(l)
		}
	}
	if _, err := state.LoadMyRules(); err != nil {
		add(friendlyStateError(err).Error())
	}
	if _, err := state.LoadSettings(); err != nil {
		add(friendlySettingsError(err).Error())
	}
	return lines
}

// statusTool's ss parameter is unused: gate finding M3 replaced the
// server's cached start-time error with a fresh EnsureVoiceFolder call on
// every status call. It stays in the signature so buildTools, which
// NewServer and RunToolMode both call with their own startState, needs no
// change.
func statusTool(ss *startState) Tool {
	return Tool{
		Name:        "byline_status",
		Description: "Reports whether Byline is set up and current, and anything wrong with its files.",
		Handler: func(args map[string]any) (any, error) {
			var notice any
			if n := updateNotice(); n != "" {
				notice = n
			}
			return map[string]any{
				"version":      version.Version(),
				"voice_folder": paths.Home(),
				"setup_done":   setupIsDone(),
				"files": map[string]any{
					"brief":      exists(paths.BriefPath()),
					"profile":    exists(paths.ProfilePath()),
					"exemplars":  exists(paths.ExemplarsPath()),
					"real_voice": exists(paths.RealVoicePath()),
				},
				"problems":      statusProblems(),
				"update_notice": notice,
				"brief_stamp":   firstLineOf(paths.BriefPath()),
			}, nil
		},
	}
}

func briefTool() Tool {
	return Tool{
		Name:        "byline_brief",
		Description: "Returns Byline's composition brief, writing profile and sample messages.",
		Handler: func(args map[string]any) (any, error) {
			brief, err := readRequiredVoiceFile(paths.BriefPath(), "brief")
			if err != nil {
				return nil, err
			}
			profile, err := readRequiredVoiceFile(paths.ProfilePath(), "profile")
			if err != nil {
				return nil, err
			}
			exemplars, err := readRequiredVoiceFile(paths.ExemplarsPath(), "sample emails")
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"brief":     brief,
				"profile":   profile,
				"exemplars": exemplars,
				"paths": map[string]any{
					"brief":     paths.BriefPath(),
					"profile":   paths.ProfilePath(),
					"exemplars": paths.ExemplarsPath(),
				},
			}, nil
		},
	}
}

// readRequiredVoiceFile reads path, turning a missing file into the plain
// sentence telling the person to restart the app (EnsureVoiceFolder
// rebuilds it on the next start).
func readRequiredVoiceFile(path, label string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("Byline's %s file is missing from %s. Quit and reopen the app so Byline can rebuild it.", label, path)
		}
		return "", err
	}
	return string(b), nil
}
