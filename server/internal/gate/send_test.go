package gate

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"byline/internal/paths"
	"byline/internal/state"
)

func TestNeedsStructureCheck(t *testing.T) {
	s := state.Settings{SendTools: state.DefaultSendTools, ExemptChannels: []string{bot}}
	const slack = "mcp__slack__conversations_add_message"
	for _, c := range []struct {
		tool string
		in   map[string]any
		want bool
	}{
		{"mcp__claude_ai_Microsoft_365__outlook_create_draft", map[string]any{"body": "x"}, true},
		{"mcp__claude_ai_Microsoft_365__outlook_create_reply_draft", map[string]any{}, true},
		{"mcp__claude_ai_Microsoft_365__outlook_send_mail", map[string]any{}, true},
		{"mcp__claude_ai_Microsoft_365__outlook_forward_mail", map[string]any{}, true},
		{"mcp__gmail__draft_email", map[string]any{}, true},
		{slack, map[string]any{"channel": bot}, false},
		{slack, map[string]any{"channelId": bot}, false},
		{slack, map[string]any{"channelId": person, "channel": bot}, true},
	} {
		if got := needsStructureCheck(c.tool, c.in, s); got != c.want {
			t.Errorf("%s %v: got %v, want %v", c.tool, c.in, got, c.want)
		}
	}
}

func TestOutgoingText(t *testing.T) {
	f := filepath.Join(t.TempDir(), "body.html")
	os.WriteFile(f, []byte("from file"), 0o600)
	for _, c := range []struct {
		in   map[string]any
		want string
	}{
		{map[string]any{"text": "a", "body": "b"}, "a"},
		{map[string]any{"body": "b", "message": "c"}, "b"},
		{map[string]any{"message": "c"}, "c"},
		{map[string]any{"text": "", "bodyFilePath": f}, "from file"},
		{map[string]any{"text": 5, "body": "b"}, "b"},
		{map[string]any{"bodyFilePath": "/no/such"}, ""},
	} {
		if got := outgoingText(c.in); got != c.want {
			t.Errorf("%v: got %q, want %q", c.in, got, c.want)
		}
	}
}

// TestExemptChannel_M2_AllPresentFieldsMustBeExempt reproduces M2's
// failing input: channelId alone used to win the bot-chat exemption even
// when channel (the field the real Slack tool actually has) named a
// person. Exemption now requires every channel field present to be on
// the list.
func TestExemptChannel_M2_AllPresentFieldsMustBeExempt(t *testing.T) {
	exempt := []string{bot}
	if exemptChannel(map[string]any{"channelId": bot, "channel": person}, exempt) {
		t.Error("channelId exempt but channel isn't: want not exempt")
	}
	if !exemptChannel(map[string]any{"channelId": bot, "channel": bot}, exempt) {
		t.Error("both fields exempt: want exempt")
	}
	if exemptChannel(map[string]any{}, exempt) {
		t.Error("no channel field at all: want not exempt")
	}
}

func TestLooksLikeComm(t *testing.T) {
	twelve := "One two three four five six. Seven eight nine ten eleven twelve."
	for text, want := range map[string]bool{
		"Thanks, got it.":                         false,
		"Thanks, I will take a look at it today.": false,
		"Reminder to send the vendor docs. Rough is fine, no formatting needed at all.": true,
		shortReal:                   true,
		":+1:":                      false,
		strings.Repeat("word ", 19): false,
		strings.Repeat("word ", 20): true,
		twelve:                      true,
		"One two three four five. Seven eight nine ten eleven twelve.":                  false, // 11 words
		"<p>One two three four five six.</p><p>Seven eight nine ten eleven twelve.</p>": true,
		strings.Replace(twelve, ". ", ". ", 1):                                          true, // no-break space counts, as in JS
		strings.Replace(twelve, ". ", ".\r\n", 1):                                       true,
	} {
		if got := looksLikeComm(text); got != want {
			t.Errorf("%.50q: got %v, want %v", text, got, want)
		}
	}
}

func TestOutgoingWords(t *testing.T) {
	got := outgoingWords("<b>Quarterly</b>&nbsp;budget &amp; the reconciliation's &#160;at it")
	want := map[string]bool{"quarterly": true, "budget": true, "reconciliation's": true}
	if !maps.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestRewriteLinks reproduces I2: Slack's <url|label> and an HTML
// <a href="url">label</a> stripped a tag's label words along with the
// tag, so they never matched what a receipt stored from the raw draft.
func TestRewriteLinks(t *testing.T) {
	got := rewriteLinks(`See <https://a.b/c|the plan> and <a href="https://x.y/z">the doc</a> and <https://bare.example>.`)
	want := `See https://a.b/c the plan and https://x.y/z the doc and https://bare.example.`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestPreTool_I2_SlackLinksMatchAfterCheck reproduces I2's failing input:
// a Slack message using <url|label> links, checked verbatim and then
// sent verbatim, which was wrongly denied because stripping the tag
// whole dropped the label words the receipt had stored.
func TestPreTool_I2_SlackLinksMatchAfterCheck(t *testing.T) {
	newHome(t)
	const linkMessage = "Hi, thanks for flagging this. I pulled the three sources " +
		"<https://example.com/one|first report>, <https://example.com/two|second report>, " +
		"and <https://example.com/three|third report> so you can check the numbers yourself " +
		"before we send anything out to the team."
	addReceipt(t, linkMessage, 0)
	o, _ := hook(t, "pre-tool", map[string]any{"tool_name": "mcp__slack__conversations_add_message",
		"tool_input": map[string]any{"channel": person, "text": linkMessage}})
	if dec(o) != "ask" {
		t.Fatalf("a message checked verbatim, with Slack links, must pass its own structure check: got %+v", o)
	}
}

func TestBestCoverage(t *testing.T) {
	newHome(t)
	w := outgoingWords(bodySlack)
	if cov, unmatched := bestCoverage(w, time.Now()); cov != 0 || unmatched != len(w) {
		t.Fatalf("no receipts folder: got cov=%v unmatched=%v", cov, unmatched)
	}
	addReceipt(t, bodyMD, 0)
	if cov, _ := bestCoverage(w, time.Now()); cov < coverageNeeded {
		t.Fatalf("fresh matching receipt: got %v", cov)
	}
	htmlWords := outgoingWords(bodyHTML())
	if cov, unmatched := bestCoverage(htmlWords, time.Now()); cov < coverageNeeded || unmatched > extraWordBudget(len(htmlWords)) {
		t.Fatalf("HTML and signature must still match: cov=%v unmatched=%v budget=%v", cov, unmatched, extraWordBudget(len(htmlWords)))
	}
	if cov, _ := bestCoverage(w, time.Now().Add(61*time.Minute)); cov != 0 {
		t.Fatalf("a receipt over 60 minutes old must not count: got %v", cov)
	}
	d := paths.ReceiptsDir()
	os.WriteFile(filepath.Join(d, "note.txt"), []byte(`{"words":["alpha","beta","gamma","delta","epsilon","zeta"]}`), 0o600)
	os.WriteFile(filepath.Join(d, "broken.json"), []byte("{"), 0o600)
	os.WriteFile(filepath.Join(d, "tiny.json"), []byte(`{"words":["alpha","beta","gamma","delta","epsilon","alpha"]}`), 0o600)
	if cov, _ := bestCoverage(outgoingWords("alpha beta gamma delta epsilon zeta"), time.Now()); cov != 0 {
		t.Fatalf("non-.json, broken and under-6-word receipts must be ignored: got %v", cov)
	}
}

// TestBestCoverage_I1_UnmatchedWords reproduces I1: a receipt whose words
// are all present in the message passed, even though the message added
// far more new content than the receipt ever saw.
func TestBestCoverage_I1_UnmatchedWords(t *testing.T) {
	newHome(t)
	addReceipt(t, "alpha beta gamma delta epsilon zeta", 0)
	words := outgoingWords("alpha beta gamma delta epsilon zeta plus seven eight nine ten words that were never checked at all today here")
	cov, unmatched := bestCoverage(words, time.Now())
	if cov < coverageNeeded {
		t.Fatalf("the receipt's words are all present: want full coverage, got %v", cov)
	}
	if unmatched == 0 {
		t.Fatalf("want the added words counted as unmatched, got 0")
	}
}

func TestExtraWordBudget(t *testing.T) {
	for _, c := range []struct{ n, want int }{
		{0, 8}, {10, 8}, {53, 8}, {54, 9}, {100, 15},
	} {
		if got := extraWordBudget(c.n); got != c.want {
			t.Errorf("extraWordBudget(%d) = %d, want %d", c.n, got, c.want)
		}
	}
}

// TestPreTool_I1_AddedContentAfterCheckIsDenied reproduces I1's failing
// input: a receipt whose words are all present in a much longer message,
// well past the review's word-budget class, so forward coverage alone
// wrongly passed it.
func TestPreTool_I1_AddedContentAfterCheckIsDenied(t *testing.T) {
	newHome(t)
	addReceipt(t, bodyMD, 0)
	// 30 sentences of genuinely distinct wording (not a repeated template),
	// so the unique content words added comfortably clear any budget.
	words := strings.Fields("harbor lantern velvet orchard thistle granite meadow falcon anchor copper " +
		"willow ember glacier compass driftwood sparrow quartz lagoon cedar prairie ridgeline sable tundra " +
		"violet marble canyon ferry orbit zephyr lumber")
	var extra strings.Builder
	for i, w := range words {
		fmt.Fprintf(&extra, "Sentence %d brings up the %s nobody on the team has reviewed or approved yet. ", i, w)
	}
	text := bodyMD + "\n" + extra.String()
	o, _ := hook(t, "pre-tool", map[string]any{"tool_name": "mcp__claude_ai_Microsoft_365__outlook_send_mail", "tool_input": map[string]any{"body": text}})
	if dec(o) != "deny" {
		t.Fatalf("content added after the check must still be denied: got %+v", o)
	}
	if !strings.Contains(o.HookSpecificOutput.PermissionDecisionReason, "never saw") {
		t.Errorf("deny reason should say plainly the copy has words the check never saw: got %q", o.HookSpecificOutput.PermissionDecisionReason)
	}
}

// TestPreTool_I3_ReceiptsFolderUnusable reproduces I3's failing input:
// BYLINE_HOME under a regular file, so the receipts folder can never be
// created. Before the fix this fell through to the generic "no structure
// check has passed" deny, which trying again can never fix; it must ask
// instead, naming the folder.
func TestPreTool_I3_ReceiptsFolderUnusable(t *testing.T) {
	dir := t.TempDir()
	regularFile := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(regularFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BYLINE_HOME", filepath.Join(regularFile, "byline"))
	o, _ := hook(t, "pre-tool", map[string]any{"tool_name": "mcp__claude_ai_Microsoft_365__outlook_send_mail",
		"tool_input": map[string]any{"body": longUnchecked}})
	if dec(o) != "ask" {
		t.Fatalf("an unusable receipts folder must ask, not deny: got %+v", o)
	}
	if !strings.Contains(o.HookSpecificOutput.PermissionDecisionReason, "can't save its structure checks") {
		t.Errorf("reason should name the folder problem: got %q", o.HookSpecificOutput.PermissionDecisionReason)
	}
}

// TestPreTool_M6_UnreadableBodyFileAsks reproduces M6: a bodyFilePath the
// hook can't read counted as an empty message and passed, even though
// the mail server might still read it. It must ask instead, naming the
// file.
func TestPreTool_M6_UnreadableBodyFileAsks(t *testing.T) {
	newHome(t)
	missing := filepath.Join(t.TempDir(), "missing.html")
	o, _ := hook(t, "pre-tool", map[string]any{"tool_name": "mcp__claude_ai_Microsoft_365__outlook_create_draft",
		"tool_input": map[string]any{"operation": "create", "bodyFilePath": missing}})
	if dec(o) != "ask" {
		t.Fatalf("an unreadable bodyFilePath must ask, not silently pass: got %+v", o)
	}
	if !strings.Contains(o.HookSpecificOutput.PermissionDecisionReason, "couldn't read") || !strings.Contains(o.HookSpecificOutput.PermissionDecisionReason, missing) {
		t.Errorf("reason should say the hook couldn't read the file, naming it: got %q", o.HookSpecificOutput.PermissionDecisionReason)
	}
}

func TestStructureDenyReason_Text(t *testing.T) {
	none := structureDenyReason(0, 0, 0)
	for _, s := range []string{"No structure check has passed in the last 60 minutes.", "structure check", "questions", "kill list"} {
		if !strings.Contains(none, s) {
			t.Errorf("missing %q", s)
		}
	}
	// M4: the phrase was backwards ("of this copy" when it's the share of
	// the checked draft found in this copy), and it rounded instead of
	// rounding down.
	if !strings.Contains(structureDenyReason(0.43, 0, 100), "only 43% of the checked draft is in this copy, and it needs 90%.") {
		t.Error("coverage sentence wrong")
	}
	// 26/29 = 89.65...%, which rounds to "90%" but must floor to "89%".
	if !strings.Contains(structureDenyReason(26.0/29.0, 0, 100), "only 89% of the checked draft is in this copy") {
		t.Error("M4: coverage percentage must round down, not to the nearest whole number")
	}
	// I1: the copy added words the check never saw must be named plainly,
	// even when forward coverage alone is fine.
	if !strings.Contains(structureDenyReason(1, 50, 100), "this copy has 50 words the check never saw") {
		t.Error("I1: unmatched-words sentence missing")
	}
	if strings.Contains(structureDenyReason(1, 5, 100), "words the check never saw") {
		t.Error("I1: unmatched count inside its budget must not be mentioned")
	}
}

func TestPreTool_SendDecisions(t *testing.T) {
	newHome(t)
	post := func(ch, text string) map[string]any {
		return map[string]any{"tool_name": "mcp__slack__conversations_add_message", "tool_input": map[string]any{"channel": ch, "text": text}}
	}
	check := func(p map[string]any, want, reason string) {
		t.Helper()
		o, _ := hook(t, "pre-tool", p)
		if dec(o) != want || (reason != "" && !strings.Contains(o.HookSpecificOutput.PermissionDecisionReason, reason)) {
			t.Errorf("want %s %q, got %+v", want, reason, o)
		}
	}
	check(post(person, longUnchecked), "deny", "No structure check has passed")
	check(post(bot, longUnchecked), "allow", slackExemptReason)
	check(post(person, "Thanks, got it."), "ask", slackAskReason)
	addReceipt(t, bodyMD, 0)
	check(post(person, bodySlack), "ask", slackAskReason)
	check(map[string]any{"tool_name": "mcp__claude_ai_Microsoft_365__outlook_send_mail", "tool_input": map[string]any{"body": bodyHTML()}}, "", "")
	check(post(person, longUnchecked), "deny", "is in this copy") // M4: wording fix, was "matches only"
}

// TestPreTool_C1_ExpandedSlackToolsGetSendChecks reproduces C1's failing
// input: an armed session, nothing read, and one of the Slack server's
// other posting, scheduling, editing or draft tools carrying unchecked
// content to a person. Before the fix none of these printed anything at
// all.
func TestPreTool_C1_ExpandedSlackToolsGetSendChecks(t *testing.T) {
	newHome(t)
	for _, tool := range []string{
		"mcp__slack__conversations_add_message",
		"mcp__slack__conversations_add_message",
		"mcp__slack__conversations_add_message",
		"mcp__claude_ai_Microsoft_365__teams_send_chat_message",
		"mcp__claude_ai_Microsoft_365__teams_send_channel_message",
		"mcp__claude_ai_Microsoft_365__teams_reply_channel_message",
		"mcp__claude_ai_Microsoft_365__teams_send_channel_message",
	} {
		o, _ := hook(t, "pre-tool", map[string]any{"tool_name": tool, "tool_input": map[string]any{"channel": person, "text": longUnchecked}})
		if dec(o) != "deny" {
			t.Errorf("%s: want deny (no receipt yet), got %+v", tool, o)
		}
	}
	// post_draft carries no text of its own, so it only gets the ask.
	o, _ := hook(t, "pre-tool", map[string]any{"tool_name": "mcp__claude_ai_Microsoft_365__teams_send_chat_message", "tool_input": map[string]any{"channel": person}})
	if dec(o) != "ask" {
		t.Errorf("post_draft: want ask, got %+v", o)
	}
	// Once a matching receipt exists, the expanded tools reach the ask too.
	addReceipt(t, bodyMD, 0)
	o, _ = hook(t, "pre-tool", map[string]any{"tool_name": "mcp__claude_ai_Microsoft_365__teams_reply_channel_message",
		"tool_input": map[string]any{"channel": person, "text": bodySlack}})
	if dec(o) != "ask" {
		t.Errorf("edit_message after a passing receipt: want ask, got %+v", o)
	}
}

// TestPreTool_C1_BlocksTextCountsForStructureCheck reproduces C1's
// "outgoingText includes every text field inside blocks": a
// post_block_message call with no top-level text field still needs (and
// can satisfy) the structure check from its blocks' own text.
func TestPreTool_C1_BlocksTextCountsForStructureCheck(t *testing.T) {
	newHome(t)
	blocks := []any{
		map[string]any{"type": "section", "text": map[string]any{"type": "mrkdwn", "text": bodySlack}},
	}
	call := map[string]any{"tool_name": "mcp__slack__conversations_add_message", "tool_input": map[string]any{"channel": person, "blocks": blocks}}
	if o, _ := hook(t, "pre-tool", call); dec(o) != "deny" {
		t.Fatalf("blocks text with no receipt: want deny, got %+v", o)
	}
	addReceipt(t, bodyMD, 0)
	if o, _ := hook(t, "pre-tool", call); dec(o) != "ask" {
		t.Fatalf("blocks text matching a fresh receipt: want ask, got %+v", o)
	}
}

// TestExemptChannel_C1_ChannelsArrayAllOrNothing reproduces C1's
// "a channels array is exempt only if every entry is exempt".
func TestExemptChannel_C1_ChannelsArrayAllOrNothing(t *testing.T) {
	exempt := []string{bot}
	if !exemptChannel(map[string]any{"channels": []any{bot, bot}}, exempt) {
		t.Error("every entry exempt: want exempt")
	}
	if exemptChannel(map[string]any{"channels": []any{bot, person}}, exempt) {
		t.Error("one entry not exempt: want not exempt")
	}
}

// TestCrashAskReason_M5 reproduces M5: the crash reason said "this send"
// even when the call was a polish-pass Skill call, and had no closing
// period.
func TestCrashAskReason_M5(t *testing.T) {
	want := "Byline's checks couldn't run, so approve this step yourself, or reinstall Byline."
	if crashAskReason != want {
		t.Errorf("got %q, want %q", crashAskReason, want)
	}
}

func TestPreTool_PluginPrefixedGraspAndCustomTools(t *testing.T) {
	newHome(t)
	for _, c := range []struct {
		tool string
		in   map[string]any
		want string
	}{
		{"mcp__plugin_SomePlugin_claude_ai_Microsoft_365__outlook_send_mail", map[string]any{"body": longUnchecked}, "deny"},
		{"mcp__plugin_SomePlugin_slack__conversations_add_message", map[string]any{"channel": bot, "text": longUnchecked}, "allow"},
		{"mcp__gmail__draft_email", map[string]any{"action": "create", "body": longUnchecked}, "deny"},
	} {
		if o, _ := hook(t, "pre-tool", map[string]any{"tool_name": c.tool, "tool_input": c.in}); dec(o) != c.want {
			t.Errorf("%s: got %q, want %q", c.tool, dec(o), c.want)
		}
	}
	os.WriteFile(paths.SettingsPath(), []byte(`{"format":1,"send_tools":["mcp__x__send"]}`), 0o600)
	for _, name := range []string{"mcp__x__send", "mcp__plugin_SomePlugin_x__send"} {
		if o, _ := hook(t, "pre-tool-extra", map[string]any{"tool_name": name, "tool_input": map[string]any{"text": longUnchecked}}); dec(o) != "deny" {
			t.Errorf("%s: a send tool the person added is gated by pre-tool --extra", name)
		}
	}
	if o, _ := hook(t, "pre-tool", map[string]any{"tool_name": "mcp__slack__conversations_add_message", "tool_input": map[string]any{"channel": person, "text": longUnchecked}}); dec(o) != "deny" {
		t.Error("adding a tool must not drop a default one from pre-tool")
	}
}

func TestPreTool_WritingDenyBeatsSlackAllow(t *testing.T) {
	newHome(t)
	hook(t, "prompt", map[string]any{"session_id": "s", "prompt": "draft a slack message to the vendor"})
	o, _ := hook(t, "pre-tool", map[string]any{"session_id": "s", "tool_name": "mcp__slack__conversations_add_message",
		"tool_input": map[string]any{"channel": bot, "text": longUnchecked}})
	if dec(o) != "deny" || o.HookSpecificOutput.PermissionDecisionReason != writingDenyReason([]string{"brief", "profile", "exemplars"}) {
		t.Fatalf("got %+v", o)
	}
}

func TestGate_NonSendMcpToolIsSilent(t *testing.T) {
	newHome(t)
	hook(t, "prompt", map[string]any{"session_id": "s", "prompt": "draft a slack message to the vendor"}) // armed, nothing read
	call := func(name string) map[string]any {
		return map[string]any{"session_id": "s", "tool_name": name, "transcript_path": "/no/such/log",
			"tool_input": map[string]any{"channel": person, "text": longUnchecked}}
	}
	for _, name := range []string{"mcp__slack__conversations_search_messages", "mcp__claude_ai_Microsoft_365__outlook_read_mail",
		"mcp__plugin_SomePlugin_slack__conversations_history", "mcp__builder-mcp__InternalSearch", "mcp__slack__conversations_add_message_v2", "mcp__x__send"} {
		if _, raw := hook(t, "pre-tool-extra", call(name)); raw != "" {
			t.Errorf("%s: not a send tool, so want silence, got %s", name, raw)
		}
	}
	os.WriteFile(paths.SettingsPath(), []byte(`{"format":1,"send_tools":["mcp__x__send"]}`), 0o600)
	for _, name := range []string{"mcp__x__send", "mcp__plugin_SomePlugin_x__send"} {
		if o, _ := hook(t, "pre-tool-extra", call(name)); o.HookSpecificOutput.PermissionDecisionReason != writingDenyReason([]string{"brief", "profile", "exemplars"}) {
			t.Errorf("%s: once added in settings it is a send tool, so the armed session stops it: got %+v", name, o)
		}
	}
}

func TestGate_ExtraModeSkipsDefaultSendTools(t *testing.T) {
	newHome(t)
	os.WriteFile(paths.SettingsPath(), []byte(`{"format":1,"send_tools":["mcp__x__send"]}`), 0o600)
	hook(t, "prompt", map[string]any{"session_id": "s", "prompt": "draft a slack message to the vendor"}) // armed, nothing read
	call := func(name string) map[string]any {
		return map[string]any{"session_id": "s", "tool_name": name, "tool_input": map[string]any{"channel": person, "text": longUnchecked}}
	}
	for _, name := range append(slices.Clone(state.DefaultSendTools), "mcp__plugin_SomePlugin_slack__conversations_add_message") {
		if _, raw := hook(t, "pre-tool-extra", call(name)); raw != "" {
			t.Errorf("%s: a default send tool belongs to pre-tool, so --extra stays silent: got %s", name, raw)
		}
		if o, _ := hook(t, "pre-tool", call(name)); dec(o) != "deny" {
			t.Errorf("%s: pre-tool still stops it", name)
		}
	}
	if _, raw := hook(t, "pre-tool-extra", polish("s")); raw != "" {
		t.Errorf("--extra never handles the polish pass: got %s", raw)
	}
	if _, raw := hook(t, "pre-tool-extra", "not json"); raw != "" {
		t.Errorf("--extra is silent on a payload it can't read: got %s", raw)
	}
	orig := modes["pre-tool-extra"]
	modes["pre-tool-extra"] = func([]byte) *hookOutput { panic("boom") }
	var out bytes.Buffer
	Run("pre-tool-extra", strings.NewReader("{}"), &out)
	modes["pre-tool-extra"] = orig
	if out.Len() != 0 {
		t.Errorf("a panic under --extra stays silent: got %s", out.String())
	}
}

// TestPreTool_I3_SendingSavedDraftWithNoBodyAsks reproduces I3: sending a
// saved draft with no body sends whatever the draft holds, so the gate
// asks the person instead of passing text it never saw. A send that
// carries a checked body still goes through.
func TestPreTool_I3_SendingSavedDraftWithNoBodyAsks(t *testing.T) {
	newHome(t)
	send := func(in map[string]any) map[string]any {
		return map[string]any{"tool_name": "mcp__claude_ai_Microsoft_365__outlook_send_draft", "tool_input": in}
	}
	o, _ := hook(t, "pre-tool", send(map[string]any{"draftId": "d1"}))
	if dec(o) != "ask" || !strings.Contains(o.HookSpecificOutput.PermissionDecisionReason, "can't see") {
		t.Fatalf("sending a saved draft with no body must ask, saying Byline can't see the text: got %+v", o)
	}
	o, _ = hook(t, "pre-tool", send(map[string]any{"draftId": "d1", "body": ""}))
	if dec(o) != "ask" {
		t.Fatalf("an empty body is still no text to check, so it must ask: got %+v", o)
	}
	o, _ = hook(t, "pre-tool", send(map[string]any{"draftId": "d1", "body": bodyHTML()}))
	if dec(o) != "deny" {
		t.Fatalf("sending with an unchecked body must still be stopped: got %+v", o)
	}
	addReceipt(t, bodyMD, 0)
	if o, _ = hook(t, "pre-tool", send(map[string]any{"draftId": "d1", "body": bodyHTML()})); dec(o) != "" {
		t.Fatalf("sending with a body that passed its check must go through: got %+v", o)
	}
}

// TestPreTool_M2_AddedToolWithUnseenTextAsks: a send tool the person added
// whose text is in a field the gate doesn't read gets an ask; one whose
// text it can read still gets the structure check, and a default tool with
// no text asks too.
func TestPreTool_M2_AddedToolWithUnseenTextAsks(t *testing.T) {
	newHome(t)
	os.WriteFile(paths.SettingsPath(), []byte(`{"format":1,"send_tools":["mcp__slack__files_upload"]}`), 0o600)
	call := func(input map[string]any) (hookOutput, string) {
		return hook(t, "pre-tool-extra", map[string]any{"tool_name": "mcp__slack__files_upload", "tool_input": input})
	}
	if o, _ := call(map[string]any{"channelId": person, "content": "x", "initialComment": longUnchecked}); dec(o) != "ask" || o.HookSpecificOutput.PermissionDecisionReason != unseenTextAskReason {
		t.Errorf("text in a field the gate can't read: want ask, got %+v", o)
	}
	if o, _ := call(map[string]any{"channelId": person, "content": "x", "text": "   ", "initialComment": longUnchecked}); dec(o) != "ask" {
		t.Errorf("whitespace-only text with the real text elsewhere: want ask, got %+v", o)
	}
	if o, _ := call(map[string]any{"channelId": person, "text": longUnchecked}); dec(o) != "deny" {
		t.Errorf("text the gate can read: want the structure check's deny, got %+v", o)
	}
	if o, _ := hook(t, "pre-tool", map[string]any{"tool_name": "mcp__claude_ai_Microsoft_365__outlook_create_draft", "tool_input": map[string]any{"subject": "x"}}); dec(o) != "ask" {
		t.Errorf("a default send tool with no text it can read: want ask, got %+v", o)
	}
}
