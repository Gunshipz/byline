package evlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := strings.TrimRight(string(data), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func TestAppend_WritesOneJSONLineWithFileAndFolderPerms(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "log.jsonl")
	Append(path, map[string]any{"a": 1})

	lines := readLines(t, path)
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1: %v", len(lines), lines)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &m); err != nil {
		t.Fatalf("not valid JSON: %v", lines[0])
	}

	fi, err := os.Stat(path)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("file perm = %v, err %v", fi.Mode(), err)
	}
	di, err := os.Stat(filepath.Dir(path))
	if err != nil || di.Mode().Perm() != 0o700 {
		t.Fatalf("dir perm = %v, err %v", di.Mode(), err)
	}
}

func TestAppend_SecondCallAppendsRatherThanOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.jsonl")
	Append(path, map[string]any{"n": 1})
	Append(path, map[string]any{"n": 2})
	if lines := readLines(t, path); len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %v", len(lines), lines)
	}
}

// TestAppend_RotatesAtCap: a log already at or past maxBytes is rolled to
// ".1" (replacing any older one) before the new line is written, so the
// live file never grows without bound.
func TestAppend_RotatesAtCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.jsonl")
	rotated := path + ".1"
	if err := os.WriteFile(path, []byte(strings.Repeat("x", maxBytes)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rotated, []byte("stale-older-rotation\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	Append(path, map[string]any{"fresh": true})

	rotatedData, err := os.ReadFile(rotated)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rotatedData), "stale-older-rotation") {
		t.Fatal("the older .1 was not replaced by the rotated-out file")
	}
	if !strings.Contains(string(rotatedData), strings.Repeat("x", 10)) {
		t.Fatal("rotated file doesn't hold the content that was at the cap")
	}

	lines := readLines(t, path)
	if len(lines) != 1 || !strings.Contains(lines[0], `"fresh":true`) {
		t.Fatalf("fresh file after rotation = %v", lines)
	}
}

// TestAppend_UnwritableFolderDropsSilently: when path's folder can't be
// created (here, something not a directory sits where the folder needs
// to go), Append must not panic and must leave nothing behind.
func TestAppend_UnwritableFolderDropsSilently(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "logs")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(blocker, "log.jsonl")
	Append(path, map[string]any{"a": 1}) // must not panic
	if _, err := os.Stat(path); err == nil {
		t.Fatal("a line was written despite an unwritable folder")
	}
}

// TestAppend_ConcurrentRotationKeepsHistory: two hooks can write at once
// (Claude Code runs matching hooks in parallel). When several writers find
// the log at its cap together, exactly one rolls it over: the rotated file
// keeps the history and no line is lost.
func TestAppend_ConcurrentRotationKeepsHistory(t *testing.T) {
	for round := 0; round < 5; round++ {
		path := filepath.Join(t.TempDir(), "log.jsonl")
		if err := os.WriteFile(path, []byte(strings.Repeat("x", maxBytes)+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		for g := 0; g < 8; g++ {
			wg.Add(1)
			go func(g int) {
				defer wg.Done()
				for i := 0; i < 20; i++ {
					Append(path, map[string]any{"g": g, "i": i})
				}
			}(g)
		}
		wg.Wait()
		rotated, err := os.ReadFile(path + ".1")
		if err != nil || len(rotated) < maxBytes {
			t.Fatalf("round %d: the rotated file lost the history (%d bytes, %v)", round, len(rotated), err)
		}
		fresh := len(readLines(t, path))
		inRotated := strings.Count(string(rotated), "\n") - 1 // minus the filler line
		if fresh+inRotated != 160 {
			t.Fatalf("round %d: %d of 160 lines survived", round, fresh+inRotated)
		}
	}
}
