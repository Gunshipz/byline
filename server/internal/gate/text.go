package gate

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"byline/internal/paths"
)

const (
	malformedDenyReason   = "Byline couldn't read this request, so it stopped it to be safe. Try again; if it keeps happening, reinstall Byline."
	crashAskReason        = "Byline's checks couldn't run, so approve this step yourself, or reinstall Byline." // M5: also fits a polish step, and ends with a period.
	setupTemplateMarker   = "<!-- byline template"
	missingSystemMessage  = "Byline isn't set up yet, so its writing checks have nothing to work from. Ask Claude to set up Byline."
	templateSystemMessage = "Byline hasn't learned your writing voice yet. Ask Claude to set up Byline."
	templateContext       = "Byline is still using blank templates: the person hasn't run setup, so drafts won't sound like them yet. The first time they ask for a draft, offer setup once."
)

// promptContext is the context UserPromptSubmit injects on a draft request:
// what to read, and the checks a send must pass.
func promptContext() string {
	return fmt.Sprintf("Byline: this is a request to write a message. Before you write any of it, read these three files with the Read tool: "+
		"the writing brief at %s, the person's writing profile at %s, and their sample emails at %s. "+
		"Write the draft with all three loaded. Check every number, name, date and quote against its source yourself. "+
		"Before anything is sent, write a spec for the draft and pass Byline's structure check on it; "+
		"a send is stopped until that check passes on the same words. "+
		"Finish with Byline's polish pass, and show the person only the final version.",
		paths.BriefPath(), paths.ProfilePath(), paths.ExemplarsPath())
}

// missingContext names the voice-folder files session-start couldn't find.
func missingContext(names []string) string {
	return fmt.Sprintf("Byline isn't fully set up on this computer. Missing from %s: %s. "+
		"Before drafting anything, tell the person and offer to run Byline setup.",
		paths.Home(), strings.Join(names, ", "))
}

const (
	slackAskReason      = "Byline asks you to approve every Slack post to a person."
	slackExemptReason   = "This chat is on Byline's list of bot chats, so it goes through without asking."
	unseenTextAskReason = "Byline can't see the message in this send, so it can't check it. Approve it yourself if it's right."
)

// receiptsFolderAskReason explains that Byline can't save or read its
// structure-check receipts at dir, so a check can never pass (I3) --
// this is an ask, not the "no check has passed" deny, since trying
// again can't fix it.
func receiptsFolderAskReason(dir string) string {
	return fmt.Sprintf("Byline can't save its structure checks at %s, so approve this send yourself, or fix that folder.", dir)
}

// bodyFileAskReason explains that the hook couldn't read path, the file
// a bodyFilePath named (M6), so it has no way to check this send.
func bodyFileAskReason(path string) string {
	return fmt.Sprintf("Byline couldn't read the file at %s, so it can't check this send. Approve it yourself, or fix the file and try again.", path)
}

// structureDenyReason explains why a send was stopped for not having a
// fresh, matching structure-check receipt. coverage and unmatched are 0
// and wordCount when no receipt was found at all; otherwise they're the
// closest receipt's share of its own words found in the message (M4:
// rounded down) and that receipt's count of the message's words it never
// saw, out of wordCount total (I1: content added after the check,
// including a cut kill-list item, must not go out unchecked).
func structureDenyReason(coverage float64, unmatched, wordCount int) string {
	mins := int(receiptLifetime.Minutes())
	detail := fmt.Sprintf("No structure check has passed in the last %d minutes.", mins)
	if !(coverage == 0 && unmatched == wordCount) {
		var parts []string
		if coverage < coverageNeeded {
			// M4: the phrase was backwards (this is the share of the
			// checked draft found in this copy, not the reverse), and
			// %.0f rounds to the nearest percent instead of down.
			parts = append(parts, fmt.Sprintf("only %.0f%% of the checked draft is in this copy, and it needs %.0f%%",
				math.Floor(coverage*100), coverageNeeded*100))
		}
		if budget := extraWordBudget(wordCount); unmatched > budget {
			parts = append(parts, fmt.Sprintf("this copy has %d words the check never saw, and it allows at most %d new ones", unmatched, budget))
		}
		detail = fmt.Sprintf("The closest check from the last %d minutes: %s. The copy changed after it was checked, or a different draft was checked.",
			mins, strings.Join(parts, "; "))
	}
	return "Byline stopped this send: no structure check has passed on this copy.\n" + detail + "\n\n" +
		"The structure check catches what a word check can't: content the person already cut coming back, " +
		"a draft over its word budget, and sections that don't answer the reader's questions.\n\n" +
		"To send it:\n" +
		"1. Write a spec next to the draft: who will read it, what it's for, the 2 to 4 questions that reader will ask, " +
		"a word budget, the sections (none for a short chat message), and a kill list of everything the person has cut.\n" +
		"2. Run Byline's structure check on the draft and its spec.\n" +
		"3. Fix every failure. Each time the person cuts something, add it to the kill list and run the check again.\n" +
		"4. Try the send again once the check passes."
}

// writingDenySystemMessage is shown to the person when a polish pass or a
// send is stopped because the three writing files haven't been read yet.
const writingDenySystemMessage = "Byline paused this until your writing brief, profile and sample emails are read."

// writingDenyReason names, in requiredFiles order, which of the required
// files missing hasn't been read yet, and what to do about it.
func writingDenyReason(missing []string) string {
	var b strings.Builder
	b.WriteString("Byline stopped this step. This is a writing request, and these files haven't been read yet in this conversation:\n")
	for _, f := range requiredFiles {
		if slices.Contains(missing, f.key) {
			fmt.Fprintf(&b, "  - %s: %s\n", f.name, f.path())
		}
	}
	b.WriteString("\nRead each one with the Read tool (reading it another way doesn't count), write the draft with them loaded, then try again. " +
		"Writing first and polishing afterwards is the exact failure this check stops, so trying again without reading them will be stopped again.")
	return b.String()
}
