package testpackaging

import (
	"bytes"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildableRoot copies server/, skills/ and rules/ from the real repo into a
// fresh temp root, ready for build.sh to run against. It also copies
// templates/ if the repo has one yet; otherwise it synthesizes a minimal one,
// since a release build needs something there to embed regardless of
// whether that phase of the plan has landed in this branch yet.
func buildableRoot(t *testing.T) string {
	t.Helper()
	root := repoRoot(t)
	tmp := t.TempDir()
	copyTree(t, filepath.Join(root, "server"), filepath.Join(tmp, "server"))
	copyTree(t, filepath.Join(root, "skills"), filepath.Join(tmp, "skills"))
	copyTree(t, filepath.Join(root, "rules"), filepath.Join(tmp, "rules"))

	templatesSrc := filepath.Join(root, "templates")
	templatesDst := filepath.Join(tmp, "templates")
	if _, err := os.Stat(templatesSrc); err == nil {
		copyTree(t, templatesSrc, templatesDst)
	} else {
		if err := os.MkdirAll(templatesDst, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(templatesDst, "placeholder.md"), []byte("placeholder\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return tmp
}

// skillVersion reads the version field out of root's skills/byline/skill.json.
func skillVersion(t *testing.T, root string) string {
	t.Helper()
	var m map[string]any
	readJSON(t, filepath.Join(root, "skills", "byline", "skill.json"), &m)
	v, _ := m["version"].(string)
	if v == "" {
		t.Fatalf("skill.json has no version")
	}
	return v
}

// runBuild runs server/scripts/build.sh under root with the given OUTDIR and
// any extra environment variables, and returns its stdout, stderr and error.
func runBuild(t *testing.T, root, outdir string, extraEnv ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := exec.Command("sh", filepath.Join(root, "server", "scripts", "build.sh"), outdir)
	cmd.Env = append(os.Environ(), extraEnv...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err = cmd.Run()
	return out.String(), errb.String(), err
}

// strayBuildOutputs lists any byline-mcp-* files directly under dir.
func strayBuildOutputs(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var stray []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "byline-mcp-") {
			stray = append(stray, e.Name())
		}
	}
	return stray
}

func TestVersion_SkillJSONEmbeddedAndBinaryAgree(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	root := repoRoot(t)
	version := skillVersion(t, root)

	embedded, err := os.ReadFile(filepath.Join(root, "server", "embed", "skills", "byline", "skill.json"))
	if err != nil {
		t.Fatalf("read embedded skill.json: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(root, "skills", "byline", "skill.json"))
	if err != nil {
		t.Fatalf("read source skill.json: %v", err)
	}
	if !bytes.Equal(embedded, source) {
		t.Fatalf("server/embed/skills/byline/skill.json differs from skills/byline/skill.json")
	}

	bin := buildHost(t)
	out, err := exec.Command(bin, "version").Output()
	if err != nil {
		t.Fatalf("run %s version: %v", bin, err)
	}
	want := "byline-mcp " + version + "\n"
	if string(out) != want {
		t.Fatalf("stdout = %q, want %q", out, want)
	}
}

func TestBuild_RefusesMissingEmbedSource(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	root := repoRoot(t)
	tmp := t.TempDir()
	copyTree(t, filepath.Join(root, "server"), filepath.Join(tmp, "server"))
	copyTree(t, filepath.Join(root, "skills"), filepath.Join(tmp, "skills"))
	copyTree(t, filepath.Join(root, "rules"), filepath.Join(tmp, "rules"))
	// templates/ deliberately not copied

	outdir := filepath.Join(tmp, "out")
	if err := os.MkdirAll(outdir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, stderr, err := runBuild(t, tmp, outdir)
	if err == nil {
		t.Fatal("build.sh succeeded, want it to fail with templates/ missing")
	}
	if !strings.Contains(stderr, "templates/") {
		t.Fatalf("stderr %q does not name templates/", stderr)
	}
	if stray := strayBuildOutputs(t, outdir); len(stray) > 0 {
		t.Fatalf("OUTDIR has stray files %v", stray)
	}
}

func TestBuild_FiveTargetsRightFormat(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	tmp := buildableRoot(t)
	outdir := filepath.Join(tmp, "out")
	if err := os.MkdirAll(outdir, 0o755); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := runBuild(t, tmp, outdir)
	if err != nil {
		t.Fatalf("build.sh failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}

	want := []string{
		"byline-mcp-darwin-arm64",
		"byline-mcp-darwin-amd64",
		"byline-mcp-linux-amd64",
		"byline-mcp-linux-arm64",
		"byline-mcp.exe",
	}
	entries, err := os.ReadDir(outdir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(want) {
		var got []string
		for _, e := range entries {
			got = append(got, e.Name())
		}
		t.Fatalf("OUTDIR has %v, want exactly %v", got, want)
	}
	for _, w := range want {
		if _, err := os.Stat(filepath.Join(outdir, w)); err != nil {
			t.Fatalf("OUTDIR missing %s: %v", w, err)
		}
	}

	checkMacho := func(name string, wantCPU macho.Cpu) {
		f, err := os.Open(filepath.Join(outdir, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		defer f.Close()
		mf, err := macho.NewFile(f)
		if err != nil {
			t.Fatalf("%s: not a macho binary: %v", name, err)
		}
		if mf.Cpu != wantCPU {
			t.Fatalf("%s: Cpu = %v, want %v", name, mf.Cpu, wantCPU)
		}
	}
	checkMacho("byline-mcp-darwin-arm64", macho.CpuArm64)
	checkMacho("byline-mcp-darwin-amd64", macho.CpuAmd64)

	checkElf := func(name string, wantMachine elf.Machine) {
		f, err := os.Open(filepath.Join(outdir, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		defer f.Close()
		ef, err := elf.NewFile(f)
		if err != nil {
			t.Fatalf("%s: not an elf binary: %v", name, err)
		}
		if ef.Machine != wantMachine {
			t.Fatalf("%s: Machine = %v, want %v", name, ef.Machine, wantMachine)
		}
	}
	checkElf("byline-mcp-linux-amd64", elf.EM_X86_64)
	checkElf("byline-mcp-linux-arm64", elf.EM_AARCH64)

	f, err := os.Open(filepath.Join(outdir, "byline-mcp.exe"))
	if err != nil {
		t.Fatalf("byline-mcp.exe: %v", err)
	}
	defer f.Close()
	pf, err := pe.NewFile(f)
	if err != nil {
		t.Fatalf("byline-mcp.exe: not a pe binary: %v", err)
	}
	if pf.Machine != 0x8664 {
		t.Fatalf("byline-mcp.exe: Machine = %#x, want 0x8664", pf.Machine)
	}
}

func TestBuild_BinariesAreStaticAndTrimmed(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	tmp := buildableRoot(t)
	outdir := filepath.Join(tmp, "out")
	if err := os.MkdirAll(outdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, stderr, err := runBuild(t, tmp, outdir); err != nil {
		t.Fatalf("build.sh failed: %v\n%s", err, stderr)
	}

	bin := filepath.Join(outdir, "byline-mcp-linux-amd64")
	out, err := exec.Command("go", "version", "-m", bin).CombinedOutput()
	if err != nil {
		t.Fatalf("go version -m %s: %v\n%s", bin, err, out)
	}
	for _, want := range []string{"CGO_ENABLED=0", "-trimpath=true"} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("go version -m output missing %q:\n%s", want, out)
		}
	}

	needle := []byte(repoRoot(t))
	entries, err := os.ReadDir(outdir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(outdir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, needle) {
			t.Fatalf("%s contains the repo root path", e.Name())
		}
	}
}

func TestBuild_RemovesStaleBinariesKeepsLaunchers(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	tmp := buildableRoot(t)
	outdir := filepath.Join(tmp, "out")
	if err := os.MkdirAll(outdir, 0o755); err != nil {
		t.Fatal(err)
	}

	stale := filepath.Join(outdir, "byline-mcp-plan9-386")
	hookLauncher := filepath.Join(outdir, "byline-hook")
	posixLauncher := filepath.Join(outdir, "byline-mcp")
	hookBytes := []byte("hook launcher bytes\n")
	posixBytes := []byte("posix launcher bytes\n")
	if err := os.WriteFile(stale, []byte("stale"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hookLauncher, hookBytes, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(posixLauncher, posixBytes, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, stderr, err := runBuild(t, tmp, outdir); err != nil {
		t.Fatalf("build.sh failed: %v\n%s", err, stderr)
	}

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale binary still present (stat err=%v)", err)
	}
	gotHook, err := os.ReadFile(hookLauncher)
	if err != nil {
		t.Fatalf("byline-hook missing: %v", err)
	}
	if !bytes.Equal(gotHook, hookBytes) {
		t.Fatalf("byline-hook changed: got %q want %q", gotHook, hookBytes)
	}
	gotPosix, err := os.ReadFile(posixLauncher)
	if err != nil {
		t.Fatalf("byline-mcp launcher missing: %v", err)
	}
	if !bytes.Equal(gotPosix, posixBytes) {
		t.Fatalf("byline-mcp launcher changed: got %q want %q", gotPosix, posixBytes)
	}
}

func TestBuild_FailsWithoutGo(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	tmp := buildableRoot(t)
	outdir := filepath.Join(tmp, "out")
	if err := os.MkdirAll(outdir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, stderr, err := runBuild(t, tmp, outdir, "GO=/nonexistent/go")
	if err == nil {
		t.Fatal("build.sh succeeded, want it to fail without Go")
	}
	if !strings.Contains(stderr, "Go 1.22 or later is needed to build byline-mcp") {
		t.Fatalf("stderr %q missing expected message", stderr)
	}
	if stray := strayBuildOutputs(t, outdir); len(stray) > 0 {
		t.Fatalf("OUTDIR has stray files %v", stray)
	}
}
