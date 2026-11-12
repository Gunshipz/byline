package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPaths_UnderBYLINE_HOME(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BYLINE_HOME", dir)
	for _, c := range []struct{ got, want string }{
		{Home(), dir},
		{BriefPath(), filepath.Join(dir, "brief.md")},
		{ProfilePath(), filepath.Join(dir, "profile.md")},
		{ExemplarsPath(), filepath.Join(dir, "exemplars.md")},
		{MyRulesPath(), filepath.Join(dir, "my-rules.json")},
		{MyStancePath(), filepath.Join(dir, "my-stance.md")},
		{SettingsPath(), filepath.Join(dir, "settings.json")},
		{RealVoicePath(), filepath.Join(dir, "fixtures", "my_real_voice.txt")},
		{SuitePath(), filepath.Join(dir, "fixtures", "suite.json")},
		{RulesSavedPath(), filepath.Join(dir, "state", "rules-saved")},
		{ReceiptsDir(), filepath.Join(dir, "receipts")},
		{StateDir(), filepath.Join(dir, "state")},
		{BackupsDir(), filepath.Join(dir, "backups")},
		{FailuresPath(), filepath.Join(dir, "failures.jsonl")},
		{LogsDir(), filepath.Join(dir, "logs")},
		{ToolCallLogPath(), filepath.Join(dir, "logs", "tool-calls.jsonl")},
		{GateDecisionLogPath(), filepath.Join(dir, "logs", "gate-decisions.jsonl")},
	} {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
}

func TestHome_ReadsEnvOnEveryCall(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	t.Setenv("BYLINE_HOME", a)
	first := BriefPath()
	t.Setenv("BYLINE_HOME", b)
	if first != filepath.Join(a, "brief.md") || BriefPath() != filepath.Join(b, "brief.md") {
		t.Fatal(first, BriefPath())
	}
}

func TestHome_DefaultsToDotBylineInUserHome(t *testing.T) {
	t.Setenv("BYLINE_HOME", "")
	h, _ := os.UserHomeDir()
	if got := Home(); got != filepath.Join(h, ".byline") {
		t.Fatalf("got %q", got)
	}
}

func TestHome_RelativeValueIsMadeAbsoluteAndClean(t *testing.T) {
	wd, _ := os.Getwd()
	t.Setenv("BYLINE_HOME", "rel/x/")
	if got := Home(); got != filepath.Join(wd, "rel", "x") {
		t.Fatalf("got %q", got)
	}
}
