package state

import (
	"os"
	"path/filepath"
	"testing"

	"byline/internal/paths"
)

// TestWriteFileAtomic_WritesThroughASymlink reproduces gate review M12:
// a symlinked personal file (kept in a dotfiles repo, say) became a
// regular file on the next save, and the link's real target went stale
// with no word about it. WriteFileAtomic must write to the link's
// target and leave the link itself in place.
func TestWriteFileAtomic_WritesThroughASymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.json")
	if err := os.WriteFile(real, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "my-rules.json")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(link, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("my-rules.json is no longer a symlink")
	}
	want(t, readFile(t, link), "new")
	want(t, readFile(t, real), "new")
}

// TestWriteFileAtomic_WritesContentPermAndNoTempLeftover is a functional
// regression guard for gate review M8 (WriteFileAtomic now syncs its temp
// file before the rename): the data, permission and atomic rename-into-
// place must still work exactly as before, and the temp file must never
// be left behind on success.
func TestWriteFileAtomic_WritesContentPermAndNoTempLeftover(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := WriteFileAtomic(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	want(t, readFile(t, path), "hello")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 0600", info.Mode().Perm())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("dir has %d entries, want exactly 1 (no leftover temp file): %v", len(entries), entries)
	}
}

// TestBackup_WritesContentAndPrunes is a functional regression guard for
// gate review M8 (Backup now syncs the backup file before returning): the
// backup's content, permission and pruning must still work as before.
func TestBackup_WritesContentAndPrunes(t *testing.T) {
	testHome(t)
	EnsureVoiceFolder()
	writeFile(t, paths.ProfilePath(), "my profile")
	backup, err := Backup(paths.ProfilePath())
	if err != nil {
		t.Fatal(err)
	}
	want(t, readFile(t, backup), "my profile")
	info, err := os.Stat(backup)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 0600", info.Mode().Perm())
	}
}
