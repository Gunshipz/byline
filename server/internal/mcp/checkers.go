package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"byline/internal/draftcheck"
	"byline/internal/factlint"
	"byline/internal/lint"
	"byline/internal/paths"
	"byline/internal/rules"
	"byline/internal/state"
	"byline/internal/version"
	"byline/internal/voice"
)

// currentRules is rules.LoadPersonalFile, the one entry point every
// checker tool shares with the CLI (gate finding: this duplicated
// server/cmd/byline-mcp/main.go's loadSet). warning is a plain one-line
// sentence, non-empty only when my-rules.json didn't parse and Byline
// fell back to core rules only; the caller folds it into its own result
// with withWarning, since stdout and stderr belong to the MCP transport,
// not to a tool's result.
func currentRules() (set *rules.Set, warning string, err error) {
	return rules.LoadPersonalFile()
}

// withWarning adds warning's text to out under "warning", when warning
// isn't empty, folding the one plain sentence currentRules can carry into
// a tool's own successful result instead of a stray stderr line (the MCP
// transport owns stdout and stderr).
func withWarning(out map[string]any, warning string) map[string]any {
	if warning != "" {
		out["warning"] = warning
	}
	return out
}

// withWarningOn is withWarning for a tool whose success result isn't
// already a map (voicecheckTool's measured voice.Result). A non-empty
// warning re-encodes result as a map first; an empty warning returns
// result untouched.
func withWarningOn(result any, warning string) any {
	if warning == "" {
		return result
	}
	b, err := json.Marshal(result)
	if err != nil {
		return result
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return result
	}
	m["warning"] = warning
	return m
}

// loadRules is rules.Load, kept here so only this file imports the rules
// package's Load entry point.
func loadRules(personal []byte) (*rules.Set, error) { return rules.Load(personal) }

// runLint, runFactlint, runDraftcheck, runVoiceScore and runVoiceCalibrate
// are thin wrappers over Byline's Phase 1-2 checkers, so no other mcp
// file imports those packages.
func runLint(set *rules.Set, text string) []lint.Violation { return lint.Run(set, text) }

func runFactlint(set *rules.Set, draft, source string) []factlint.Flag {
	return factlint.Run(set, draft, source)
}

func runDraftcheck(draft string, spec []byte) (draftcheck.Result, error) {
	return draftcheck.Run(draft, spec)
}

func runVoiceScore(set *rules.Set, text string) voice.Result { return voice.Score(set, text) }

func runVoiceCalibrate(corpus string) map[string]any { return voice.Calibrate(corpus) }

// friendlyStateError turns ErrNewerFormat or a *state.UnreadableError from
// a direct LoadMyRules/LoadSettings/UpdateMyRules call into the same plain
// sentence state.EnsureVoiceFolder already uses for the same failure
// (internal/state/voicefolder.go); any other error passes through
// unchanged.
func friendlyStateError(err error) error {
	if err == nil {
		return nil
	}
	var ue *state.UnreadableError
	if errors.As(err, &ue) {
		return fmt.Errorf("Byline can't read your personal rules file at %s (%w). It hasn't changed it; fix it or copy one back from %s.",
			ue.Path, ue.Err, paths.BackupsDir())
	}
	if errors.Is(err, state.ErrNewerFormat) {
		return fmt.Errorf("Your personal rules were saved by a newer version of Byline, so this one won't change them; to update, %s, then quit and reopen the app.",
			version.UpdateStep(os.Getenv))
	}
	return err
}

// friendlySettingsError turns ErrNewerFormat from a direct LoadSettings
// call into a settings-specific sentence (gate finding M2: the shared
// personal-rules sentence named the wrong file and the wrong fix). Any
// other settings error already names settings.json on its own
// (state/settings.go's readSettingsDoc) and passes through unchanged.
func friendlySettingsError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, state.ErrNewerFormat) {
		return fmt.Errorf("Your settings were saved by a newer version of Byline, so this one won't change them; to update, %s, then quit and reopen the app.",
			version.UpdateStep(os.Getenv))
	}
	return err
}
