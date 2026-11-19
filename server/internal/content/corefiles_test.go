package content

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// coreFiles returns the non-comment, non-blank lines of
// scripts/core-files.txt: the allow-list scripts/sync-core.sh copies from
// the private copy this started from.
func coreFiles(t *testing.T, root string) []string {
	t.Helper()
	f, err := os.Open(filepath.Join(root, "scripts", "core-files.txt"))
	if err != nil {
		t.Fatalf("open core-files.txt: %v", err)
	}
	defer f.Close()
	var files []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		files = append(files, line)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan core-files.txt: %v", err)
	}
	return files
}

// coreFileHashes parses scripts/core-files.sha256 ("<hex sha256>  <path>"
// per line, scripts/sync-core.sh's own output format) into a
// path-to-hash map.
func coreFileHashes(t *testing.T, root string) map[string]string {
	t.Helper()
	f, err := os.Open(filepath.Join(root, "scripts", "core-files.sha256"))
	if err != nil {
		t.Fatalf("open core-files.sha256: %v", err)
	}
	defer f.Close()
	hashes := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("core-files.sha256: malformed line %q", line)
		}
		hashes[fields[1]] = fields[0]
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan core-files.sha256: %v", err)
	}
	return hashes
}

// sha256HexOf returns path's hex-encoded sha256, the same form
// scripts/sync-core.sh writes.
func sha256HexOf(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// TestCoreFiles_MatchStamp is the guard against editing a shared file
// directly in this copy instead of through scripts/sync-core.sh: it fails
// when a file on scripts/core-files.txt no longer matches the sha256
// scripts/sync-core.sh recorded for it in scripts/core-files.sha256, or
// when the two files disagree on which paths are listed.
func TestCoreFiles_MatchStamp(t *testing.T) {
	root := repoRoot(t)
	files := coreFiles(t, root)
	hashes := coreFileHashes(t, root)

	listed := map[string]bool{}
	for _, rel := range files {
		listed[rel] = true
		want, ok := hashes[rel]
		if !ok {
			t.Errorf("%s: on core-files.txt but not stamped in core-files.sha256: run scripts/sync-core.sh", rel)
			continue
		}
		got := sha256HexOf(t, filepath.Join(root, rel))
		if got != want {
			t.Errorf("%s: sha256 %s, stamp wants %s: edited directly instead of through scripts/sync-core.sh", rel, got, want)
		}
	}
	for rel := range hashes {
		if !listed[rel] {
			t.Errorf("%s: stamped in core-files.sha256 but not on core-files.txt", rel)
		}
	}
}

// TestSkillBody_MatchesStamp guards skills/byline/SKILL.md's one fixed sync
// rule (scripts/sync-core.sh): its body, after the frontmatter, must be
// scripts/skill-body.stamp.md with the bare "/byline " and "/byline`"
// command form rewritten to "/byline:byline " and "/byline:byline`". A
// mismatch means the body was edited here directly instead of through that
// script.
func TestSkillBody_MatchesStamp(t *testing.T) {
	root := repoRoot(t)
	stamp, err := os.ReadFile(filepath.Join(root, "scripts", "skill-body.stamp.md"))
	if err != nil {
		t.Fatalf("read scripts/skill-body.stamp.md: %v", err)
	}
	want := strings.NewReplacer(
		"/byline`", "/byline:byline`",
		"/byline ", "/byline:byline ",
	).Replace(string(stamp))

	skill := readFile(t, "skills/byline/SKILL.md")
	idx := strings.Index(skill, "\n---\n")
	if idx < 0 {
		t.Fatal("skills/byline/SKILL.md has no closing frontmatter --- line")
	}
	if got := skill[idx+len("\n---\n"):]; got != want {
		t.Error("skills/byline/SKILL.md body doesn't match what sync-core.sh's rule produces from scripts/skill-body.stamp.md: edited directly instead of through that script")
	}
}

// TestCoreRules_SharedMatchStamp guards rules/core-rules.json's 49 rules
// shared with the private copy this started from: each must still match
// scripts/core-rules-shared.stamp.json, the sha256 scripts/sync-core.sh's
// fixed rule for that file last wrote. A mismatch means a shared rule was
// edited here directly instead of through that script.
func TestCoreRules_SharedMatchStamp(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "scripts", "core-rules-shared.stamp.json"))
	if err != nil {
		t.Fatalf("read scripts/core-rules-shared.stamp.json: %v", err)
	}
	var stamped []map[string]any
	if err := json.Unmarshal(data, &stamped); err != nil {
		t.Fatalf("parse scripts/core-rules-shared.stamp.json: %v", err)
	}

	byID := map[string]map[string]any{}
	for _, r := range coreRules(t).Rules {
		byID[str(r, "id")] = r
	}
	for _, want := range stamped {
		id, _ := want["id"].(string)
		got, ok := byID[id]
		if !ok {
			t.Errorf("rule %q is in scripts/core-rules-shared.stamp.json but not in rules/core-rules.json", id)
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("rule %q doesn't match scripts/core-rules-shared.stamp.json: edited directly instead of through scripts/sync-core.sh", id)
		}
	}
}
