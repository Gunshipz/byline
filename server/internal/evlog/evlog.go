// Package evlog is the tiny shared helper behind Byline's two local
// observability logs (BYLINE_HOME/logs/tool-calls.jsonl and
// gate-decisions.jsonl -- a local stand-in for a central logging
// integration, which Go doesn't have yet): append one JSON line, folder
// and file locked down, rolled over once a log grows past its cap.
package evlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// maxBytes is where a log rolls to its ".1" file and starts fresh.
const maxBytes = 5 << 20 // 5 MiB

// Append JSON-encodes v and appends it as one line to path, creating
// path's folder (0700) and the file itself (0600) as needed, and first
// rolling path to path+".1" (replacing any older one) once path is
// already at or past maxBytes. Any error along the way -- the folder or
// file can't be made, written, or rolled over -- drops the line silently:
// this log must never change or slow down whatever it's recording.
func Append(path string, v any) {
	line, err := json.Marshal(v)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	if info, err := os.Stat(path); err == nil && info.Size() >= maxBytes {
		rotate(path)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(append(line, '\n'))
}

// rotate rolls path to path+".1" (replacing any older one). Two hooks can
// write at once, so a lock file lets exactly one writer roll over, and it
// checks the size again under the lock: without that, a second writer
// would roll the fresh file over the history the first one just saved.
// A writer that finds the lock taken just appends to the current file.
func rotate(path string) {
	lock := path + ".lock"
	l, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if info, err := os.Stat(lock); err == nil && time.Since(info.ModTime()) > 10*time.Second {
			os.Remove(lock) // left by a writer that crashed mid-rollover
		}
		return
	}
	l.Close()
	defer os.Remove(lock)
	if info, err := os.Stat(path); err != nil || info.Size() < maxBytes {
		return
	}
	os.Remove(path + ".1") // ignored: fine if there's no older one to replace
	os.Rename(path, path+".1")
}
