package content

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const shingleLen = 12

var shingleWordRe = regexp.MustCompile(`[a-z0-9']+`)

// wordsOf lowercases text and splits it into words.
func wordsOf(text string) []string {
	return shingleWordRe.FindAllString(strings.ToLower(text), -1)
}

// shingleSet returns every run of n consecutive words in words, as a set.
func shingleSet(words []string, n int) map[string]bool {
	set := map[string]bool{}
	for i := 0; i+n <= len(words); i++ {
		set[strings.Join(words[i:i+n], " ")] = true
	}
	return set
}

// TestSharedPart_NoLongRunsFromSources fails if any shared-part file
// contains a 12-word run copied from one of the licence source texts under
// $BYLINE_PRIVATE_FIXTURES/legacy/sources.
func TestSharedPart_NoLongRunsFromSources(t *testing.T) {
	dir := privateDir(t)
	sourcesDir := filepath.Join(dir, "legacy", "sources")
	entries, err := os.ReadDir(sourcesDir)
	if err != nil {
		t.Fatalf("read %s: %v", sourcesDir, err)
	}

	type source struct {
		name string
		set  map[string]bool
	}
	var sources []source
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(sourcesDir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		sources = append(sources, source{e.Name(), shingleSet(wordsOf(string(data)), shingleLen)})
	}

	walkTextFiles(t, repoRoot(t), func(rel, abs string) {
		data, err := os.ReadFile(abs)
		if err != nil {
			t.Fatalf("read %s: %v", abs, err)
		}
		words := wordsOf(string(data))
		for _, src := range sources {
			for i := 0; i+shingleLen <= len(words); i++ {
				run := strings.Join(words[i:i+shingleLen], " ")
				if src.set[run] {
					t.Errorf("%s: shares %q with %s", rel, run, src.name)
					break
				}
			}
		}
	})
}
