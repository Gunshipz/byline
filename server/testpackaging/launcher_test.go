package testpackaging

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"byline/internal/state"
)

// askLine is the exact bytes byline-hook prints (plus one trailing
// newline) when it can't get a real answer for a send tool. Task 6.4's
// tests reuse it, along with stage and the payload consts below, since
// they share this package.
const askLine = `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask","permissionDecisionReason":"Byline's checks couldn't run, so approve this step yourself, or reinstall Byline."}}`

// sendPayload and prettySendPayload name a tool from state.DefaultSendTools,
// in the hook's plain and plugin-prefixed matcher forms.
var sendPayload = fmt.Sprintf(`{"tool_name":%q}`, state.DefaultSendTools[0])

var prettySendPayload = fmt.Sprintf(`{
  "tool_name": "mcp__plugin_Some-all_%s"
}`, strings.TrimPrefix(state.DefaultSendTools[len(state.DefaultSendTools)-1], "mcp__"))

const skillPayload = `{"tool_name":"Skill"}`

const bashOtherPayload = `{"tool_name":"Bash","tool_input":{"command":"git commit -m \"fix my typo\""}}`

// stage copies both launchers from the repo into <tmp>/<dirName>/bin/ and
// returns that bin directory.
func stage(t *testing.T, dirName string) (binDir string) {
	t.Helper()
	root := repoRoot(t)
	binDir = filepath.Join(t.TempDir(), dirName, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"byline-mcp", "byline-hook"} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(launcherDir), name))
		if err != nil {
			t.Fatalf("read launcher %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(binDir, name), data, 0o755); err != nil {
			t.Fatalf("stage launcher %s: %v", name, err)
		}
	}
	return binDir
}

// stub writes a #!/bin/sh script at path with body appended, then sets
// its mode.
func stub(t *testing.T, path, body string, mode fs.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "#!/bin/sh\n" + body
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

// fakeUname puts a uname first on PATH that answers -s with s and -m
// with m, and returns the directory to prepend to PATH.
func fakeUname(t *testing.T, s, m string) (pathDir string) {
	t.Helper()
	pathDir = t.TempDir()
	body := fmt.Sprintf("case \"$1\" in\n\t-s) echo '%s' ;;\n\t-m) echo '%s' ;;\nesac\n", s, m)
	stub(t, filepath.Join(pathDir, "uname"), body, 0o755)
	return pathDir
}

// run executes <binDir>/<launcher> via sh with the given extra
// environment entries (appended to the current environment, so later
// duplicates win), feeding it stdin, and returns its stdout, stderr and
// exit code.
func run(t *testing.T, binDir, launcher string, env []string, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	path := filepath.Join(binDir, launcher)
	cmdArgs := append([]string{path}, args...)
	cmd := exec.Command("sh", cmdArgs...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("run %s %v: %v", launcher, args, err)
		}
	}
	return out.String(), errb.String(), code
}

// hostBinaryName is the byline-mcp-<os>-<arch> (or .exe) name byline-mcp
// picks on this host, straight from runtime.GOOS/GOARCH.
func hostBinaryName() string {
	switch runtime.GOOS {
	case "darwin":
		return "byline-mcp-darwin-" + runtime.GOARCH
	case "linux":
		return "byline-mcp-linux-" + runtime.GOARCH
	case "windows":
		return "byline-mcp.exe"
	}
	return ""
}

func TestMCPLauncher(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("launchers are POSIX sh; windows isn't covered here")
	}

	t.Run("mcp_launcher_picks_host_binary", func(t *testing.T) {
		binDir := stage(t, "host")
		stub(t, filepath.Join(binDir, hostBinaryName()), "echo host\n", 0o755)
		stdout, _, code := run(t, binDir, "byline-mcp", nil, "")
		if stdout != "host\n" || code != 0 {
			t.Fatalf("stdout=%q code=%d, want %q 0", stdout, code, "host\n")
		}
	})

	t.Run("mcp_launcher_windows_uname_picks_exe", func(t *testing.T) {
		pathDir := fakeUname(t, "MINGW64_NT-10.0-19045", "x86_64")
		binDir := stage(t, "win")
		stub(t, filepath.Join(binDir, "byline-mcp.exe"), "echo win\n", 0o755)
		env := []string{"PATH=" + pathDir + ":" + os.Getenv("PATH")}
		stdout, _, code := run(t, binDir, "byline-mcp", env, "")
		if stdout != "win\n" || code != 0 {
			t.Fatalf("stdout=%q code=%d, want %q 0", stdout, code, "win\n")
		}
	})

	t.Run("mcp_launcher_linux_arm64_uname", func(t *testing.T) {
		pathDir := fakeUname(t, "Linux", "aarch64")
		binDir := stage(t, "linux-arm64")
		stub(t, filepath.Join(binDir, "byline-mcp-linux-arm64"), "echo linux-arm64\n", 0o755)
		env := []string{"PATH=" + pathDir + ":" + os.Getenv("PATH")}
		stdout, _, code := run(t, binDir, "byline-mcp", env, "")
		if stdout != "linux-arm64\n" || code != 0 {
			t.Fatalf("stdout=%q code=%d, want %q 0", stdout, code, "linux-arm64\n")
		}
	})

	t.Run("mcp_launcher_unsupported_platform_exits_1", func(t *testing.T) {
		pathDir := fakeUname(t, "SunOS", "sparc")
		binDir := stage(t, "sunos")
		env := []string{"PATH=" + pathDir + ":" + os.Getenv("PATH")}
		stdout, stderr, code := run(t, binDir, "byline-mcp", env, "")
		if code != 1 {
			t.Fatalf("code=%d, want 1", code)
		}
		if stdout != "" {
			t.Fatalf("stdout=%q, want empty", stdout)
		}
		want := "Byline doesn't run on this kind of computer.\n"
		if stderr != want {
			t.Fatalf("stderr=%q, want %q", stderr, want)
		}
	})

	// mcp_launcher_missing_binary_names_it_and_says_reinstall is M1's
	// regression: a missing platform binary used to surface chmod's own
	// raw "No such file or directory" message. It now names the missing
	// file and tells the person to reinstall (final review M9: the old
	// build-step hint was developer text that reached end users).
	t.Run("mcp_launcher_missing_binary_names_it_and_says_reinstall", func(t *testing.T) {
		binDir := stage(t, "missing-platform-binary") // stage copies only the two launchers
		path := filepath.Join(binDir, hostBinaryName())
		stdout, stderr, code := run(t, binDir, "byline-mcp", nil, "")
		if code != 1 {
			t.Fatalf("code=%d, want 1", code)
		}
		if stdout != "" {
			t.Fatalf("stdout=%q, want empty", stdout)
		}
		if !strings.Contains(stderr, path) {
			t.Errorf("stderr = %q, want it to name %q", stderr, path)
		}
		if !strings.Contains(stderr, "Reinstall Byline") || strings.Contains(stderr, "fill-plugin.sh") {
			t.Errorf("stderr = %q, want reinstall advice and no build step", stderr)
		}
		if strings.Contains(stderr, "chmod") || strings.Contains(stderr, "No such file") {
			t.Errorf("stderr = %q, still leaks chmod's own raw message", stderr)
		}
	})

	t.Run("mcp_launcher_passes_args_stdin_exit_code", func(t *testing.T) {
		binDir := stage(t, "argstdin")
		dir := t.TempDir()
		argsFile := filepath.Join(dir, "args")
		stdinFile := filepath.Join(dir, "stdin")
		body := fmt.Sprintf("printf '%%s' \"$*\" > %q\ncat > %q\nexit 3\n", argsFile, stdinFile)
		stub(t, filepath.Join(binDir, hostBinaryName()), body, 0o755)
		sentStdin := "hello from the test\n"
		_, _, code := run(t, binDir, "byline-mcp", nil, sentStdin, "a", "b")
		if code != 3 {
			t.Fatalf("code=%d, want 3", code)
		}
		gotArgs, err := os.ReadFile(argsFile)
		if err != nil {
			t.Fatalf("read argsFile: %v", err)
		}
		if string(gotArgs) != "a b" {
			t.Fatalf("recorded args=%q, want %q", gotArgs, "a b")
		}
		gotStdin, err := os.ReadFile(stdinFile)
		if err != nil {
			t.Fatalf("read stdinFile: %v", err)
		}
		if string(gotStdin) != sentStdin {
			t.Fatalf("recorded stdin=%q, want %q", gotStdin, sentStdin)
		}
	})

	t.Run("mcp_launcher_restores_exec_bit", func(t *testing.T) {
		binDir := stage(t, "noexec")
		stub(t, filepath.Join(binDir, hostBinaryName()), "echo ok\n", 0o644)
		stdout, _, code := run(t, binDir, "byline-mcp", nil, "")
		if stdout != "ok\n" || code != 0 {
			t.Fatalf("stdout=%q code=%d, want %q 0", stdout, code, "ok\n")
		}
	})
}

func TestLauncher(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("launchers are POSIX sh; windows isn't covered here")
	}

	// sendToolPayloads pairs every default send tool with its bare and
	// plugin-wrapped spellings, each as a one-key JSON payload.
	sendToolPayloads := func() map[string]string {
		m := map[string]string{
			"sendPayload":       sendPayload,
			"prettySendPayload": prettySendPayload,
		}
		for _, t := range state.DefaultSendTools {
			bare := fmt.Sprintf(`{"tool_name":%q}`, t)
			wrapped := fmt.Sprintf(`{"tool_name":%q}`, "mcp__plugin_Some-all_"+t[len("mcp__"):])
			m["bare:"+t] = bare
			m["wrapped:"+t] = wrapped
		}
		return m
	}

	t.Run("launcher_missing_binary_asks_on_pre_tool", func(t *testing.T) {
		for name, payload := range sendToolPayloads() {
			t.Run(name, func(t *testing.T) {
				binDir := stage(t, "missing-"+sanitize(name))
				if err := os.Remove(filepath.Join(binDir, hostBinaryName())); err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
				stdout, _, code := run(t, binDir, "byline-hook", nil, payload, "hook", "pre-tool")
				if code != 0 {
					t.Fatalf("code=%d, want 0", code)
				}
				if stdout != askLine+"\n" {
					t.Fatalf("stdout=%q, want %q", stdout, askLine+"\n")
				}
			})
		}
	})

	t.Run("launcher_missing_binary_silent_on_other_mcp_tools", func(t *testing.T) {
		for _, payload := range []string{
			`{"tool_name":"mcp__builder-mcp__ReadInternalWebsites"}`,
			`{"tool_name":"mcp__my-chat__send_message"}`,
		} {
			binDir := stage(t, "silent")
			stdout, _, code := run(t, binDir, "byline-hook", nil, payload, "hook", "pre-tool")
			if code != 0 {
				t.Fatalf("code=%d, want 0", code)
			}
			if stdout != "" {
				t.Fatalf("stdout=%q, want empty, payload=%s", stdout, payload)
			}
		}
	})

	t.Run("launcher_missing_binary_asks_on_gated_bash_sends", func(t *testing.T) {
		for _, c := range bashSendCommandsToCheck() {
			payload := fmt.Sprintf(`{"tool_name":"Bash","tool_input":{"command":"my %s --message hi"}}`, c)
			binDir := stage(t, "bash-"+sanitize(c))
			stdout, _, code := run(t, binDir, "byline-hook", nil, payload, "hook", "pre-tool")
			if code != 0 || stdout != askLine+"\n" {
				t.Fatalf("%s: stdout=%q code=%d, want the ask line and 0", c, stdout, code)
			}
		}
		binDir := stage(t, "bash-other")
		stdout, _, code := run(t, binDir, "byline-hook", nil, bashOtherPayload, "hook", "pre-tool")
		if code != 0 || stdout != "" {
			t.Fatalf("a shell command this hook doesn't gate: stdout=%q code=%d, want empty 0", stdout, code)
		}
	})

	t.Run("launcher_extra_mode_never_asks", func(t *testing.T) {
		payloads := []string{sendPayload, `{"tool_name":"mcp__my-chat__send_message"}`}
		for _, payload := range payloads {
			binDir := stage(t, "extra-missing")
			stdout, _, code := run(t, binDir, "byline-hook", nil, payload, "hook", "pre-tool", "--extra")
			if code != 0 || stdout != "" {
				t.Fatalf("no-binary: stdout=%q code=%d, want empty 0", stdout, code)
			}

			binDir2 := stage(t, "extra-failing")
			stub(t, filepath.Join(binDir2, hostBinaryName()), "exit 2\n", 0o755)
			stdout2, _, code2 := run(t, binDir2, "byline-hook", nil, payload, "hook", "pre-tool", "--extra")
			if code2 != 0 || stdout2 != "" {
				t.Fatalf("failing binary: stdout=%q code=%d, want empty 0", stdout2, code2)
			}
		}
	})

	t.Run("launcher_failing_binary_asks_on_pre_tool", func(t *testing.T) {
		binDir := stage(t, "failing")
		stub(t, filepath.Join(binDir, hostBinaryName()), "echo partial\necho oops >&2\nexit 2\n", 0o755)
		stdout, stderr, code := run(t, binDir, "byline-hook", nil, sendPayload, "hook", "pre-tool")
		if code != 0 {
			t.Fatalf("code=%d, want 0", code)
		}
		if stdout != askLine+"\n" {
			t.Fatalf("stdout=%q, want %q", stdout, askLine+"\n")
		}
		if stderr != "" {
			t.Fatalf("stderr=%q, want empty", stderr)
		}
	})

	t.Run("launcher_killed_binary_asks_on_pre_tool", func(t *testing.T) {
		binDir := stage(t, "killed")
		stub(t, filepath.Join(binDir, hostBinaryName()), "kill -9 $$\n", 0o755)
		stdout, stderr, code := run(t, binDir, "byline-hook", nil, sendPayload, "hook", "pre-tool")
		if code != 0 {
			t.Fatalf("code=%d, want 0", code)
		}
		if stdout != askLine+"\n" {
			t.Fatalf("stdout=%q, want %q", stdout, askLine+"\n")
		}
		if stderr != "" {
			t.Fatalf("stderr=%q, want empty", stderr)
		}
	})

	t.Run("launcher_unsupported_platform_asks_on_pre_tool", func(t *testing.T) {
		pathDir := fakeUname(t, "SunOS", "sparc")
		binDir := stage(t, "sunos-hook")
		env := []string{"PATH=" + pathDir + ":" + os.Getenv("PATH")}
		stdout, _, code := run(t, binDir, "byline-hook", env, sendPayload, "hook", "pre-tool")
		if code != 0 {
			t.Fatalf("code=%d, want 0", code)
		}
		if stdout != askLine+"\n" {
			t.Fatalf("stdout=%q, want %q", stdout, askLine+"\n")
		}
	})

	t.Run("launcher_unreadable_payload_asks_on_pre_tool", func(t *testing.T) {
		binDir := stage(t, "unreadable")
		stdout, _, code := run(t, binDir, "byline-hook", nil, "not json", "hook", "pre-tool")
		if code != 0 {
			t.Fatalf("code=%d, want 0", code)
		}
		if stdout != askLine+"\n" {
			t.Fatalf("stdout=%q, want %q", stdout, askLine+"\n")
		}
	})

	t.Run("launcher_missing_mcp_launcher_asks_on_pre_tool", func(t *testing.T) {
		binDir := stage(t, "no-mcp-launcher")
		if err := os.Remove(filepath.Join(binDir, "byline-mcp")); err != nil {
			t.Fatal(err)
		}
		stdout, _, code := run(t, binDir, "byline-hook", nil, sendPayload, "hook", "pre-tool")
		if code != 0 {
			t.Fatalf("code=%d, want 0", code)
		}
		if stdout != askLine+"\n" {
			t.Fatalf("stdout=%q, want %q", stdout, askLine+"\n")
		}
	})

	t.Run("launcher_silent_on_skill_pre_tool", func(t *testing.T) {
		binDir := stage(t, "skill")
		stdout, _, code := run(t, binDir, "byline-hook", nil, skillPayload, "hook", "pre-tool")
		if code != 0 {
			t.Fatalf("code=%d, want 0", code)
		}
		if stdout != "" {
			t.Fatalf("stdout=%q, want empty", stdout)
		}
	})

	t.Run("launcher_silent_on_other_modes", func(t *testing.T) {
		modes := []string{"session-start", "prompt", "read"}
		for _, mode := range modes {
			binDir := stage(t, "mode-missing-"+mode)
			stdout, _, code := run(t, binDir, "byline-hook", nil, sendPayload, "hook", mode)
			if code != 0 || stdout != "" {
				t.Fatalf("mode=%s no-binary: stdout=%q code=%d, want empty 0", mode, stdout, code)
			}

			binDir2 := stage(t, "mode-failing-"+mode)
			stub(t, filepath.Join(binDir2, hostBinaryName()), "exit 2\n", 0o755)
			stdout2, _, code2 := run(t, binDir2, "byline-hook", nil, sendPayload, "hook", mode)
			if code2 != 0 || stdout2 != "" {
				t.Fatalf("mode=%s failing binary: stdout=%q code=%d, want empty 0", mode, stdout2, code2)
			}
		}
	})

	t.Run("launcher_passes_output_args_and_stdin", func(t *testing.T) {
		binDir := stage(t, "passthrough")
		dir := t.TempDir()
		argsFile := filepath.Join(dir, "args")
		stdinFile := filepath.Join(dir, "stdin")
		body := fmt.Sprintf("printf '%%s' \"$*\" > %q\ncat > %q\necho '{\"ok\":true}'\n", argsFile, stdinFile)
		stub(t, filepath.Join(binDir, hostBinaryName()), body, 0o755)

		sent := strings.Repeat("x", 1024*1024)
		stdout, _, code := run(t, binDir, "byline-hook", nil, sent, "hook", "pre-tool")
		if code != 0 {
			t.Fatalf("code=%d, want 0", code)
		}
		if stdout != "{\"ok\":true}\n" {
			t.Fatalf("stdout=%q, want %q", stdout, "{\"ok\":true}\n")
		}
		gotArgs, err := os.ReadFile(argsFile)
		if err != nil {
			t.Fatalf("read argsFile: %v", err)
		}
		if string(gotArgs) != "hook pre-tool" {
			t.Fatalf("recorded args=%q, want %q", gotArgs, "hook pre-tool")
		}
		gotStdin, err := os.ReadFile(stdinFile)
		if err != nil {
			t.Fatalf("read stdinFile: %v", err)
		}
		if string(gotStdin) != sent {
			t.Fatalf("recorded stdin differs: got %d bytes, want %d bytes", len(gotStdin), len(sent))
		}
	})

	t.Run("launcher_silent_when_binary_prints_nothing", func(t *testing.T) {
		binDir := stage(t, "quiet")
		stub(t, filepath.Join(binDir, hostBinaryName()), "exit 0\n", 0o755)
		stdout, _, code := run(t, binDir, "byline-hook", nil, sendPayload, "hook", "pre-tool")
		if code != 0 {
			t.Fatalf("code=%d, want 0", code)
		}
		if stdout != "" {
			t.Fatalf("stdout=%q, want empty", stdout)
		}
	})

	t.Run("launcher_runs_from_path_with_space", func(t *testing.T) {
		binDir := stage(t, "dir with space")
		stub(t, filepath.Join(binDir, hostBinaryName()), "echo '{\"ok\":true}'\n", 0o755)
		stdout, _, code := run(t, binDir, "byline-hook", nil, sendPayload, "hook", "pre-tool")
		if code != 0 {
			t.Fatalf("code=%d, want 0", code)
		}
		if stdout != "{\"ok\":true}\n" {
			t.Fatalf("stdout=%q, want %q", stdout, "{\"ok\":true}\n")
		}
	})

	t.Run("launcher_files_have_lf_endings", func(t *testing.T) {
		root := repoRoot(t)
		for _, name := range []string{"byline-mcp", "byline-hook"} {
			data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(launcherDir), name))
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			if bytes.Contains(data, []byte("\r")) {
				t.Fatalf("%s contains a carriage return", name)
			}
			if !bytes.HasPrefix(data, []byte("#!/bin/sh\n")) {
				t.Fatalf("%s does not start with %q", name, "#!/bin/sh\n")
			}
		}
	})
}

// sanitize turns a subtest name into something safe for stage's temp
// directory name.
func sanitize(s string) string {
	return strings.NewReplacer("/", "-", ":", "-", " ", "-").Replace(s)
}
