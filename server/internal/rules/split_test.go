package rules

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func splitInputs(t *testing.T) (*Core, *Core) {
	b, _ := os.ReadFile("testdata/split/old.json")
	old, err := ParseCore(b)
	core, err2 := LoadCore(os.DirFS("testdata/split"))
	if err != nil || err2 != nil {
		t.Fatal(err, err2)
	}
	return old, core
}

func TestSplit_Synthetic(t *testing.T) {
	old, core := splitInputs(t)
	m, notes := Split(old, core)
	got, _ := json.Marshal(m)
	want := `{"format":1,"add_rules":[{"id":"B1","family":2,"name":"Beta","ban":"b","strings":["beta"],"fix":"f","bad":"x","good":"y"}],` +
		`"add_strings":{"A1":["alpha two"]},"switch_off":{"C1":[{"string":"gamma old","reason":"Sam writes it"}]},` +
		`"replace_examples":{"A1":{"bad":"Sam's bad","good":"Sam's good"}},"voice_target":{"register":"Sam's register."}}`
	if string(got) != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	if !reflect.DeepEqual(notes, []string{"family 1: core test wording kept",
		`rule A1: new core string "alpha three" kept`, "rule D1: new core rule kept"}) {
		t.Fatal(notes)
	}
}

func TestSplit_MergeRestoresOld(t *testing.T) {
	old, core := splitInputs(t)
	m, _ := Split(old, core)
	s, err := Merge(core, m)
	var ids []string
	for _, r := range s.Active() {
		ids = append(ids, r.ID)
	}
	vt := s.VoiceTarget()
	if err != nil || !reflect.DeepEqual(ids, []string{"A1", "C1", "D1", "B1"}) ||
		!reflect.DeepEqual(ruleOf(s, "A1").Strings, []string{"alpha one", "alpha three", "alpha two"}) ||
		!reflect.DeepEqual(ruleOf(s, "C1").Strings, []string{"gamma"}) || ruleOf(s, "A1").Bad != "Sam's bad" ||
		vt["register"] != "Sam's register." || vt["asks"] != "Sam asks." {
		t.Fatal(err, ids, vt)
	}
}

func TestSplitCLI(t *testing.T) {
	dir := t.TempDir()
	stance, extras := filepath.Join(dir, "stance.md"), filepath.Join(dir, "extras.json")
	os.WriteFile(stance, []byte("## S1\n\nPropose.\n"), 0o600)
	os.WriteFile(extras, []byte(`{"signature_lines":["Jordan Sample"],"known_names":["Jordan"]}`), 0o600)
	core, out := os.DirFS("testdata/split"), filepath.Join(dir, "home")
	args := []string{"--rules", "testdata/split/old.json", "--stance", stance, "--extras", extras, "--out", out}
	var o, e bytes.Buffer
	if code := RunSplitCLI(args, core, &o, &e); code != 0 || !strings.Contains(o.String(), "add_rules: 1\n") {
		t.Fatal(code, e.String())
	}
	b, _ := os.ReadFile(filepath.Join(out, "my-rules.json"))
	m, _ := ParseMyRules(b)
	st, _ := os.ReadFile(filepath.Join(out, "my-stance.md"))
	saved, _ := os.ReadFile(filepath.Join(out, "state", "rules-saved"))
	if m.AddRules[0].ID != "B1" || m.SignatureLines[0] != "Jordan Sample" || string(st) != "## S1\n\nPropose.\n" ||
		!regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ\n?$`).Match(saved) {
		t.Fatal(m, string(st), string(saved))
	}
	if code := RunSplitCLI(args, core, &o, &e); code != 1 || !strings.Contains(e.String(), "already exists") {
		t.Fatal("must refuse to overwrite", code)
	}
}

func TestSplitCLI_VoiceBaselineFromExtras(t *testing.T) {
	dir := t.TempDir()
	stance, extras := filepath.Join(dir, "stance.md"), filepath.Join(dir, "extras.json")
	os.WriteFile(stance, []byte("## S1\n\nPropose.\n"), 0o600)
	os.WriteFile(extras, []byte(`{"signature_lines":["Jordan Sample"],"voice_baseline":{"sent_len_mean":{"lo":10,"hi":20}}}`), 0o600)
	core, out := os.DirFS("testdata/split"), filepath.Join(dir, "home")
	args := []string{"--rules", "testdata/split/old.json", "--stance", stance, "--extras", extras, "--out", out}
	var o, e bytes.Buffer
	if code := RunSplitCLI(args, core, &o, &e); code != 0 {
		t.Fatal(code, e.String())
	}
	b, _ := os.ReadFile(filepath.Join(out, "my-rules.json"))
	m, err := ParseMyRules(b)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := DecodeObject(kvValue(m.VoiceTarget, "baseline"))
	if err != nil || len(baseline) != 1 || baseline[0].Key != "sent_len_mean" {
		t.Fatalf("baseline %v %v", baseline, err)
	}
}

// kvValue returns o's value for key, or nil.
func kvValue(o Object, key string) json.RawMessage {
	for _, kv := range o {
		if kv.Key == key {
			return kv.Value
		}
	}
	return nil
}

func TestSplitCLI_MalformedVoiceBaselineRefusedNothingWritten(t *testing.T) {
	dir := t.TempDir()
	stance, extras := filepath.Join(dir, "stance.md"), filepath.Join(dir, "extras.json")
	os.WriteFile(stance, []byte("## S1\n\nPropose.\n"), 0o600)
	os.WriteFile(extras, []byte(`{"voice_baseline":{"vibes":{"lo":1,"hi":2}}}`), 0o600)
	core, out := os.DirFS("testdata/split"), filepath.Join(dir, "home")
	args := []string{"--rules", "testdata/split/old.json", "--stance", stance, "--extras", extras, "--out", out}
	var o, e bytes.Buffer
	if code := RunSplitCLI(args, core, &o, &e); code != 1 || !strings.Contains(e.String(), `unknown metric "vibes"`) {
		t.Fatal(code, e.String())
	}
	if _, err := os.Stat(filepath.Join(out, "my-rules.json")); err == nil {
		t.Fatal("my-rules.json must not be written when voice_baseline is malformed")
	}
}

func TestSplitCLI_RefusesOutInsideGitRepo(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, ".git"), 0o755)
	out := filepath.Join(root, "a", "b")
	var o, e bytes.Buffer
	code := RunSplitCLI([]string{"--rules", "testdata/split/old.json", "--stance", "testdata/split/old.json", "--out", out},
		os.DirFS("testdata/split"), &o, &e)
	if _, err := os.Stat(filepath.Join(out, "my-rules.json")); code != 1 || !strings.Contains(e.String(), "inside a git repository") || err == nil {
		t.Fatal(code, e.String())
	}
}
