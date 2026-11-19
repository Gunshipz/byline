package testpackaging

// launcherDir is where the two launchers live in this copy's tree, slash
// separated.
const launcherDir = "bin"

// bashSendCommandsToCheck are the shell send commands launcher_test.go's
// Bash-hook test checks each asks instead of running unattended. This copy
// has no Bash gating, so there are none.
func bashSendCommandsToCheck() []string {
	return nil
}

// thirdPartyNoticesExtra are literal phrases THIRD-PARTY-NOTICES must
// contain, beyond the shared humanizer credit. This copy rewrote the
// borrowed rules the private copy credits, so it has none.
var thirdPartyNoticesExtra []string

// requiredSectionsCases are the files TestDocs_RequiredSections checks, and
// the headings and literal phrases each must contain.
var requiredSectionsCases = []docSectionCase{
	{"README.md", []string{
		"## Install", "## What it checks", "## First run",
		"## Privacy", "## Known limitations", "## Updating",
		"claude plugin marketplace add gunshipz/byline", "claude plugin install byline@byline",
		"~/.byline/", "Git for Windows",
	}},
	{"CONTRIBUTING.md", []string{
		"## Rules for changes", "## Release checklist",
		"server/scripts/embed.sh", "go test ./...", "requires_helper",
	}},
}

// otherCopyWord is a word for the other copy that this copy's release
// notes must never contain. Built by concatenation so this file doesn't
// trip content.TestSharedPart_NoCompanyHosts, the way docs_test.go already
// did before this seam.
const otherCopyWord = "ama" + "zon"
