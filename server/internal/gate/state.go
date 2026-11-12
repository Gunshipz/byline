package gate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"byline/internal/paths"
)

// stateMaxAge is how long a session's state files live before pruneState
// removes them.
const stateMaxAge = 7 * 24 * time.Hour

// slugMaxLen is the longest sessionSlug returns before hashing: short
// enough to leave room for a ".read-exemplars" suffix on every
// filesystem's name-length limit.
const slugMaxLen = 100

// slugDisallowed matches every character sessionSlug replaces.
var slugDisallowed = regexp.MustCompile(`[^A-Za-z0-9_.-]`)

// sessionSlug turns a session id into a safe file-name component: every
// character outside [A-Za-z0-9_.-] becomes "_"; a result over slugMaxLen
// characters is replaced by its SHA-256 hex hash, so a long id still arms
// instead of silently failing to fit on disk (M8).
func sessionSlug(id string) string {
	slug := slugDisallowed.ReplaceAllString(id, "_")
	if len(slug) <= slugMaxLen {
		return slug
	}
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:])
}

// armedFile and readMarkFile name a session's state files under
// paths.StateDir().
func armedFile(slug string) string {
	return filepath.Join(paths.StateDir(), slug+".armed")
}

func readMarkFile(slug, key string) string {
	return filepath.Join(paths.StateDir(), slug+".read-"+key)
}

// writeStateFile creates the state folder (0o700) and writes data to path
// (0o600). It ignores every error, per the state layout's contract.
func writeStateFile(path string, data []byte) {
	if err := os.MkdirAll(paths.StateDir(), 0o700); err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o600)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// arm marks a session as having had a draft request detected. Does
// nothing when slug is empty.
func arm(slug string) {
	if slug == "" {
		return
	}
	writeStateFile(armedFile(slug), []byte(fmt.Sprintf(`{"at":%q}`, time.Now().Format(time.RFC3339))))
}

// isArmed reports whether arm(slug) has been called and not since reset.
func isArmed(slug string) bool {
	if slug == "" {
		return false
	}
	return fileExists(armedFile(slug))
}

// recordRead marks that a session's hook has seen a real Read of the file
// named by key (one of "brief", "profile", "exemplars"). Does nothing
// when slug is empty.
func recordRead(slug, key string) {
	if slug == "" {
		return
	}
	writeStateFile(readMarkFile(slug, key), nil)
}

// hasRead reports whether recordRead(slug, key) has been called and not
// since reset.
func hasRead(slug, key string) bool {
	if slug == "" {
		return false
	}
	return fileExists(readMarkFile(slug, key))
}

// resetSession clears a session's armed mark and its three read marks, on
// startup and clear.
func resetSession(slug string) {
	if slug == "" {
		return
	}
	os.Remove(armedFile(slug))
	for _, key := range []string{"brief", "profile", "exemplars"} {
		os.Remove(readMarkFile(slug, key))
	}
}

// isSessionMark reports whether name is one of the gate's own session
// marks under paths.StateDir(): an armed file (slug+".armed") or a read
// mark (slug+".read-"+key). pruneState touches only these, never another
// file something else in Byline put in state/, such as rules-saved (M1).
func isSessionMark(name string) bool {
	return strings.HasSuffix(name, ".armed") || strings.Contains(name, ".read-")
}

// pruneDir removes every entry in dir older than maxAge for which
// prunable reports true. Does nothing, without error, when dir doesn't
// exist yet.
func pruneDir(dir string, maxAge time.Duration, prunable func(name string) bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-maxAge)
	for _, e := range entries {
		if !prunable(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// pruneState removes the gate's own session marks (see isSessionMark)
// under paths.StateDir() older than maxAge, and nothing else (M1).
func pruneState(maxAge time.Duration) {
	pruneDir(paths.StateDir(), maxAge, isSessionMark)
}

// pruneReceipts removes every structure-check receipt older than maxAge,
// so ~/.byline/receipts doesn't keep every checked message's words
// forever when only the last receiptLifetime is ever read back (M7).
func pruneReceipts(maxAge time.Duration) {
	pruneDir(paths.ReceiptsDir(), maxAge, func(string) bool { return true })
}
