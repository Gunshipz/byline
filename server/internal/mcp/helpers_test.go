package mcp

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"byline/internal/rules"
	"byline/internal/state"
)

// Shared across this package's test files (later tasks use them too).

func want(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

// rpc sends each line, newline-terminated, to a fresh run of s.Serve and
// returns each parsed JSON-RPC reply, in order.
func rpc(t *testing.T, s *Server, lines ...string) []map[string]any {
	t.Helper()
	in := strings.Join(lines, "\n") + "\n"
	var out bytes.Buffer
	if err := s.Serve(strings.NewReader(in), &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	var replies []map[string]any
	for _, line := range strings.Split(strings.TrimRight(out.String(), "\n"), "\n") {
		if line == "" {
			continue
		}
		dec := json.NewDecoder(strings.NewReader(line))
		dec.UseNumber()
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			t.Fatalf("bad reply line %q: %v", line, err)
		}
		replies = append(replies, m)
	}
	return replies
}

// callTool sends initialize then a tools/call for name/args and returns the
// decoded result (if its text is JSON), whether it's an error, and the raw
// text a tools/call result carries.
func callTool(t *testing.T, s *Server, name string, args map[string]any) (out map[string]any, isError bool, text string) {
	t.Helper()
	params, err := json.Marshal(map[string]any{"name": name, "arguments": args})
	if err != nil {
		t.Fatal(err)
	}
	replies := rpc(t, s, initLine, fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":%s}`, params))
	if len(replies) < 2 {
		t.Fatalf("expected 2 replies, got %d: %v", len(replies), replies)
	}
	result, ok := replies[1]["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result in %v", replies[1])
	}
	content, _ := result["content"].([]any)
	if len(content) > 0 {
		text, _ = content[0].(map[string]any)["text"].(string)
	}
	isError, _ = result["isError"].(bool)
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	dec.Decode(&out)
	return out, isError, text
}

// setupHome sets BYLINE_HOME, HOME and USERPROFILE to fresh temp dirs,
// clears this copy's own environment knobs, and makes the voice folder.
// It returns BYLINE_HOME.
func setupHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("BYLINE_HOME", home)
	hd := t.TempDir()
	t.Setenv("HOME", hd)
	t.Setenv("USERPROFILE", hd)
	clearCopyEnv(t)
	if err := state.EnsureVoiceFolder(); err != nil {
		t.Fatal(err)
	}
	return home
}

// snapshot maps every file under dir, by its path relative to dir, to the
// hex SHA-256 of its contents. Part 2's logs/ folder is skipped: every
// tool call appends to it, read-only tools included, so it's never part
// of what these tests mean by "nothing changed" (its own fate is
// tool_log_test.go's job).
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != dir && filepath.Base(path) == "logs" {
				return filepath.SkipDir
			}
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		out[rel] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// writeFile writes s to path, 0644, creating path's folder (0755) first.
func writeFile(t *testing.T, path, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// readFile reads path, failing the test on any error.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
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

// decodeUseNumber decodes s as a JSON object, numbers kept as json.Number.
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

// jsonRoundTrip marshals v, then decodes it the same way callTool decodes
// a tool's result (json.Number kept), so a test can compare a checker
// package's return value directly against a tool's output.
func jsonRoundTrip(t *testing.T, v any) any {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

// testdata reads server/testdata/<rel>, relative to this package.
func testdata(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("../../testdata", rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// jsonObj decodes s as a JSON object, for building a tool argument whose
// schema type is "object".
func jsonObj(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// toStrings converts a decoded JSON array of strings ([]any) to []string.
func toStrings(v any) []string {
	arr, _ := v.([]any)
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// assertOneContains fails unless at least one entry of list contains sub.
func assertOneContains(t *testing.T, list []string, sub string) {
	t.Helper()
	for _, s := range list {
		if strings.Contains(s, sub) {
			return
		}
	}
	t.Fatalf("no entry of %v contains %q", list, sub)
}

// firstCoreStringRule returns the first active (non-retired) core rule
// with at least one banned string.
func firstCoreStringRule(t *testing.T) rules.Rule {
	t.Helper()
	set, err := rules.Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range set.Core() {
		if !r.Retired && len(r.Strings) > 0 {
			return r
		}
	}
	t.Fatal("core rules has no active rule with strings")
	return rules.Rule{}
}

// firstTwoCoreStringRules returns the first two distinct active core
// rules that each have at least one banned string, for a test that needs
// one message to trip 2 distinct core rules at once.
func firstTwoCoreStringRules(t *testing.T) (rules.Rule, rules.Rule) {
	t.Helper()
	set, err := rules.Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	var out []rules.Rule
	for _, r := range set.Core() {
		if !r.Retired && len(r.Strings) > 0 {
			out = append(out, r)
			if len(out) == 2 {
				return out[0], out[1]
			}
		}
	}
	t.Fatal("core rules has fewer than 2 active rules with strings")
	return rules.Rule{}, rules.Rule{}
}

// firstRetiredCoreRule returns the first retired core rule, skipping the
// test when core has none.
func firstRetiredCoreRule(t *testing.T) rules.Rule {
	t.Helper()
	set, err := rules.Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range set.Core() {
		if r.Retired {
			return r
		}
	}
	t.Skip("core rules has no retired rule")
	return rules.Rule{}
}

func assertNoFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected no file at %s", path)
	}
}
func toSlice(v any) []any {
	s, _ := v.([]any)
	return s
}
