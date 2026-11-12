package mcp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestToolMode_RunsSameHandlerAsMCP(t *testing.T) {
	setupHome(t)
	r := firstCoreStringRule(t)
	args := map[string]any{"text": "We will " + r.Strings[0] + " today."}
	_, _, viaMCP := callTool(t, NewServer(), "byline_lint", args)
	stdin, _ := json.Marshal(args)
	var out bytes.Buffer
	want(t, RunToolMode("byline_lint", bytes.NewReader(stdin), &out), 0)
	want(t, out.String(), viaMCP+"\n") // byte for byte what the MCP result carries

	for _, c := range []struct{ name, in, msg string }{
		{"byline_lint", `{"text":"a","draft":"x"}`, `byline_lint does not take "draft". It takes: text.`},
		{"nope", `{}`, `There's no Byline tool called "nope".`},
		{"byline_lint", `[1]`, "The arguments must be a JSON object."},
	} {
		out.Reset()
		want(t, RunToolMode(c.name, strings.NewReader(c.in), &out), 1)
		want(t, decodeUseNumber(t, out.String())["error"], c.msg)
	}

	out.Reset()
	want(t, RunToolMode("byline_status", strings.NewReader(""), &out), 0) // empty stdin means no arguments
}
