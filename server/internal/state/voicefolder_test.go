package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"byline/internal/paths"
	"byline/internal/version"
)

// TestEnsureVoiceFolder_FolderCreationFailureIsOnePlainSentence is the
// residual finding from the gate re-review (gate-fixes-rereview.md): when
// the voice folder itself can't be created, EnsureVoiceFolder returned a
// \n-joined stack of raw OS error lines (one "mkdir ...: not a
// directory" per subfolder, since every one of them nests under the
// same unmade parent). It must return one plain sentence naming the
// folder and the reason instead.
func TestEnsureVoiceFolder_FolderCreationFailureIsOnePlainSentence(t *testing.T) {
	notADir := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(notADir, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BYLINE_HOME", filepath.Join(notADir, "byline"))
	err := EnsureVoiceFolder()
	if err == nil {
		t.Fatal("want an error when the voice folder can't be created")
	}
	msg := err.Error()
	if strings.Count(msg, "\n") > 0 {
		t.Fatalf("error is more than one line: %q", msg)
	}
	if !strings.Contains(msg, paths.Home()) {
		t.Fatalf("error doesn't name the folder: %q", msg)
	}
}

func TestInstall_CreatesVoiceFolderFromTemplates(t *testing.T) {
	home := filepath.Join(testHome(t), "fresh") // does not exist yet
	t.Setenv("BYLINE_HOME", home)
	if err := EnsureVoiceFolder(); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"profile.md", "exemplars.md", "my-rules.json", "my-stance.md", "settings.json"} {
		want(t, readFile(t, filepath.Join(home, n)), tmpl(t, n))
	}
	for _, d := range []string{"receipts", "state", "backups", "fixtures"} {
		assertDir(t, filepath.Join(home, d), 0o700)
	}
	first := firstLine(t, paths.BriefPath())
	if !strings.HasPrefix(first, "<!-- byline "+version.Version()+" | core rules ") || !strings.Contains(first, "| personal rules saved never |") {
		t.Fatalf("brief stamp = %q", first)
	}
	assertNoFile(t, paths.RealVoicePath()) // setup writes it, install must not

	backdate(t, 2*time.Hour, allFiles(t, home)...)
	backdate(t, time.Hour, paths.BriefPath())
	before := snapshotTimes(t, home)
	if err := EnsureVoiceFolder(); err != nil {
		t.Fatal(err)
	}
	want(t, snapshotTimes(t, home), before) // a second start touches nothing
}

// TestInstallTemplate_ExistingFileEvenEmptyIsNeverTouched pins
// installTemplate's documented contract (its own doc comment): a file
// that already exists before the call, even a zero-byte one, is left
// alone. gate review M9's fix (removing a file this call itself just
// created, on a write error) must not loosen this: a zero-byte file
// that predates the call is still skipped, not deleted or rewritten.
func TestInstallTemplate_ExistingFileEvenEmptyIsNeverTouched(t *testing.T) {
	home := testHome(t)
	path := filepath.Join(home, "my-rules.json")
	writeFile(t, path, "")
	if err := installTemplate("my-rules.json", path); err != nil {
		t.Fatal(err)
	}
	want(t, readFile(t, path), "")
}

func TestInstall_NeverOverwritesPersonalFiles(t *testing.T) {
	testHome(t)
	EnsureVoiceFolder()
	writeFile(t, paths.ProfilePath(), "mine")
	EnsureVoiceFolder()
	want(t, readFile(t, paths.ProfilePath()), "mine")
}

func TestInstall_RegeneratesStaleBrief(t *testing.T) {
	home := testHome(t)
	EnsureVoiceFolder()
	backdate(t, 2*time.Hour, allFiles(t, home)...)
	writeFile(t, paths.BriefPath(), "<!-- byline 0.9.0 | core rules x | personal rules saved never | generated x -->\nold")
	EnsureVoiceFolder()
	if !strings.HasPrefix(firstLine(t, paths.BriefPath()), "<!-- byline "+version.Version()+" |") {
		t.Fatal("old version stamp must regenerate")
	}

	backdate(t, time.Hour, paths.BriefPath())
	touch(t, paths.MyStancePath()) // stance now newer than the brief
	EnsureVoiceFolder()
	if mtime(t, paths.BriefPath()).Before(time.Now().Add(-time.Minute)) {
		t.Fatal("stance newer than brief must regenerate")
	}

	backdate(t, time.Hour, paths.BriefPath())
	backdate(t, 2*time.Hour, paths.MyStancePath(), paths.MyRulesPath())
	EnsureVoiceFolder()
	if mtime(t, paths.BriefPath()).After(time.Now().Add(-59 * time.Minute)) {
		t.Fatal("nothing changed, must not regenerate")
	}
}

func TestState_NewerFormatIsNeverRewritten(t *testing.T) {
	testHome(t)
	EnsureVoiceFolder()
	newer := `{"format":2,"add_rules":[],"shiny":true}`
	writeFile(t, paths.MyRulesPath(), newer)
	ensureErr := EnsureVoiceFolder()
	if _, err := LoadMyRules(); !errors.Is(err, ErrNewerFormat) {
		t.Fatalf("err = %v", err)
	}
	if _, err := UpdateMyRules(func(map[string]any) error { return nil }); !errors.Is(err, ErrNewerFormat) {
		t.Fatalf("err = %v", err)
	}
	want(t, readFile(t, paths.MyRulesPath()), newer)
	if ensureErr == nil || !strings.Contains(ensureErr.Error(), "newer version of Byline") || !strings.Contains(ensureErr.Error(), "; to update, run claude plugin update byline@byline in a terminal") {
		t.Fatalf("%v", ensureErr)
	}
}

func TestState_CorruptMyRulesIsNeverOverwritten(t *testing.T) {
	testHome(t)
	EnsureVoiceFolder()
	good := readFile(t, paths.BriefPath())
	writeFile(t, paths.MyRulesPath(), `{"add_rules": [`)
	ensureErr := EnsureVoiceFolder()
	var ue *UnreadableError
	if _, err := LoadMyRules(); !errors.As(err, &ue) {
		t.Fatalf("err = %v", err)
	}
	if _, err := UpdateMyRules(func(map[string]any) error { return nil }); !errors.As(err, &ue) {
		t.Fatalf("err = %v", err)
	}
	want(t, readFile(t, paths.MyRulesPath()), `{"add_rules": [`)
	want(t, readFile(t, paths.BriefPath()), good) // the last good brief stays
	if ensureErr == nil || !strings.Contains(ensureErr.Error(), "can't read your personal rules") {
		t.Fatalf("%v", ensureErr)
	}
}

// TestState_TrailingContentMakesMyRulesUnreadable reproduces gate review
// I2: json.Decoder.Decode stops at the end of the first JSON value, so a
// hand edit that closes the object too early and leaves more text after
// it was read as valid. LoadMyRules and UpdateMyRules must both treat
// trailing content as unreadable, and the file must never be rewritten.
func TestState_TrailingContentMakesMyRulesUnreadable(t *testing.T) {
	testHome(t)
	EnsureVoiceFolder()
	trailing := `{"format": 1, "add_rules": []}` + "\n" +
		`  , "add_strings": {"W1": ["my own banned phrase"]}, "known_names": ["Pat Example"]}`
	writeFile(t, paths.MyRulesPath(), trailing)
	var ue *UnreadableError
	if _, err := LoadMyRules(); !errors.As(err, &ue) {
		t.Fatalf("LoadMyRules err = %v, want *UnreadableError", err)
	}
	if _, err := UpdateMyRules(func(map[string]any) error { return nil }); !errors.As(err, &ue) {
		t.Fatalf("UpdateMyRules err = %v, want *UnreadableError", err)
	}
	want(t, readFile(t, paths.MyRulesPath()), trailing) // never rewritten
}

// TestState_UpdateMyRulesWritesThroughASymlink reproduces gate review
// M12's own scenario: my-rules.json kept in a dotfiles repo and
// symlinked into the voice folder. A save must land on the link's
// target and leave the link itself in place.
func TestState_UpdateMyRulesWritesThroughASymlink(t *testing.T) {
	home := testHome(t)
	EnsureVoiceFolder()
	real := filepath.Join(home, "dotfiles-my-rules.json")
	if err := os.Rename(paths.MyRulesPath(), real); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, paths.MyRulesPath()); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateMyRules(func(doc map[string]any) error {
		rs, _ := doc["add_rules"].([]any)
		doc["add_rules"] = append(rs, decodeUseNumber(t, ruleJSON("U11", false)))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(paths.MyRulesPath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("my-rules.json is no longer a symlink")
	}
	got := decodeUseNumber(t, readFile(t, real))
	want(t, len(got["add_rules"].([]any)), 1)
}

func TestState_UpdateMyRulesRetriesOnConcurrentWrite(t *testing.T) {
	testHome(t)
	EnsureVoiceFolder()
	calls := 0
	_, err := UpdateMyRules(func(doc map[string]any) error {
		calls++
		if calls == 1 { // another session saves between our read and our write
			writeFile(t, paths.MyRulesPath(), `{"format":1,"add_rules":[`+ruleJSON("U9", false)+`]}`)
		}
		rs, _ := doc["add_rules"].([]any)
		doc["add_rules"] = append(rs, decodeUseNumber(t, ruleJSON("U10", false)))
		return nil
	})
	if err != nil || calls != 2 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
	got := decodeUseNumber(t, readFile(t, paths.MyRulesPath()))
	want(t, len(got["add_rules"].([]any)), 2) // theirs (U9) and ours (U10)
	if _, err := os.Stat(paths.RulesSavedPath()); err != nil {
		t.Fatal("rules-saved stamp not written")
	}
}

func TestState_InvalidPersonalRulesAreNotSaved(t *testing.T) {
	testHome(t)
	EnsureVoiceFolder()
	before := readFile(t, paths.MyRulesPath())
	_, err := UpdateMyRules(func(doc map[string]any) error { doc["add_rules"] = "not a list"; return nil })
	if err == nil {
		t.Fatal("rules.Load must reject it before writing")
	}
	want(t, readFile(t, paths.MyRulesPath()), before)
}

func TestState_BackupsPrunedTo20(t *testing.T) {
	testHome(t)
	EnsureVoiceFolder()
	writeFile(t, filepath.Join(paths.BackupsDir(), "notes.txt"), "not ours") // never pruned
	for i := 0; i < 25; i++ {
		WriteVoiceFile(paths.ProfilePath(), []byte(fmt.Sprint(i)))
	}
	want(t, len(glob(t, filepath.Join(paths.BackupsDir(), "profile.md.*.bak"))), 20)
	want(t, readFile(t, filepath.Join(paths.BackupsDir(), "notes.txt")), "not ours")
}

// TestState_MissingFormatFieldReadsAsCurrent documents the Task 2 ruling:
// Byline has never shipped a format older than 1, so a file with no
// format field at all (never produced by this version, but tolerated) is
// read as format 1, not upgraded or backed up.
func TestState_MissingFormatFieldReadsAsCurrent(t *testing.T) {
	testHome(t)
	EnsureVoiceFolder()
	noFormat := `{"add_rules":[],"add_strings":{},"switch_off":{},"replace_examples":{},"voice_target":{}}`
	writeFile(t, paths.MyRulesPath(), noFormat)
	doc, err := LoadMyRules()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := doc["format"]; ok {
		t.Fatalf("doc has a format key it was never given: %v", doc)
	}
	if err := EnsureVoiceFolder(); err != nil {
		t.Fatal(err)
	}
	want(t, readFile(t, paths.MyRulesPath()), noFormat) // never upgraded, never backed up
	want(t, len(glob(t, filepath.Join(paths.BackupsDir(), "my-rules.json.*.bak"))), 0)
}
