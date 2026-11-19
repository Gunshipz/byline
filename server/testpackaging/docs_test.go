package testpackaging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNotices_LicenseIsUpstreamMIT(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "LICENSE"))
	if err != nil {
		t.Fatalf("read LICENSE: %v", err)
	}
	text := string(data)

	firstLine := strings.SplitN(text, "\n", 2)[0]
	if firstLine != "MIT License" {
		t.Fatalf("LICENSE first line = %q, want %q", firstLine, "MIT License")
	}
	for _, want := range []string{
		"Copyright (c) 2025 Siqi Chen",
		"Permission is hereby granted, free of charge",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("LICENSE missing %q", want)
		}
	}
}

func TestNotices_CreditsAndModificationNotice(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "THIRD-PARTY-NOTICES"))
	if err != nil {
		t.Fatalf("read THIRD-PARTY-NOTICES: %v", err)
	}
	text := string(data)

	want := []string{
		"https://github.com/blader/humanizer",
		"Copyright (c) 2025 Siqi Chen",
		"MIT License",
		"Byline is a modified version of humanizer",
		"https://en.wikipedia.org/wiki/Wikipedia:Signs_of_AI_writing",
	}
	want = append(want, thirdPartyNoticesExtra...)
	for _, w := range want {
		if !strings.Contains(text, w) {
			t.Fatalf("THIRD-PARTY-NOTICES missing %q", w)
		}
	}
}

// docSectionCase is one file TestDocs_RequiredSections checks, and the
// headings and literal phrases it must contain.
type docSectionCase struct {
	rel  string
	want []string
}

// TestDocs_RequiredSections checks that this copy's own docs each contain
// every heading and literal phrase Task 6.9 requires, so a doc edit that
// drops one fails loudly here instead of silently in a person's hands.
func TestDocs_RequiredSections(t *testing.T) {
	root := repoRoot(t)
	for _, c := range requiredSectionsCases {
		data, err := os.ReadFile(filepath.Join(root, c.rel))
		if err != nil {
			t.Fatalf("read %s: %v", c.rel, err)
		}
		text := string(data)
		for _, want := range c.want {
			if !strings.Contains(text, want) {
				t.Errorf("%s missing %q", c.rel, want)
			}
		}
	}
}

func TestVersion_ChangelogTopEntryIsCurrentVersion(t *testing.T) {
	root := repoRoot(t)
	version := skillVersion(t, root)

	data, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if err != nil {
		t.Fatalf("read CHANGELOG.md: %v", err)
	}

	var topHeading string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "## ") {
			topHeading = line
			break
		}
	}
	if topHeading == "" {
		t.Fatalf("CHANGELOG.md has no line starting %q", "## ")
	}
	fields := strings.Fields(topHeading)
	if len(fields) < 2 {
		t.Fatalf("CHANGELOG.md top heading %q has no second field", topHeading)
	}
	if fields[1] != version {
		t.Fatalf("CHANGELOG.md top entry version = %q, want %q (skill.json version)", fields[1], version)
	}
}

// TestReleaseNotes_CurrentVersionHasNotes: every release ships
// docs/release-notes/<version>.md for this copy's readers, a step in the
// release checklist, and it never names the other copy.
func TestReleaseNotes_CurrentVersionHasNotes(t *testing.T) {
	root := repoRoot(t)
	version := skillVersion(t, root)
	data, err := os.ReadFile(filepath.Join(root, "docs", "release-notes", version+".md"))
	if err != nil {
		t.Fatalf("no release notes for %s: %v", version, err)
	}
	if !strings.HasPrefix(string(data), "# Byline "+version+"\n") {
		t.Fatalf("release notes for %s must open with %q", version, "# Byline "+version)
	}
	if strings.Contains(strings.ToLower(string(data)), otherCopyWord) {
		t.Fatalf("release notes for %s name the other copy; they're for this copy's readers only", version)
	}
}
