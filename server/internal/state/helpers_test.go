package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"byline/embed"
)

// Shared across this package's test files.

func want(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

// testHome sets BYLINE_HOME to a new temp dir and returns it.
func testHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("BYLINE_HOME", home)
	return home
}

func writeFile(t *testing.T, path, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
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

// tmpl returns the bytes of embed.Files templates/<name>.
func tmpl(t *testing.T, name string) string {
	t.Helper()
	data, err := embed.Files.ReadFile("templates/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// glob returns the paths matching pattern, sorted.
func glob(t *testing.T, pattern string) []string {
	t.Helper()
	m, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(m)
	return m
}

// assertDir fails unless path is a directory with exactly perm.
func assertDir(t *testing.T, path string, perm os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", path)
	}
	if info.Mode().Perm() != perm {
		t.Fatalf("%s has mode %o, want %o", path, info.Mode().Perm(), perm)
	}
}

// assertNoFile fails if path exists.
func assertNoFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("%s exists (stat err=%v)", path, err)
	}
}

// firstLine returns path's first line, without its trailing newline.
func firstLine(t *testing.T, path string) string {
	t.Helper()
	data := readFile(t, path)
	if i := strings.IndexByte(data, '\n'); i >= 0 {
		return data[:i]
	}
	return data
}

// mtime returns path's modification time.
func mtime(t *testing.T, path string) time.Time {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.ModTime()
}

// backdate sets each path's modification time d before now.
func backdate(t *testing.T, d time.Duration, paths ...string) {
	t.Helper()
	when := time.Now().Add(-d)
	for _, p := range paths {
		if err := os.Chtimes(p, when, when); err != nil {
			t.Fatal(err)
		}
	}
}

// touch sets path's modification time to now.
func touch(t *testing.T, path string) {
	t.Helper()
	now := time.Now()
	if err := os.Chtimes(path, now, now); err != nil {
		t.Fatal(err)
	}
}

// allFiles returns every regular file under dir, recursively.
func allFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// snapshotTimes maps every file and directory under dir to its
// modification time, so a later pass can check that nothing changed.
func snapshotTimes(t *testing.T, dir string) map[string]time.Time {
	t.Helper()
	out := map[string]time.Time{}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		out[path] = info.ModTime()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// ruleJSON renders a minimal valid add_rules entry with the given id,
// retired or not.
func ruleJSON(id string, retired bool) string {
	return fmt.Sprintf(`{"id":%q,"family":1,"name":"n","ban":"b","strings":["x"],"fix":"f","bad":"a","good":"c","retired":%t}`, id, retired)
}
