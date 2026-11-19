package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"byline/internal/mcp"
	"byline/internal/paths"
	"byline/internal/version"
)

// initLine is internal/mcp's initLine (server_test.go), copied here so this
// package's subprocess tests don't need to reach into another package's
// test file.
const initLine = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "byline-mcp")
	if err != nil {
		panic(err)
	}
	os.Setenv("BYLINE_HOME", dir)
	os.Exit(m.Run())
}

func want(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func assertFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected a file at %s: %v", path, err)
	}
}

func decodeUseNumber(t *testing.T, s string) map[string]any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatal(err)
	}
	return m
}

// jsonPath decodes line as JSON-RPC (numbers kept) and walks a dotted path
// of object keys into it.
func jsonPath(t *testing.T, line, path string) any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(line))
	dec.UseNumber()
	var cur any
	if err := dec.Decode(&cur); err != nil {
		t.Fatal(err)
	}
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			t.Fatalf("not an object at %q in %s", part, line)
		}
		cur, ok = m[part]
		if !ok {
			t.Fatalf("missing %q in %s", part, line)
		}
	}
	return cur
}

// jsonText decodes line as a tools/call reply and returns its result's
// text (a tool's own JSON result) decoded as an object.
func jsonText(t *testing.T, line string) map[string]any {
	t.Helper()
	content, _ := jsonPath(t, line, "result.content").([]any)
	if len(content) == 0 {
		t.Fatalf("no content in %s", line)
	}
	first, _ := content[0].(map[string]any)
	text, _ := first["text"].(string)
	return decodeUseNumber(t, text)
}

var builtBinary struct {
	once sync.Once
	path string
	err  error
}

// buildBinary builds this package into a byline-mcp binary once per test
// run, and returns its path.
func buildBinary(t *testing.T) string {
	t.Helper()
	builtBinary.once.Do(func() {
		dir, err := os.MkdirTemp("", "byline-mcp-bin")
		if err != nil {
			builtBinary.err = err
			return
		}
		path := filepath.Join(dir, "byline-mcp")
		cmd := exec.Command("go", "build", "-o", path, ".")
		out, err := cmd.CombinedOutput()
		if err != nil {
			builtBinary.err = fmt.Errorf("go build: %w\n%s", err, out)
			return
		}
		builtBinary.path = path
	})
	if builtBinary.err != nil {
		t.Fatal(builtBinary.err)
	}
	return builtBinary.path
}

// exitCode runs cmd and returns its exit code (-1 if it couldn't even
// start).
func exitCode(cmd *exec.Cmd) int {
	err := cmd.Run()
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return -1
}

func TestMain_ServesOverStdio(t *testing.T) {
	bin := buildBinary(t)
	home := filepath.Join(t.TempDir(), "byline")
	cmd := exec.Command(bin)
	cmd.Env = append(append(os.Environ(), "BYLINE_HOME="+home, "HOME="+t.TempDir()), extraTestEnv(true)...)
	cmd.Stdin = strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"byline_status","arguments":{}}}`}, "\n") + "\n")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("exit: %v\n%s", err, stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	want(t, len(lines), 3)
	want(t, jsonPath(t, lines[0], "result.serverInfo.name"), "byline-mcp")
	want(t, jsonPath(t, lines[0], "result.serverInfo.version"), version.Version())
	want(t, len(jsonPath(t, lines[1], "result.tools").([]any)), len(mcp.Tools()))
	status := jsonText(t, lines[2]) // the tool's result JSON
	want(t, status["voice_folder"], home)
	assertFile(t, filepath.Join(home, "brief.md"))
}

func TestMain_ToolModeDispatch(t *testing.T) {
	bin := buildBinary(t)
	cmd := exec.Command(bin, "tool", "byline_status")
	cmd.Stdin = strings.NewReader("{}")
	cmd.Env = append(append(os.Environ(), "BYLINE_HOME="+filepath.Join(t.TempDir(), "byline"), "HOME="+t.TempDir()), extraTestEnv(false)...)
	out, err := cmd.Output()
	if err != nil || decodeUseNumber(t, string(out))["version"] != version.Version() {
		t.Fatalf("%v %q", err, out)
	}
	want(t, exitCode(exec.Command(bin, "tool")), 2)
}

// Gate finding M14: isTerminal is the predicate deciding whether a bare
// `byline-mcp` with no arguments prints the mode list before serving. A
// pipe or a regular file (every test's and every real launcher's stdin)
// must read as false; a pty would read as true, but the standard library
// has no way to open one to test that branch directly.
func TestIsTerminal_FalseForPipeAndRegularFile(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if isTerminal(r) {
		t.Fatal("a pipe read as a terminal")
	}

	f, err := os.CreateTemp("", "byline-mcp-isterminal")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	defer os.Remove(f.Name())
	if isTerminal(f) {
		t.Fatal("a regular file read as a terminal")
	}
}

// TestMain_NoArgsWithPipedStdinPrintsNoModeListToStderr is the subprocess
// half of M14: every real caller (and this test) pipes stdin, so startup
// must stay silent on stderr.
func TestMain_NoArgsWithPipedStdinPrintsNoModeListToStderr(t *testing.T) {
	bin := buildBinary(t)
	cmd := exec.Command(bin)
	cmd.Env = append(append(os.Environ(), "BYLINE_HOME="+filepath.Join(t.TempDir(), "byline"), "HOME="+t.TempDir()), extraTestEnv(false)...)
	cmd.Stdin = strings.NewReader(initLine + "\n")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("exit: %v\n%s", err, stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr %q should be empty for piped stdin", stderr.String())
	}
}

func TestVersionMode(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"version"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	if got := out.String(); got != "byline-mcp "+version.Version()+"\n" {
		t.Fatalf("stdout %q", got)
	}
}

func TestUnknownModeExits2(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"nope"}, &out, &errb); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

// Gate finding M10: "help" and an unknown mode print the list of modes
// instead of a bare "unknown mode".
func TestHelpPrintsModeList(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"help"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, want 0: %s", code, errb.String())
	}
	for _, mode := range []string{"version", "generate", "draftcheck", "factlint", "lint", "voicecheck"} {
		if !strings.Contains(out.String(), mode) {
			t.Fatalf("help output %q doesn't name mode %q", out.String(), mode)
		}
	}
}

func TestUnknownModePrintsModeList(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"nope"}, &out, &errb); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if errb.String() == "unknown mode\n" || !strings.Contains(errb.String(), "generate") {
		t.Fatalf("stderr %q doesn't list the modes", errb.String())
	}
}

// Gate finding M10: generate takes no arguments; extra ones are rejected
// with a plain message, not folded into the generic "unknown mode" path.
func TestGenerateWithExtraArgsRejected(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"generate", "--force"}, &out, &errb); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if strings.Contains(errb.String(), "unknown mode") || !strings.Contains(errb.String(), "generate") {
		t.Fatalf("stderr %q should name generate, not fall into unknown mode", errb.String())
	}
}

func TestGenerateMode(t *testing.T) {
	t.Setenv("BYLINE_HOME", t.TempDir())
	var out, errb bytes.Buffer
	if code := run([]string{"generate"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	b, _ := os.ReadFile(paths.BriefPath())
	if out.String() != paths.BriefPath()+"\n" || !strings.HasPrefix(string(b), "<!-- byline "+version.Version()+" | core rules ") {
		t.Fatal(out.String(), string(b))
	}
}

func TestLoadSet_MissingPersonalIsCoreOnly(t *testing.T) {
	t.Setenv("BYLINE_HOME", t.TempDir())
	var errb bytes.Buffer
	s, err := loadSet(&errb)
	if err != nil || s == nil || errb.String() != "" {
		t.Fatal(err, s, errb.String())
	}
}

// Gate finding M2: a broken personal-rules file must not stop the
// checkers; it falls back to core rules only, the way generate does, and
// the user sees a plain warning, not a raw Go JSON error.
func TestLoadSet_BrokenPersonalFallsBackToCoreOnly(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BYLINE_HOME", dir)
	if err := os.WriteFile(paths.MyRulesPath(), []byte(`{"format": "not-a-number"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var errb bytes.Buffer
	s, err := loadSet(&errb)
	if err != nil || s == nil {
		t.Fatal(err, s)
	}
	if !strings.Contains(errb.String(), "my-rules.json") {
		t.Fatalf("warning doesn't name the file: %q", errb.String())
	}
	if strings.Contains(errb.String(), "cannot unmarshal") || strings.Contains(errb.String(), "json:") {
		t.Fatalf("warning leaks a raw Go JSON error: %q", errb.String())
	}
}

// Gate finding M2: a UTF-8 BOM in my-rules.json (PowerShell's -Encoding
// UTF8 writes one) must not be treated as a broken file.
func TestLoadSet_StripsLeadingBOM(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BYLINE_HOME", dir)
	bom := append([]byte("\xef\xbb\xbf"), []byte(`{"format": 1, "known_names": ["Jordan"]}`)...)
	if err := os.WriteFile(paths.MyRulesPath(), bom, 0o600); err != nil {
		t.Fatal(err)
	}
	var errb bytes.Buffer
	s, err := loadSet(&errb)
	if err != nil || s == nil || errb.String() != "" {
		t.Fatal(err, s, errb.String())
	}
	if len(s.KnownNames) != 1 || s.KnownNames[0] != "Jordan" {
		t.Fatalf("BOM blocked the personal file from loading: %+v", s.KnownNames)
	}
}

// Gate finding M2: with the personal file broken, factlint, lint and
// voicecheck keep checking (core rules only) instead of exiting 2.
func TestBrokenPersonalRulesDoesNotStopCheckers(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BYLINE_HOME", dir)
	if err := os.WriteFile(paths.MyRulesPath(), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	draft := filepath.Join(dir, "draft.txt")
	src := filepath.Join(dir, "src.txt")
	os.WriteFile(draft, []byte("Nothing notable here.\n"), 0o600)
	os.WriteFile(src, []byte("Nothing notable here.\n"), 0o600)
	var out, errb bytes.Buffer
	if code := run([]string{"factlint", draft, src}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, want 0: %s / %s", code, out.String(), errb.String())
	}
}

// TestHookMode_ExitsZeroAlways runs the real binary as a subprocess: "hook"
// calls os.Exit(0) directly, so calling run() in-process would kill the
// test itself.
func TestHookMode_ExitsZeroAlways(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "byline-mcp")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	home := filepath.Join(t.TempDir(), ".byline")
	for _, c := range []struct {
		args        []string
		stdin, want string
	}{
		{[]string{"hook", "pre-tool"}, "not json", `"permissionDecision":"deny"`},
		{[]string{"hook", "prompt"}, `{"session_id":"s","prompt":"draft a reply to the vendor"}`, `"hookEventName":"UserPromptSubmit"`},
		{[]string{"hook", "session-start"}, ``, ``},
		{[]string{"hook", "pre-tool", "--extra"}, "not json", ``}, // --extra is silent where plain pre-tool denies
		{[]string{"hook", "nonsense"}, `{}`, ``},
		{[]string{"hook"}, `{}`, ``},
	} {
		cmd := exec.Command(bin, c.args...)
		cmd.Env = append(os.Environ(), "BYLINE_HOME="+home)
		cmd.Stdin = strings.NewReader(c.stdin)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%v: exit %v", c.args, err)
		}
		if (c.want == "" && len(out) != 0) || !strings.Contains(string(out), c.want) {
			t.Errorf("%v: got %q, want %q", c.args, out, c.want)
		}
	}
}
