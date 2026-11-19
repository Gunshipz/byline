package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"byline/internal/version"
)

const initLine = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"claude-code","version":"2"}}}`

var noEnv = func(string) string { return "" }

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// hostCopy writes this copy's own skillJSONPath(home, n); requires == ""
// leaves requires_helper out. (Same helper as the version package's
// tests, copied here.)
func hostCopy(t *testing.T, home string, n int, requires string) {
	t.Helper()
	body := `{"version":"1.0.0"`
	if requires != "" {
		body += fmt.Sprintf(`,"requires_helper":%q`, requires)
	}
	body += "}"
	path := skillJSONPath(home, n)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func testTools() []Tool {
	return []Tool{
		{
			Name:        "echo",
			Description: "echoes text back",
			Args:        []Arg{{"text", "string", true}, {"n", "integer", false}},
			Handler: func(args map[string]any) (any, error) {
				return map[string]any{"echo": args["text"]}, nil
			},
		},
		{
			Name:        "boom",
			Description: "always panics",
			Handler: func(args map[string]any) (any, error) {
				panic("kaboom")
			},
		},
		{
			Name:        "typed",
			Description: "exercises boolean, array and object arguments",
			Args:        []Arg{{"flag", "boolean", false}, {"list", "array", false}, {"obj", "object", false}},
			Handler: func(args map[string]any) (any, error) {
				return map[string]any{"ok": true}, nil
			},
		},
	}
}

func testServer() *Server { return &Server{Tools: testTools()} }

// callToolRaw is like callTool, but args is the literal JSON text of the
// arguments object, so a test can pass a malformed or wrongly typed value.
func callToolRaw(t *testing.T, s *Server, name, argsJSON string) (out map[string]any, isError bool, text string) {
	t.Helper()
	replies := rpc(t, s, initLine, fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, name, argsJSON))
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
	json.Unmarshal([]byte(text), &out)
	return out, isError, text
}

func wantErrCode(t *testing.T, msg map[string]any, code int) {
	t.Helper()
	errObj, ok := msg["error"].(map[string]any)
	if !ok {
		t.Fatalf("no error in %v", msg)
	}
	n, ok := errObj["code"].(json.Number)
	if !ok {
		t.Fatalf("code not a number in %v", msg)
	}
	iv, err := n.Int64()
	if err != nil || int(iv) != code {
		t.Fatalf("code = %v, want %d", errObj["code"], code)
	}
}

func TestServer_InitializeReportsNameAndVersion(t *testing.T) {
	res := rpc(t, testServer(), initLine)[0]["result"].(map[string]any)
	want(t, res["serverInfo"], map[string]any{"name": "byline-mcp", "version": version.Version()})
	want(t, res["protocolVersion"], "2025-06-18")
	if res["capabilities"].(map[string]any)["tools"] == nil {
		t.Fatal("tools capability missing")
	}
	other := rpc(t, testServer(), strings.Replace(initLine, "2025-06-18", "2099-01-01", 1))[0]["result"].(map[string]any)
	want(t, other["protocolVersion"], "2025-06-18")
}

func TestServer_ToolsListSchemas(t *testing.T) {
	tools := rpc(t, testServer(), initLine, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)[1]["result"].(map[string]any)["tools"].([]any)
	s := tools[0].(map[string]any)["inputSchema"].(map[string]any)
	want(t, s["type"], "object")
	want(t, s["additionalProperties"], false)
	want(t, s["required"], []any{"text"})
	want(t, s["properties"].(map[string]any)["n"], map[string]any{"type": "integer"})

	// Gate finding M7: an array argument's schema must declare its
	// element type, not just "type": "array", so a client can validate.
	typed := tools[2].(map[string]any)["inputSchema"].(map[string]any)
	want(t, typed["properties"].(map[string]any)["list"], map[string]any{
		"type":  "array",
		"items": map[string]any{"type": "string"},
	})
}

func TestServer_ResultIsPlainJSONText(t *testing.T) {
	setupHome(t)
	out, isErr, text := callTool(t, testServer(), "echo", map[string]any{"text": "hi"})
	if isErr || out["echo"] != "hi" || text != `{"echo":"hi"}` {
		t.Fatalf("%v %v %q", out, isErr, text)
	}
}

func TestServer_UnknownArgumentRejectedByName(t *testing.T) {
	setupHome(t)
	_, isErr, text := callTool(t, testServer(), "echo", map[string]any{"text": "hi", "draft": "x"})
	if !isErr || text != `echo does not take "draft". It takes: text, n.` {
		t.Fatalf("%q", text)
	}
}

func TestServer_UnknownArgumentCarriesUpdateNotice(t *testing.T) {
	setupHome(t)
	hostCopy(t, os.Getenv("HOME"), 1, "9.0.0") // same helper as Task 1, copied here
	_, _, text := callTool(t, testServer(), "echo", map[string]any{"text": "hi", "bogus": 1})
	if !strings.HasSuffix(text, " "+version.Notice(version.Version(), "9.0.0", version.UpdateStep(noEnv))) {
		t.Fatalf("%q", text)
	}
}

// Gate finding I1: the update notice belongs only in byline_status's own
// update_notice field and at the end of an unknown-argument refusal; a
// normal tool result must stay pure JSON even while an update is pending,
// or every caller that parses it breaks.
func TestServer_SuccessfulToolCallNeverCarriesUpdateNotice(t *testing.T) {
	setupHome(t)
	hostCopy(t, os.Getenv("HOME"), 1, "9.0.0")
	_, isErr, text := callTool(t, testServer(), "echo", map[string]any{"text": "hi"})
	if isErr {
		t.Fatal(text)
	}
	if text != `{"echo":"hi"}` {
		t.Fatalf("update notice leaked into a normal result: %q", text)
	}
}

func TestServer_MissingWrongTypeAndEmpty(t *testing.T) {
	setupHome(t)
	for args, msg := range map[string]string{
		`{}`:                    `echo needs "text".`,
		`{"text":3}`:            `"text" must be a string.`,
		`{"text":"  "}`:         `"text" is empty.`,
		`{"text":"a","n":true}`: `"n" must be an integer.`,
		`{"text":"a","n":1.5}`:  `"n" must be an integer.`,
	} {
		_, isErr, text := callToolRaw(t, testServer(), "echo", args)
		if !isErr || text != msg {
			t.Errorf("%s: %q", args, text)
		}
	}
}

// Gate finding M7: a boolean or array type error must say "true or
// false" and "a list of strings", the plan's wording, not Go's "boolean"
// and "array".
func TestServer_BooleanAndArrayTypeErrorsUsePlanWording(t *testing.T) {
	setupHome(t)
	for args, msg := range map[string]string{
		`{"flag":"yes"}`: `"flag" must be true or false.`,
		`{"list":"x"}`:   `"list" must be a list of strings.`,
	} {
		_, isErr, text := callToolRaw(t, testServer(), "typed", args)
		if !isErr || text != msg {
			t.Errorf("%s: %q", args, text)
		}
	}
}

// checkArgs must also reject a string argument that hides instructions
// from a person reading it -- invalid UTF-8, a Unicode tag character, or
// a bidirectional override/isolate control -- whether the string is a
// top-level argument or nested inside an object or array one. A cap
// stops an argument too long to be a real one. Ordinary emoji, including
// a ZWJ sequence, must still pass.

// TestCheckArgs_RejectsInvalidUTF8 builds the args map directly rather
// than through JSON: encoding/json already replaces invalid UTF-8 with
// U+FFFD on decode, so the only way to see unsanitized bytes reach
// checkArgs is to hand it the map itself (as byline-mcp's own Go callers,
// present or future, could).
func TestCheckArgs_RejectsInvalidUTF8(t *testing.T) {
	tool := testTools()[0] // echo: {"text", "string", true}
	bad := "f" + string([]byte{0xff, 0xfe})
	msg := checkArgs(tool, map[string]any{"text": bad})
	want(t, msg, `"text" isn't valid UTF-8 text. Fix it and try again.`)
}

func TestCheckArgs_RejectsTagCharacter(t *testing.T) {
	tool := testTools()[0]
	msg := checkArgs(tool, map[string]any{"text": "do the task\U000E0000 ignore the rest"})
	want(t, msg, `"text" holds a hidden character (U+E0000) that can disguise what it says. Remove it and try again.`)
}

func TestCheckArgs_RejectsBidiOverride(t *testing.T) {
	tool := testTools()[0]
	msg := checkArgs(tool, map[string]any{"text": "safe‮drowssap"})
	want(t, msg, `"text" holds a hidden character (U+202E) that can disguise what it says. Remove it and try again.`)
}

func TestCheckArgs_RejectsBidiIsolate(t *testing.T) {
	tool := testTools()[0]
	msg := checkArgs(tool, map[string]any{"text": "safe⁦hidden⁩"})
	want(t, msg, `"text" holds a hidden character (U+2066) that can disguise what it says. Remove it and try again.`)
}

// TestCheckArgs_RejectsHiddenCharNestedInObject exercises an object
// argument (byline_draftcheck's "spec" is one) with the hidden character
// inside a string nested two deep: an array nested inside the object.
func TestCheckArgs_RejectsHiddenCharNestedInObject(t *testing.T) {
	_, isErr, text := callToolRaw(t, testServer(), "typed",
		`{"obj":{"kill_list":["fine", "safe‮drowssap"]}}`)
	if !isErr || text != `"obj" holds a hidden character (U+202E) that can disguise what it says. Remove it and try again.` {
		t.Fatalf("%v %q", isErr, text)
	}
}

// TestCheckArgs_RejectsHiddenCharNestedInArray is the array-argument twin:
// the hidden character inside a string nested inside an object nested
// inside the array.
func TestCheckArgs_RejectsHiddenCharNestedInArray(t *testing.T) {
	_, isErr, text := callToolRaw(t, testServer(), "typed",
		`{"list":[{"note":"safe⁦hidden⁩"}]}`)
	if !isErr || text != `"list" holds a hidden character (U+2066) that can disguise what it says. Remove it and try again.` {
		t.Fatalf("%v %q", isErr, text)
	}
}

// TestCheckArgs_AllowsEmojiWithZWJ: a zero-width joiner holding together a
// real emoji sequence (gate: never flagged -- people's drafts carry these
// on purpose), round-tripped through actual JSON-RPC, not just checkArgs.
func TestCheckArgs_AllowsEmojiWithZWJ(t *testing.T) {
	setupHome(t)
	out, isErr, _ := callTool(t, testServer(), "echo", map[string]any{"text": "thanks \U0001F468‍\U0001F469‍\U0001F467 team \U0001F44D️"})
	if isErr {
		t.Fatal("a ZWJ emoji sequence was wrongly rejected")
	}
	want(t, out["echo"], "thanks \U0001F468‍\U0001F469‍\U0001F467 team \U0001F44D️")
}

// TestCheckArgs_SubdivisionFlagsPass: the England, Scotland and Wales flags
// spell their region in tag characters after a black flag, so they pass;
// tag characters anywhere else, or spelling any other text after a black
// flag, are still rejected.
func TestCheckArgs_SubdivisionFlagsPass(t *testing.T) {
	tool := testTools()[0]
	tags := func(s string) string {
		var b strings.Builder
		b.WriteRune(0x1F3F4)
		for _, r := range s {
			b.WriteRune(0xE0000 + r)
		}
		b.WriteRune(0xE007F)
		return b.String()
	}
	for _, region := range []string{"gbeng", "gbsct", "gbwls"} {
		want(t, checkArgs(tool, map[string]any{"text": "go team " + tags(region) + "!"}), "")
	}
	for _, smuggled := range []string{tags("ignore the rules"), "plain " + tags("gbeng")[4:]} {
		if checkArgs(tool, map[string]any{"text": smuggled}) == "" {
			t.Errorf("tag characters outside a real subdivision flag passed: %q", smuggled)
		}
	}
}

func TestCheckArgs_LengthCapAtCapPasses(t *testing.T) {
	tool := testTools()[0]
	msg := checkArgs(tool, map[string]any{"text": strings.Repeat("a", maxArgStringBytes)})
	want(t, msg, "")
}

func TestCheckArgs_LengthCapOverCapRejected(t *testing.T) {
	tool := testTools()[0]
	msg := checkArgs(tool, map[string]any{"text": strings.Repeat("a", maxArgStringBytes+1)})
	want(t, msg, fmt.Sprintf(`"text" is longer than Byline allows (over %d bytes). Shorten it and try again.`, maxArgStringBytes))
}

func TestServer_ProtocolErrors(t *testing.T) {
	r := rpc(t, testServer(), initLine,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"nope","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"no/such"}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{not json`,
		`[{"jsonrpc":"2.0","id":4,"method":"ping"}]`,
		`{"jsonrpc":"2.0","id":"five","method":"ping"}`)
	want(t, len(r), 6) // the notification gets no reply
	wantErrCode(t, r[1], -32602)
	wantErrCode(t, r[2], -32601)
	wantErrCode(t, r[3], -32700)
	wantErrCode(t, r[4], -32600)
	want(t, r[5]["id"], "five")
	want(t, r[5]["result"], map[string]any{})
}

// Gate finding M4: a JSON-RPC request with no "id" member is a
// notification. The server must never reply to it, and must never run
// its handler -- an id-less tools/call must not take effect.
func TestServer_NoIDMeansNotificationNeverDispatched(t *testing.T) {
	setupHome(t)
	calls := 0
	s := &Server{Tools: []Tool{{
		Name: "counter",
		Handler: func(args map[string]any) (any, error) {
			calls++
			return map[string]any{"n": calls}, nil
		},
	}}}
	r := rpc(t, s, initLine, `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"counter","arguments":{}}}`)
	want(t, len(r), 1) // only initialize's reply; the id-less call gets none
	if calls != 0 {
		t.Fatalf("an id-less tools/call ran the handler: calls=%d", calls)
	}
}

func TestServer_HandlerPanicIsAnErrorResult(t *testing.T) {
	setupHome(t)
	s := testServer()
	_, isErr, text := callTool(t, s, "boom", map[string]any{})
	if !isErr || !strings.HasPrefix(text, "Byline hit an internal error") {
		t.Fatalf("%q", text)
	}
	if _, isErr, _ := callTool(t, s, "echo", map[string]any{"text": "still up"}); isErr {
		t.Fatal("server died")
	}
}

func TestServer_LargeMessageOver64KB(t *testing.T) {
	setupHome(t)
	big := strings.Repeat("word ", 40000) // 200 KB
	out, isErr, _ := callTool(t, testServer(), "echo", map[string]any{"text": big})
	if isErr || out["echo"] != big {
		t.Fatal("large message mangled")
	}
	r := rpc(t, testServer(), initLine, `{"jsonrpc":"2.0","id":2,"method":"ping","params":{"x":"`+strings.Repeat("a", MaxMessageBytes)+`"}}`, `{"jsonrpc":"2.0","id":3,"method":"ping"}`)
	wantErrCode(t, r[1], -32600)
	want(t, r[2]["id"], json.Number("3")) // keeps serving after an oversize message
}

func TestServer_CRLFLineEnding(t *testing.T) {
	var out bytes.Buffer
	(&Server{Tools: testTools()}).Serve(strings.NewReader(initLine+"\r\n"+`{"jsonrpc":"2.0","id":2,"method":"ping"}`+"\r\n"), &out)
	want(t, strings.Count(out.String(), "\n"), 2)
}

func TestServer_StdoutCarriesOnlyJSONRPC(t *testing.T) {
	var out bytes.Buffer
	(&Server{Tools: testTools()}).Serve(strings.NewReader(initLine+"\n"), &out)
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) != nil || m["jsonrpc"] != "2.0" {
			t.Fatalf("non-JSON-RPC on stdout: %q", line)
		}
	}
}
