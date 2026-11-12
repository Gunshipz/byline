package gate

import (
	"os"
	"strings"
	"testing"
	"time"

	"byline/internal/paths"
)

// The live failure (12 Nov 2026): a short reply checked without its
// signature, then saved with the person's own signature, was denied on
// every try. The signature's words counted as words the check never saw,
// over the 8 a short message is allowed.
const sigReply = "Thanks for sending the response to our request. The letter covers only the public works files, so I'd like to ask the planning and zoning offices about the site plan approval before we pull anything."

// msoSignature is the layout an Outlook signature block pastes as: one
// MsoNormal paragraph per line, the text split across styled spans.
const msoSignature = `<p class="MsoNormal"><span style="font-size:8.0pt;font-family:Calibri,sans-serif;">&nbsp;</span></p>
<p class="MsoNormal"><b><span style="font-size:11.0pt;font-family:Calibri,sans-serif;">The Sender</span></b><span style="font-size:11.0pt;font-family:Calibri,sans-serif;"> | Senior Manager, Regional Facilities Planning</span></p>
<p class="MsoNormal"><span style="font-size:11.0pt;font-family:Calibri,sans-serif;">Example Corp | Operations Strategy and Delivery</span></p>`

const plainSignature = `<p><b>The Sender</b> | Senior Manager, Regional Facilities Planning<br>Example Corp | Operations Strategy and Delivery</p>`

const textSignature = "The Sender | Senior Manager, Regional Facilities Planning\nExample Corp | Operations Strategy and Delivery"

// withSignatureLines saves lines as the person's signature_lines, the
// way setup does.
func withSignatureLines(t *testing.T, lines ...string) {
	t.Helper()
	body := `{"format":1,"signature_lines":["` + strings.Join(lines, `","`) + `"]}`
	if err := os.WriteFile(paths.MyRulesPath(), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// htmlReply is the reply as an email body, with the style block the
// email-draft skill puts at the top of every body.
func htmlReply(sig string) string {
	return "<html><head><style><!--\nbody { font-family: Calibri, sans-serif; font-size: 11pt; color: #000000; }\n--></style></head><body><p>" + sigReply + "</p>\n" + sig + "</body></html>"
}

func sendDecision(t *testing.T, tool string, input map[string]any) hookOutput {
	t.Helper()
	o, _ := hook(t, "pre-tool", map[string]any{"tool_name": tool, "tool_input": input})
	return o
}

func TestPreTool_SignatureIsNotNewWords(t *testing.T) {
	for name, sig := range map[string]string{"Outlook MsoNormal": msoSignature, "plain paragraph": plainSignature} {
		t.Run(name, func(t *testing.T) {
			newHome(t)
			withSignatureLines(t, "The Sender")
			addReceipt(t, sigReply, 0)
			for _, tool := range []string{"mcp__gmail__draft_email", "mcp__claude_ai_Microsoft_365__outlook_send_mail"} {
				if o := sendDecision(t, tool, map[string]any{"action": "createReply", "body": htmlReply(sig)}); dec(o) == "deny" {
					t.Fatalf("%s: the person's own signature must not count as unchecked words: %s", tool, o.HookSpecificOutput.PermissionDecisionReason)
				}
			}
		})
	}
}

// A draft checked with its signature in plain text, then sent with the
// signature in HTML (or with none), still matches its receipt.
func TestPreTool_SignatureInCheckedCopyOnly(t *testing.T) {
	newHome(t)
	withSignatureLines(t, "The Sender")
	addReceipt(t, sigReply+"\n\n"+textSignature, 0)
	for name, body := range map[string]string{"HTML signature": htmlReply(msoSignature), "no signature": "<p>" + sigReply + "</p>"} {
		if o := sendDecision(t, "mcp__claude_ai_Microsoft_365__outlook_send_mail", map[string]any{"body": body}); dec(o) == "deny" {
			t.Errorf("%s: %s", name, o.HookSpecificOutput.PermissionDecisionReason)
		}
	}
}

// The saved signature line matches the sent one even when the HTML
// leaves extra spaces between its words.
func TestPreTool_FullSavedSignatureLineMatches(t *testing.T) {
	newHome(t)
	withSignatureLines(t, "The Sender | Senior Manager, Regional Facilities Planning", "Example Corp | Operations Strategy and Delivery")
	addReceipt(t, sigReply, 0)
	if o := sendDecision(t, "mcp__claude_ai_Microsoft_365__outlook_send_mail", map[string]any{"body": htmlReply(msoSignature)}); dec(o) == "deny" {
		t.Fatal(o.HookSpecificOutput.PermissionDecisionReason)
	}
}

// Text that only starts like a signature, then runs on as a message,
// still counts: new content can't hide behind the person's name.
func TestPreTool_ContentAfterSignatureCueStillCounts(t *testing.T) {
	newHome(t)
	withSignatureLines(t, "The Sender")
	addReceipt(t, sigReply, 0)
	tail := "<p>The Sender</p><p>Also the vendor invoices for racking were never reconciled.</p><p>Harbor lantern velvet orchard thistle granite.</p><p>Meadow falcon anchor copper willow ember.</p><p>Glacier compass driftwood sparrow quartz lagoon.</p><p>Cedar prairie ridgeline sable tundra violet.</p>"
	if o := sendDecision(t, "mcp__claude_ai_Microsoft_365__outlook_send_mail", map[string]any{"body": "<p>" + sigReply + "</p>" + tail}); dec(o) != "deny" {
		t.Fatalf("six lines of new content after a name line must still be denied: got %+v", o)
	}
}

// A check run on the exact HTML body (tags and all) is a check on that
// copy, so sending that body passes.
func TestPreTool_ReceiptFromExactHTMLBody(t *testing.T) {
	newHome(t)
	withSignatureLines(t, "The Sender")
	body := htmlReply(msoSignature)
	addReceipt(t, body, 0)
	if o := sendDecision(t, "mcp__claude_ai_Microsoft_365__outlook_send_mail", map[string]any{"body": body}); dec(o) == "deny" {
		t.Fatal(o.HookSpecificOutput.PermissionDecisionReason)
	}
}

// Two receipts cover a message equally well: the gate must judge it by
// the one that saw the most of it, not whichever is older. Before the
// fix, the older receipt for a shorter version hid the newer, matching one.
func TestBestCoverage_TieUsesFewerUnmatched(t *testing.T) {
	newHome(t)
	extra := " Harbor lantern velvet orchard thistle granite meadow falcon anchor copper willow."
	addReceipt(t, sigReply, 2*time.Minute)
	addReceipt(t, sigReply+extra, time.Minute)
	if o := sendDecision(t, "mcp__claude_ai_Microsoft_365__outlook_send_mail", map[string]any{"body": sigReply + extra}); dec(o) == "deny" {
		t.Fatalf("the newer receipt checked this exact copy: %s", o.HookSpecificOutput.PermissionDecisionReason)
	}
}

// A body line that only starts like a job title ("Manager, ...") is
// message, not signature: new lines after it still count (AutoSDE, CR
// review of 1.0.1).
func TestPreTool_TitleLineInBodyIsNotASignature(t *testing.T) {
	newHome(t)
	withSignatureLines(t, "The Sender")
	addReceipt(t, sigReply, 0)
	tail := "<p>Manager, please review the attached before Friday.</p><p>Harbor lantern velvet orchard thistle granite meadow.</p><p>Falcon anchor copper willow ember glacier compass.</p>"
	if o := sendDecision(t, "mcp__claude_ai_Microsoft_365__outlook_send_mail", map[string]any{"body": "<p>" + sigReply + "</p>" + tail}); dec(o) != "deny" {
		t.Fatalf("new lines after a body line starting with a title must still be denied: got %+v", o)
	}
}

// Even after the person's own name line, a long block is more message:
// a signature is 30 words or fewer.
func TestPreTool_LongBlockAfterNameStillCounts(t *testing.T) {
	newHome(t)
	withSignatureLines(t, "The Sender")
	addReceipt(t, sigReply, 0)
	tail := "<p>The Sender | harbor lantern velvet orchard thistle granite meadow falcon anchor</p><p>Copper willow ember glacier compass driftwood sparrow quartz lagoon cedar</p><p>Prairie ridgeline sable tundra violet marble canyon ferry orbit zephyr</p>"
	if o := sendDecision(t, "mcp__claude_ai_Microsoft_365__outlook_send_mail", map[string]any{"body": "<p>" + sigReply + "</p>" + tail}); dec(o) != "deny" {
		t.Fatalf("a 30-plus word block after the name line must still be denied: got %+v", o)
	}
}
