package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"byline/internal/state"
)

// RunToolMode runs one Byline tool directly over stdin/stdout, for
// `byline-mcp tool <name>`: the same handler and argument checks a
// tools/call would run, printing exactly the text a tools/call result
// carries. It builds its own server and runs EnsureVoiceFolder itself.
func RunToolMode(name string, stdin io.Reader, stdout io.Writer) int {
	ss := &startState{err: state.EnsureVoiceFolder()}
	s := &Server{Tools: buildTools(ss)}

	data, err := io.ReadAll(io.LimitReader(stdin, MaxMessageBytes))
	if err != nil {
		return printToolError(stdout, "Couldn't read the arguments.")
	}
	args, ok := decodeToolArgs(data)
	if !ok {
		return printToolError(stdout, "The arguments must be a JSON object.")
	}

	text, isError, known := s.CallTool(name, args)
	if !known {
		return printToolError(stdout, fmt.Sprintf("There's no Byline tool called %q.", name))
	}
	if isError {
		return printToolError(stdout, text)
	}
	fmt.Fprintln(stdout, text)
	return 0
}

// decodeToolArgs decodes data as tool mode's stdin: empty, or all
// whitespace, means no arguments at all (ok, nil map); anything else must
// decode to a JSON object.
func decodeToolArgs(data []byte) (args map[string]any, ok bool) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, true
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	m, ok := v.(map[string]any)
	return m, ok
}

// printToolError writes {"error":"<msg>"} plus a newline and returns 1.
func printToolError(stdout io.Writer, msg string) int {
	data, err := json.Marshal(map[string]string{"error": msg})
	if err != nil {
		data = []byte(`{"error":"Byline hit an internal error."}`)
	}
	fmt.Fprintln(stdout, string(data))
	return 1
}
