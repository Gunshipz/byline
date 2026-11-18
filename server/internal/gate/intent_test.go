package gate

import (
	"strings"
	"testing"
)

func TestDetectDraftIntent_Patterns(t *testing.T) {
	for _, p := range []string{
		"write a quick note for the vendor",
		"create a cold outreach outline for a slack message", // the 2026-08-25 miss
		"respond to the vendor's question",
		"reach out to the vendor about the renewal",
		"send them a quick email",
		"can you get a message to the vendor today",
		"tell the team the launch moved",
		"post in #general that the build is green",
		"do my weekly update",
		"fill out my standup",
		"1on1 prep for tomorrow",
		"prep me for the meeting with finance",
		"DRAFT A REPLY",
	} {
		if !detectDraftIntent(p, nil) {
			t.Errorf("%q: want a draft request", p)
		}
	}
	for _, p := range []string{"", "what did the vendor say about the new accounts"} {
		if detectDraftIntent(p, nil) {
			t.Errorf("%q: want not a draft request", p)
		}
	}
}

func TestDetectDraftIntent_SkipsComeFirst(t *testing.T) {
	for _, p := range []string{
		"create a hook that forces you to use the humanizer before you draft a message",
		"update settings.json so a draft message is checked",
		"write a 6-pager on the filing archive",
		"draft the PR FAQ announcement",
		"draft a post-mortem follow-up",
		"write the design doc for the importer",
	} {
		if detectDraftIntent(p, nil) {
			t.Errorf("%q: the skip must win", p)
		}
	}
	if detectDraftIntent("tell the vendor about the hook", []string{"tell the vendor"}) {
		t.Error("extra phrases must not beat the skips")
	}
}

// Several of these would also arm the gate on text alone (pattern 1, via a
// draft-shaped word after the mode name), which is what actually proves the
// new skip does the work rather than the prompt just missing every pattern
// on its own.
func TestDetectDraftIntent_BylineModeCommandsSkip(t *testing.T) {
	for _, p := range []string{
		"/byline:byline update",
		"/byline:byline update now please",
		"/byline:byline update create a message explaining this to the team",
		"/byline:byline setup",
		"/byline:byline setup create a quick email for onboarding",
		"/byline:byline flag this phrase",
		"/byline:byline check write a reply to the vendor",
		"/byline:byline review draft a response for the customer",
		"  /byline:byline UPDATE",
		"/BYLINE:BYLINE update create an announcement",
	} {
		if detectDraftIntent(p, nil) {
			t.Errorf("%q: /byline:byline's own mode command must skip", p)
		}
	}
}

func TestDetectDraftIntent_BylineModeSkipDoesNotWiden(t *testing.T) {
	// "/byline:byline" and "/byline:byline polish" must keep today's
	// result: neither one is a draft request now, and the new skip must
	// not touch that.
	for _, p := range []string{"/byline:byline", "/byline:byline polish"} {
		if detectDraftIntent(p, nil) {
			t.Errorf("%q: must not newly count as a draft request", p)
		}
	}
	// A plain writing request must keep arming the gate, same as before.
	if !detectDraftIntent("draft a quick note for the vendor", nil) {
		t.Error("a plain writing request must still count as a draft request")
	}
}

func TestDetectDraftIntent_NoTeamNamesByDefault(t *testing.T) {
	if detectDraftIntent("tell the vendor the plan", nil) {
		t.Error("a bare 'tell <someone>' must not arm by default")
	}
	if !detectDraftIntent("tell the team the plan", nil) {
		t.Error("'tell the team' still arms")
	}
	const want = `(?i)\b(send|post|tell|message|slack|email)\b\s+(the\s+team)\b`
	if got := draftPatterns[5].String(); got != want {
		t.Errorf("pattern 6 = %s, want %s", got, want)
	}
}

func TestDetectDraftIntent_ExtraPhrases(t *testing.T) {
	if !detectDraftIntent("Tell the   vendor the plan", []string{" tell THE vendor "}) {
		t.Error("a phrase matches regardless of case and spacing")
	}
	if detectDraftIntent("anything at all", []string{"", "   "}) {
		t.Error("blank phrases are ignored")
	}
}

func TestIsMachineTurn(t *testing.T) {
	for p, want := range map[string]bool{
		"<system-reminder>\nAgent \"drafter\" completed: draft a reply": true,
		"  \n<system-reminder>x":                                               true,
		" <system-reminder>x":                                                  true, // JS \s includes no-break space
		"please quote <system-reminder> later":                                 false,
		"[SYSTEM NOTIFICATION - NOT USER INPUT] Agent finished.":               true,
		"x <task-notification><status>completed</status>":                      true,
		"Another Claude session sent a message: <agent-message from=\"a\"> hi": true,
		"<agent-messages> is not the marker":                                   false,
		"[Subagent hand-back] done":                                            true,
		strings.Repeat("a", 400) + "[SYSTEM NOTIFICATION":                      false, // only the first 400 characters count
		strings.Repeat("é", 300) + "<task-notification>":                       true,  // characters, not bytes
		"": false,
	} {
		if got := isMachineTurn(p); got != want {
			t.Errorf("%.40q: got %v, want %v", p, got, want)
		}
	}
}
