package testpackaging

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runEmbed runs server/scripts/embed.sh found under root.
func runEmbed(t *testing.T, root string) {
	t.Helper()
	cmd := exec.Command("sh", filepath.Join(root, "server", "scripts", "embed.sh"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("embed.sh failed: %v\n%s", err, out)
	}
}

// TestEmbed_CommittedCopiesInSync checks the copies actually committed in
// this repo against their sources. A copy is allowed to hold files its
// source doesn't have yet (a forward-placed placeholder embed.sh will
// replace once the source catches up); it is never allowed to be missing a
// source file or to hold stale content.
func TestEmbed_CommittedCopiesInSync(t *testing.T) {
	root := repoRoot(t)
	pairs := [][2]string{
		{filepath.Join(root, "skills", "byline"), filepath.Join(root, "server", "embed", "skills", "byline")},
		{filepath.Join(root, "rules"), filepath.Join(root, "server", "embed", "rules")},
		{filepath.Join(root, "templates"), filepath.Join(root, "server", "embed", "templates")},
	}
	for _, p := range pairs {
		src, dst := p[0], p[1]
		if _, err := os.Stat(src); os.IsNotExist(err) {
			continue
		}
		var drift []string
		for _, d := range withoutEvals(sameTree(t, src, dst)) {
			if strings.HasPrefix(d, "extra in ") {
				continue
			}
			drift = append(drift, d)
		}
		if len(drift) > 0 {
			t.Fatalf("%s out of sync with %s: %v: run server/scripts/embed.sh and commit", dst, src, drift)
		}
	}
}

func TestEmbed_CopiesAreExact(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	copyTree(t, filepath.Join(root, "skills"), filepath.Join(tmp, "skills"))
	copyTree(t, filepath.Join(root, "rules"), filepath.Join(tmp, "rules"))
	copyTree(t, filepath.Join(root, "templates"), filepath.Join(tmp, "templates"))
	copyTree(t, filepath.Join(root, "server", "scripts"), filepath.Join(tmp, "server", "scripts"))

	runEmbed(t, tmp)

	pairs := [][2]string{
		{filepath.Join(tmp, "skills", "byline"), filepath.Join(tmp, "server", "embed", "skills", "byline")},
		{filepath.Join(tmp, "rules"), filepath.Join(tmp, "server", "embed", "rules")},
		{filepath.Join(tmp, "templates"), filepath.Join(tmp, "server", "embed", "templates")},
	}
	for _, p := range pairs {
		if diffs := withoutEvals(sameTree(t, p[0], p[1])); len(diffs) > 0 {
			t.Fatalf("%s vs %s: %v", p[0], p[1], diffs)
		}
	}
}

func TestEmbed_RemovesStaleFilesKeepsOthers(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	copyTree(t, filepath.Join(root, "skills"), filepath.Join(tmp, "skills"))
	copyTree(t, filepath.Join(root, "server", "scripts"), filepath.Join(tmp, "server", "scripts"))

	stale := filepath.Join(tmp, "server", "embed", "skills", "byline", "stale.md")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	embedGo := filepath.Join(tmp, "server", "embed", "embed.go")
	want := "package embed\n"
	if err := os.WriteFile(embedGo, []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}

	runEmbed(t, tmp)

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale.md still exists (stat err=%v)", err)
	}
	got, err := os.ReadFile(embedGo)
	if err != nil {
		t.Fatalf("embed.go missing: %v", err)
	}
	if string(got) != want {
		t.Fatalf("embed.go changed: got %q want %q", got, want)
	}
}

func TestEmbed_MissingSourceIsSkipped(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	copyTree(t, filepath.Join(root, "skills"), filepath.Join(tmp, "skills"))
	copyTree(t, filepath.Join(root, "server", "scripts"), filepath.Join(tmp, "server", "scripts"))

	runEmbed(t, tmp)

	if _, err := os.Stat(filepath.Join(tmp, "server", "embed", "skills", "byline", "skill.json")); err != nil {
		t.Fatalf("skills/byline not copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "server", "embed", "rules")); !os.IsNotExist(err) {
		t.Fatalf("rules copy should be absent, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "server", "embed", "templates")); !os.IsNotExist(err) {
		t.Fatalf("templates copy should be absent, err=%v", err)
	}
}

// withoutEvals drops the skill's evals/ folder from a tree comparison:
// embed.sh leaves it out of the helper's copy on purpose, since the helper
// never runs the evals.
func withoutEvals(diffs []string) []string {
	var out []string
	for _, d := range diffs {
		if strings.Contains(d, ": evals/") || strings.HasSuffix(d, ": evals") {
			continue
		}
		out = append(out, d)
	}
	return out
}
