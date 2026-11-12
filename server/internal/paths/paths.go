// Package paths names the files and folders Byline reads and writes under
// BYLINE_HOME, the voice folder. Every function reads the environment
// variable fresh on every call and creates nothing.
package paths

import (
	"os"
	"path/filepath"
)

// Home is BYLINE_HOME, made absolute and clean, or ~/.byline when it's unset
// or empty.
func Home() string {
	if v := os.Getenv("BYLINE_HOME"); v != "" {
		if abs, err := filepath.Abs(v); err == nil {
			return abs
		}
		return filepath.Clean(v)
	}
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".byline")
	}
	abs, _ := filepath.Abs(".byline")
	return abs
}

func BriefPath() string           { return filepath.Join(Home(), "brief.md") }
func ProfilePath() string         { return filepath.Join(Home(), "profile.md") }
func ExemplarsPath() string       { return filepath.Join(Home(), "exemplars.md") }
func MyRulesPath() string         { return filepath.Join(Home(), "my-rules.json") }
func MyStancePath() string        { return filepath.Join(Home(), "my-stance.md") }
func SettingsPath() string        { return filepath.Join(Home(), "settings.json") }
func RealVoicePath() string       { return filepath.Join(Home(), "fixtures", "my_real_voice.txt") }
func SuitePath() string           { return filepath.Join(Home(), "fixtures", "suite.json") }
func RulesSavedPath() string      { return filepath.Join(Home(), "state", "rules-saved") }
func ReceiptsDir() string         { return filepath.Join(Home(), "receipts") }
func StateDir() string            { return filepath.Join(Home(), "state") }
func BackupsDir() string          { return filepath.Join(Home(), "backups") }
func FailuresPath() string        { return filepath.Join(Home(), "failures.jsonl") }
func LogsDir() string             { return filepath.Join(Home(), "logs") }
func ToolCallLogPath() string     { return filepath.Join(LogsDir(), "tool-calls.jsonl") }
func GateDecisionLogPath() string { return filepath.Join(LogsDir(), "gate-decisions.jsonl") }
