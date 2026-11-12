// Package state owns Byline's on-disk voice folder: personal rules,
// settings and their backups.
package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"byline/internal/paths"
)

// MaxBackupsPerFile is how many backups of one file Backup keeps; it prunes
// the oldest beyond this count.
const MaxBackupsPerFile = 20

// Format is the voice-folder JSON files' current schema version.
const Format = 1

// ErrNewerFormat means a voice-folder file was saved by a newer version of
// Byline than this one; this version must never read or rewrite it.
var ErrNewerFormat = errors.New("saved by a newer version of Byline")

// WriteFileAtomic writes data to a temp file in path's folder, syncs it
// to disk, then renames it into place, so a reader never sees a
// half-written file and a power loss right after a save can't leave an
// empty file where the rename lands (M8). If path is a symlink -- kept
// in a dotfiles repo, say -- the bytes go to whatever it points at, so
// the link survives instead of becoming a regular file (M12). The temp
// file is removed on any error.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	target, err := resolveSymlink(path)
	if err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(target), fmt.Sprintf("%s.tmp-%d", filepath.Base(target), os.Getpid()))
	if err := writeFileSynced(tmp, data, perm); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// resolveSymlink returns path's real target when path is a symlink
// (M12), else path unchanged. A path that doesn't exist yet isn't a
// symlink.
func resolveSymlink(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return path, nil
		}
		return "", err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return path, nil
	}
	return filepath.EvalSymlinks(path)
}

// writeFileSynced writes data to path like os.WriteFile, but calls Sync
// before closing, so the bytes are flushed to disk instead of left in a
// buffer a power loss could still drop (M8).
func writeFileSynced(path string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Backup copies an existing file into BackupsDir() with a UTC-timestamped
// name, then prunes older backups of the same base name beyond
// MaxBackupsPerFile. It returns the backup's path. The backup is synced
// to disk before this returns (M8), the same as WriteFileAtomic.
func Backup(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	base := filepath.Base(path)
	stamp := time.Now().UTC().Format("20060102T150405.000000Z")
	dest := filepath.Join(paths.BackupsDir(), base+"."+stamp+".bak")
	for i := 2; fileExists(dest); i++ {
		dest = filepath.Join(paths.BackupsDir(), fmt.Sprintf("%s.%s-%d.bak", base, stamp, i))
	}
	if err := writeFileSynced(dest, data, 0o600); err != nil {
		return "", err
	}
	pruneBackups(base)
	return dest, nil
}

// WriteVoiceFile backs up path if it already exists, then writes data to
// it atomically at 0600. It returns the backup's path, "" if there was
// nothing to back up.
func WriteVoiceFile(path string, data []byte) (string, error) {
	var backup string
	if fileExists(path) {
		b, err := Backup(path)
		if err != nil {
			return "", err
		}
		backup = b
	}
	if err := WriteFileAtomic(path, data, 0o600); err != nil {
		return backup, err
	}
	return backup, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func pruneBackups(base string) {
	re := regexp.MustCompile(`^` + regexp.QuoteMeta(base) + `\.\d{8}T\d{6}\.\d{6}Z(-\d+)?\.bak$`)
	entries, err := os.ReadDir(paths.BackupsDir())
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && re.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names) // the fixed-width timestamp sorts oldest-first lexically
	for len(names) > MaxBackupsPerFile {
		os.Remove(filepath.Join(paths.BackupsDir(), names[0]))
		names = names[1:]
	}
}
