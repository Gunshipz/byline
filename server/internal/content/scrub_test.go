package content

import (
	"bufio"
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"
)

// skipName reports whether a file or directory with this base name is
// skipped by the shared-part scans: .git (a directory in a normal clone, a
// plain file in a linked worktree), everything this repo's own
// .gitignore excludes (/build/, /dist/, /.superpowers/), and the
// maintainer's untracked TASKS.md.
func skipName(name string) bool {
	return name == ".git" || name == "build" || name == "dist" || name == ".superpowers" || name == "TASKS.md"
}

// hasNUL reports whether the first 8,000 bytes of path contain a NUL byte,
// the signal used here to treat a file as binary and skip it.
func hasNUL(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	buf := make([]byte, 8000)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return false, err
	}
	return bytes.IndexByte(buf[:n], 0) >= 0, nil
}

// walkTextFiles calls fn, with the path relative to root and the absolute
// path, for every regular file under root except those skipName excludes
// by name or hasNUL excludes as binary.
func walkTextFiles(t *testing.T, root string, fn func(relPath, absPath string)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if skipName(d.Name()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		bin, err := hasNUL(path)
		if err != nil {
			t.Fatalf("walkTextFiles: read %s: %v", path, err)
		}
		if bin {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		fn(rel, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walkTextFiles: %v", err)
	}
}

// loadLines reads path and returns its non-empty lines.
func loadLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("loadLines: read %s: %v", path, err)
	}
	var lines []string
	for _, l := range strings.Split(string(data), "\n") {
		l = strings.TrimRight(l, "\r")
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

// matcher pairs a scrub term or pattern with the compiled regex that
// finds it. isTerm is true only for a scrub-terms.txt entry (never a
// scrub-patterns.txt regex), the ones anyTokenEquals also checks.
type matcher struct {
	label  string
	re     *regexp.Regexp
	isTerm bool
}

// isAcronym reports whether term is written in all capitals. An acronym
// matches case-sensitively as a whole word; every other term still
// matches case-insensitively.
func isAcronym(term string) bool {
	return term == strings.ToUpper(term) && term != strings.ToLower(term)
}

// termMatcher compiles the whole-word regex for one scrub term:
// case-sensitive for an all-capitals acronym, case-insensitive otherwise.
func termMatcher(term string) (*regexp.Regexp, error) {
	flag := "(?i)"
	if isAcronym(term) {
		flag = ""
	}
	return regexp.Compile(flag + `(^|[^\p{L}\p{N}_])` + regexp.QuoteMeta(term) + `($|[^\p{L}\p{N}_])`)
}

// splitIdentifierTokens returns every sub-word of every identifier-like
// run (letters, digits, underscore) in text that an underscore or a
// lowercase-to-uppercase transition actually splits into two or more
// pieces — the two boundaries termMatcher's [^\p{L}\p{N}_] word-boundary
// class can't see on its own, since an underscore counts as a word
// character and a case change within one run of letters isn't a boundary
// at all. A hyphen (or any other punctuation) is already a boundary
// under that class, so it needs no extra handling here. A run that isn't
// actually split (an ordinary word, regardless of its own case) returns
// nothing here — it's already covered, correctly, by termMatcher's own
// acronym case rule, and returning it again as a trivial one-piece
// "split" would bypass that rule and reintroduce the false positive the
// acronym case distinction exists to prevent (an all-capitals acronym
// matching an unrelated, ordinary lowercase word spelled the same way).
func splitIdentifierTokens(text string) []string {
	var tokens []string
	var run []rune
	flushRun := func() {
		if len(run) == 0 {
			return
		}
		var pieces []string
		var cur []rune
		for i, r := range run {
			if r == '_' {
				if len(cur) > 0 {
					pieces = append(pieces, string(cur))
					cur = nil
				}
				continue
			}
			if i > 0 && unicode.IsLower(run[i-1]) && unicode.IsUpper(r) && len(cur) > 0 {
				pieces = append(pieces, string(cur))
				cur = nil
			}
			cur = append(cur, r)
		}
		if len(cur) > 0 {
			pieces = append(pieces, string(cur))
		}
		if len(pieces) > 1 {
			tokens = append(tokens, pieces...)
		}
		run = nil
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			run = append(run, r)
		} else {
			flushRun()
		}
	}
	flushRun()
	return tokens
}

// anyTokenEquals reports whether term case-insensitively equals one
// whole sub-token line splits into — the identifier-boundary check that
// runs alongside termMatcher's regex, for scrub-terms.txt entries only
// (never a scrub-patterns.txt regex, which already defines its own
// boundaries).
func anyTokenEquals(term, line string) bool {
	for _, tok := range splitIdentifierTokens(line) {
		if strings.EqualFold(tok, term) {
			return true
		}
	}
	return false
}

// creditLine reports whether line is the maintainer's own credit, the one
// place a name may appear: a copyright line in LICENSE, or the author or
// owner field of the plugin and marketplace manifests.
func creditLine(rel, line string) bool {
	switch filepath.ToSlash(rel) {
	case "LICENSE":
		return strings.HasPrefix(line, "Copyright (c) ")
	case ".claude-plugin/plugin.json", ".claude-plugin/marketplace.json":
		t := strings.TrimSpace(line)
		return strings.HasPrefix(t, `"author": {`) || strings.HasPrefix(t, `"owner": {`)
	}
	return false
}

// TestSharedPart_NoScrubTerms fails if any file in the shared part of the
// repo (everywhere outside ~/.byline/ and the private fixtures) contains a
// scrub term or matches a scrub pattern, except on a creditLine. Matching
// is case-insensitive everywhere.
func TestSharedPart_NoScrubTerms(t *testing.T) {
	dir := privateDir(t)
	terms := loadLines(t, filepath.Join(dir, "scrub-terms.txt"))
	patterns := loadLines(t, filepath.Join(dir, "scrub-patterns.txt"))

	var matchers []matcher
	for _, term := range terms {
		re, err := termMatcher(term)
		if err != nil {
			t.Fatalf("bad scrub term %q: %v", term, err)
		}
		matchers = append(matchers, matcher{term, re, true})
	}
	for _, pat := range patterns {
		re, err := regexp.Compile(`(?i)` + pat)
		if err != nil {
			t.Fatalf("bad scrub pattern %q: %v", pat, err)
		}
		matchers = append(matchers, matcher{pat, re, false})
	}

	walkTextFiles(t, repoRoot(t), func(rel, abs string) {
		data, err := os.ReadFile(abs)
		if err != nil {
			t.Fatalf("read %s: %v", abs, err)
		}
		lineNo := 0
		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			if creditLine(rel, line) {
				continue
			}
			for _, m := range matchers {
				if m.re.MatchString(line) || (m.isTerm && anyTokenEquals(m.label, line)) {
					t.Errorf("%s:%d: %q", rel, lineNo, m.label)
				}
			}
		}
		if err := scanner.Err(); err != nil {
			t.Errorf("%s: scan stopped early: %v", rel, err)
		}
	})
}

// TestSharedPart_AcronymScrubTermsAreCaseSensitive is a focused
// regression: a scrub term written in all capitals matches
// case-sensitively as a whole word, so the same letters in lowercase (an
// ordinary word) are not a false hit; every other term still matches
// case-insensitively.
func TestSharedPart_AcronymScrubTermsAreCaseSensitive(t *testing.T) {
	// Built by concatenation, not written as a literal, so this
	// regression test doesn't trip the very scan it exercises.
	acronym := "GO" + "ES"
	name := "Kev" + "in"

	goes, err := termMatcher(acronym)
	if err != nil {
		t.Fatal(err)
	}
	nameRe, err := termMatcher(name)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		re   *regexp.Regexp
		line string
		want bool
	}{
		{goes, "the launcher " + acronym + " to the next step", true},
		{goes, "the launcher " + strings.ToLower(acronym) + " to the next step", false},
		{nameRe, "ask " + name + " about it", true},
		{nameRe, "ask " + strings.ToLower(name) + " about it", true},
	}
	for _, c := range cases {
		if got := c.re.MatchString(c.line); got != c.want {
			t.Errorf("%s against %q = %v, want %v", c.re, c.line, got, c.want)
		}
	}
}

// TestSharedPart_ScrubTermsCatchIdentifierBoundaries is a focused
// regression for the identifier gap: a scrub term glued into an
// identifier with no space or punctuation around it — snake_case
// (an underscore) or camelCase (a lowercase-to-uppercase transition) —
// is still caught, the same as if it had its own word boundary. An
// ordinary word that merely contains the term as a substring, with no
// such split point, is still not a false hit.
func TestSharedPart_ScrubTermsCatchIdentifierBoundaries(t *testing.T) {
	// Built by concatenation, not written as a literal, so this
	// regression test doesn't trip the very scan it exercises.
	acronym := "GO" + "ES"
	name := "Kev" + "in"

	cases := []struct {
		term string
		line string
		want bool
	}{
		{acronym, "the launcher " + strings.ToLower(acronym) + "_board reports status", true},
		{acronym, "the launcher " + strings.ToLower(acronym) + "Board reports status", true},
		{name, strings.ToLower(name) + "_handler ran the job", true},
		{name, strings.ToLower(name) + "Handler ran the job", true},
		{name, "ask " + strings.ToLower(name) + "ish about it", false},
	}
	for _, c := range cases {
		re, err := termMatcher(c.term)
		if err != nil {
			t.Fatal(err)
		}
		got := re.MatchString(c.line) || anyTokenEquals(c.term, c.line)
		if got != c.want {
			t.Errorf("%q against %q = %v, want %v", c.term, c.line, got, c.want)
		}
	}
}

// TestWalkTextFiles_SkipsGitIgnoredPaths is a focused regression: the
// shared-part scans must skip exactly what this repo's own .gitignore
// excludes (/.superpowers/, /build/, /dist/), the same as git itself would.
func TestWalkTextFiles_SkipsGitIgnoredPaths(t *testing.T) {
	root := t.TempDir()
	write := func(rel string) {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("placeholder\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".superpowers/sdd/build-plan.md")
	write("build/output.txt")
	write("dist/bundle.txt")
	write("kept.md")

	var visited []string
	walkTextFiles(t, root, func(rel, _ string) {
		visited = append(visited, rel)
	})

	for _, skipped := range []string{".superpowers/sdd/build-plan.md", "build/output.txt", "dist/bundle.txt"} {
		for _, v := range visited {
			if v == skipped {
				t.Errorf("walkTextFiles visited %q, want it skipped", skipped)
			}
		}
	}
	kept := false
	for _, v := range visited {
		if v == "kept.md" {
			kept = true
		}
	}
	if !kept {
		t.Error("walkTextFiles did not visit kept.md, want it visited")
	}
}

// TestSharedPart_NoCompanyHosts keeps the shared part free of any
// company-internal host name, so nothing in this public repo points at a
// system outsiders can't reach. The pattern is split so this file
// doesn't match itself.
func TestSharedPart_NoCompanyHosts(t *testing.T) {
	re := regexp.MustCompile(`(?i)\b(ama` + `zon|a2` + `z|aws\.` + `dev)\b`)
	walkTextFiles(t, repoRoot(t), func(rel, abs string) {
		data, err := os.ReadFile(abs)
		if err != nil {
			t.Fatalf("read %s: %v", abs, err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if m := re.FindString(line); m != "" {
				t.Errorf("%s:%d: %q names a company-internal system", rel, i+1, m)
			}
		}
	})
}
