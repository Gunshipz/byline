// Package mcp is Byline's stdio JSON-RPC transport (the Model Context
// Protocol): reading and writing framed messages, dispatching initialize,
// ping, tools/list and tools/call, and checking a tool's arguments before
// its handler ever runs.
package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"byline/internal/evlog"
	"byline/internal/paths"
	"byline/internal/state"
	"byline/internal/version"
)

// MaxMessageBytes is the largest single JSON-RPC message Serve accepts;
// past this, the message is discarded and answered with a protocol error.
const MaxMessageBytes = 16 << 20

// ProtocolVersions are the MCP protocol versions this server understands,
// newest first. ProtocolVersions[0] is offered when a client asks for one
// we don't know.
var ProtocolVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

// Arg describes one argument a Tool's handler takes. Type is one of
// "string", "integer", "boolean", "object" or "array".
type Arg struct {
	Name     string
	Type     string
	Required bool
}

// Tool is one capability exposed over MCP. A returned error becomes an
// isError result whose text is err.Error(); a returned value is
// marshalled as the result JSON.
type Tool struct {
	Name        string
	Description string
	Args        []Arg
	Handler     func(args map[string]any) (any, error)
}

// Server is a byline-mcp stdio JSON-RPC server. OnInitialize, if set, is
// called once with the client's declared name: when it sends initialize,
// or, if it never does, on this server's first tools/call.
type Server struct {
	Tools        []Tool
	OnInitialize func(clientName string)
	initialized  bool
	ss           *startState
}

// startState runs once per server: establishes the voice folder, keeping
// EnsureVoiceFolder's error for byline_status to report.
type startState struct {
	once sync.Once
	err  error
}

func (ss *startState) start(client string) {
	ss.once.Do(func() { ss.err = state.EnsureVoiceFolder() })
}

// NewServer builds byline-mcp's real server: every tool, and a start that
// runs exactly once per server, from initialize or (if it never comes)
// this server's first tools/call.
func NewServer() *Server {
	ss := &startState{}
	s := &Server{Tools: buildTools(ss), ss: ss}
	s.OnInitialize = func(clientName string) { ss.start(clientName) }
	return s
}

// Serve reads JSON-RPC messages from r, one per line, and writes replies
// to w, flushing after each one. It returns nil at end of input.
func (s *Server) Serve(r io.Reader, w io.Writer) error {
	br := bufio.NewReader(r)
	bw := bufio.NewWriter(w)
	for {
		line, oversize, err := readMessage(br)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		line = bytes.TrimRight(line, "\r\n")
		if oversize {
			writeReply(bw, errorReply(nil, -32600, "Invalid Request: message too large"))
			continue
		}
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		if reply := s.handleMessage(line); reply != nil {
			writeReply(bw, reply)
		}
	}
}

// readMessage reads one line (through the trailing '\n', or to EOF),
// appending across bufio.ErrBufferFull so a line can exceed the reader's
// internal buffer. Once the accumulated line exceeds MaxMessageBytes it
// stops appending (oversize) and keeps reading, discarding, through the
// next newline.
func readMessage(br *bufio.Reader) (line []byte, oversize bool, err error) {
	for {
		chunk, rerr := br.ReadSlice('\n')
		if !oversize {
			line = append(line, chunk...)
			if len(line) > MaxMessageBytes {
				oversize = true
				line = nil
			}
		}
		switch rerr {
		case nil:
			return line, oversize, nil
		case bufio.ErrBufferFull:
			continue
		case io.EOF:
			if len(line) == 0 && !oversize {
				return nil, false, io.EOF
			}
			return line, oversize, nil
		default:
			return nil, false, rerr
		}
	}
}

func writeReply(bw *bufio.Writer, reply map[string]any) {
	data, err := json.Marshal(reply)
	if err != nil {
		data, _ = json.Marshal(map[string]any{"jsonrpc": "2.0", "id": nil, "error": map[string]any{"code": -32603, "message": "internal error"}})
	}
	bw.Write(data)
	bw.WriteByte('\n')
	bw.Flush()
}

func errorReply(id any, code int, message string) map[string]any {
	return map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": message}}
}

func successReply(id any, result any) map[string]any {
	return map[string]any{"jsonrpc": "2.0", "id": id, "result": result}
}

// handleMessage decodes and dispatches one JSON-RPC message, or nil for a
// notification, which gets no reply.
func (s *Server) handleMessage(raw []byte) map[string]any {
	var generic any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&generic); err != nil {
		return errorReply(nil, -32700, "Parse error")
	}
	obj, ok := generic.(map[string]any)
	if !ok {
		return errorReply(nil, -32600, "Invalid Request")
	}
	id, hasID := obj["id"]
	if v, _ := obj["jsonrpc"].(string); v != "2.0" {
		return errorReply(id, -32600, "Invalid Request")
	}
	method, _ := obj["method"].(string)
	if method == "" {
		return errorReply(id, -32600, "Invalid Request")
	}
	// Gate finding M4: JSON-RPC 2.0 defines a notification as a request
	// with no "id" member at all (not even id:null) -- it must never get
	// a reply, and its method (tools/call included) must never run.
	if strings.HasPrefix(method, "notifications/") || !hasID {
		return nil
	}
	switch method {
	case "initialize":
		return s.handleInitialize(id, obj["params"])
	case "ping":
		return successReply(id, map[string]any{})
	case "tools/list":
		return s.handleToolsList(id)
	case "tools/call":
		s.runOnInitialize("")
		return s.handleToolsCall(id, obj["params"])
	default:
		return errorReply(id, -32601, "Method not found")
	}
}

// runOnInitialize calls OnInitialize exactly once per server: on
// initialize, with the client's declared name, or, if a tools/call
// arrives first, with "".
func (s *Server) runOnInitialize(clientName string) {
	if s.initialized {
		return
	}
	s.initialized = true
	if s.OnInitialize != nil {
		s.OnInitialize(clientName)
	}
}

func (s *Server) handleInitialize(id, params any) map[string]any {
	p, _ := params.(map[string]any)
	clientName := ""
	if ci, ok := p["clientInfo"].(map[string]any); ok {
		clientName, _ = ci["name"].(string)
	}
	s.runOnInitialize(clientName)
	requested, _ := p["protocolVersion"].(string)
	pv := ProtocolVersions[0]
	for _, v := range ProtocolVersions {
		if v == requested {
			pv = requested
			break
		}
	}
	return successReply(id, map[string]any{
		"protocolVersion": pv,
		"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
		"serverInfo":      map[string]any{"name": "byline-mcp", "version": version.Version()},
		"instructions":    "Byline's writing checks and the person's voice folder. Call byline_status first; if it returns update_notice, pass that sentence to the person once, word for word.",
	})
}

func (s *Server) handleToolsList(id any) map[string]any {
	tools := make([]any, len(s.Tools))
	for i, tl := range s.Tools {
		tools[i] = map[string]any{
			"name":        tl.Name,
			"description": tl.Description,
			"inputSchema": inputSchemaFor(tl),
		}
	}
	return successReply(id, map[string]any{"tools": tools})
}

func inputSchemaFor(tl Tool) map[string]any {
	props := map[string]any{}
	required := []string{}
	for _, a := range tl.Args {
		prop := map[string]any{"type": a.Type}
		if a.Type == "array" {
			// Gate finding M7: every array argument in this server holds
			// strings, so its schema says so.
			prop["items"] = map[string]any{"type": "string"}
		}
		props[a.Name] = prop
		if a.Required {
			required = append(required, a.Name)
		}
	}
	return map[string]any{
		"type":                 "object",
		"properties":           props,
		"required":             required,
		"additionalProperties": false,
	}
}

func (s *Server) handleToolsCall(id, params any) map[string]any {
	p, _ := params.(map[string]any)
	name, _ := p["name"].(string)
	if name == "" {
		return errorReply(id, -32602, "Invalid params: missing tool name")
	}
	args, _ := p["arguments"].(map[string]any)
	text, isError, known := s.CallTool(name, args)
	if !known {
		return errorReply(id, -32602, fmt.Sprintf("Invalid params: unknown tool %q", name))
	}
	return successReply(id, map[string]any{
		"content": []any{map[string]any{"type": "text", "text": text}},
		"isError": isError,
	})
}

// CallTool checks args against name's declared arguments and runs its
// handler, returning exactly the text a tools/call result carries. Both
// tools/call and the CLI tool mode use it. known is false when name isn't
// one of s.Tools.
//
// Gate finding I1: this never appends the update notice itself. A normal
// result must stay pure JSON even while an update is pending; the notice
// belongs only in byline_status's own update_notice field and at the end
// of checkArgs's unknown-argument refusal.
func (s *Server) CallTool(name string, args map[string]any) (text string, isError bool, known bool) {
	var tool *Tool
	for i := range s.Tools {
		if s.Tools[i].Name == name {
			tool = &s.Tools[i]
			break
		}
	}
	if tool == nil {
		return "", false, false
	}
	start := time.Now()
	text, isError, outcome := runTool(*tool, args)
	logToolCall(name, args, outcome, time.Since(start))
	return text, isError, true
}

// updateNotice is the one-line update sentence for the current user, or
// "" when there's nothing pending or the home folder can't be found.
func updateNotice() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return version.UpdateNotice(os.Getenv, home)
}

// runTool runs tool's argument checks and handler, and also reports
// outcome -- "rejected" (checkArgs refused it), "error" or "ok" -- for
// CallTool's tool-calls.jsonl entry (Part 2).
func runTool(tool Tool, args map[string]any) (text string, isError bool, outcome string) {
	if args == nil {
		args = map[string]any{}
	}
	if msg := checkArgs(tool, args); msg != "" {
		return msg, true, "rejected"
	}
	text, isError = invoke(tool, args)
	if isError {
		return text, isError, "error"
	}
	return text, isError, "ok"
}

// logToolCall appends one line to tool-calls.jsonl (Part 2's local
// observability log, since Go has no Security Events Library
// integration yet) for every tools/call this server or the CLI tool
// mode runs: UTC time, the tool's name, outcome, how long it took, and
// the sorted argument NAMES -- never a value, never message text.
// evlog.Append drops any error silently, so a logging failure never
// changes or slows the tool's own result.
func logToolCall(name string, args map[string]any, outcome string, elapsed time.Duration) {
	names := make([]string, 0, len(args))
	for k := range args {
		names = append(names, k)
	}
	sort.Strings(names)
	evlog.Append(paths.ToolCallLogPath(), map[string]any{
		"time":        time.Now().UTC().Format(time.RFC3339),
		"tool":        name,
		"outcome":     outcome,
		"duration_ms": elapsed.Milliseconds(),
		"args":        names,
	})
}

func invoke(tool Tool, args map[string]any) (text string, isError bool) {
	defer func() {
		if r := recover(); r != nil {
			text = fmt.Sprintf("Byline hit an internal error: %v. Nothing was saved.", r)
			isError = true
		}
	}()
	result, err := tool.Handler(args)
	if err != nil {
		return err.Error(), true
	}
	data, err := json.Marshal(result)
	if err != nil {
		return err.Error(), true
	}
	return string(data), false
}

// checkArgs runs the argument checks in order: unknown names (sorted,
// first one named), required, type, blank string.
func checkArgs(tool Tool, args map[string]any) string {
	valid := map[string]bool{}
	var names []string
	for _, a := range tool.Args {
		valid[a.Name] = true
		names = append(names, a.Name)
	}
	var unknown []string
	for k := range args {
		if !valid[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		msg := fmt.Sprintf("%s does not take %q. It takes: %s.", tool.Name, unknown[0], strings.Join(names, ", "))
		if notice := updateNotice(); notice != "" {
			msg += " " + notice
		}
		return msg
	}
	for _, a := range tool.Args {
		if _, present := args[a.Name]; a.Required && !present {
			return fmt.Sprintf("%s needs %q.", tool.Name, a.Name)
		}
	}
	for _, a := range tool.Args {
		v, present := args[a.Name]
		if !present {
			continue
		}
		if msg := checkType(a, v); msg != "" {
			return msg
		}
	}
	for _, a := range tool.Args {
		if a.Type != "string" {
			continue
		}
		v, present := args[a.Name]
		if !present {
			continue
		}
		if sv, ok := v.(string); ok && strings.TrimSpace(sv) == "" {
			return fmt.Sprintf("%q is empty.", a.Name)
		}
	}
	for _, a := range tool.Args {
		v, present := args[a.Name]
		if !present {
			continue
		}
		if msg := checkStringsIn(a.Name, v); msg != "" {
			return msg
		}
	}
	return ""
}

// maxArgStringBytes caps any one string a tool argument carries, top-level
// or nested inside an object or array. byline_setup_save's real_voice
// sample -- Byline's single largest legitimate argument, about 4,000
// words of someone's own sent mail and Slack messages (SKILL.md step 3)
// -- runs well under 100 KiB; this leaves that room to spare while
// staying far below MaxMessageBytes, the whole message's own cap.
const maxArgStringBytes = 1 << 20 // 1 MiB

// checkStringsIn walks v -- a string, or an object or array that may hold
// strings at any depth -- rejecting the first string that isn't valid
// UTF-8, carries a hidden-instruction character (ASR "Model Input and
// Output Validation" / "Implement Tool Input and Response Validation": a
// Unicode tag character or a bidirectional override/isolate can hide or
// reorder text a person reading the draft never sees), or is longer than
// maxArgStringBytes. name is the enclosing argument's name, for the
// message. A zero-width joiner, zero-width non-joiner, the emoji
// variation selector and ordinary emoji are left alone -- people's drafts
// carry those on purpose.
func checkStringsIn(name string, v any) string {
	switch t := v.(type) {
	case string:
		if len(t) > maxArgStringBytes {
			return fmt.Sprintf("%q is longer than Byline allows (over %d bytes). Shorten it and try again.", name, maxArgStringBytes)
		}
		if !utf8.ValidString(t) {
			return fmt.Sprintf("%q isn't valid UTF-8 text. Fix it and try again.", name)
		}
		if r := hiddenChar(t); r != 0 {
			return fmt.Sprintf("%q holds a hidden character (U+%04X) that can disguise what it says. Remove it and try again.", name, r)
		}
	case map[string]any:
		for _, vv := range t {
			if msg := checkStringsIn(name, vv); msg != "" {
				return msg
			}
		}
	case []any:
		for _, vv := range t {
			if msg := checkStringsIn(name, vv); msg != "" {
				return msg
			}
		}
	}
	return ""
}

// hiddenChar returns the first Unicode tag character (U+E0000-U+E007F) or
// bidirectional override/isolate control (U+202A-U+202E, U+2066-U+2069)
// in s, or 0 when there is none.
func hiddenChar(s string) rune {
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		if n := subdivisionFlagLen(rs[i:]); n > 0 {
			i += n - 1
			continue
		}
		switch r := rs[i]; {
		case r >= 0xE0000 && r <= 0xE007F, r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069:
			return r
		}
	}
	return 0
}

// subdivisionFlagLen returns the rune length of the England, Scotland or
// Wales flag at the start of rs, or 0. Those three emoji spell their region
// in tag characters after a black flag; only these exact spellings pass, so
// tag characters can't carry any other hidden text.
func subdivisionFlagLen(rs []rune) int {
	for _, region := range []string{"gbeng", "gbsct", "gbwls"} {
		n := len(region) + 2
		if len(rs) < n || rs[0] != 0x1F3F4 || rs[n-1] != 0xE007F {
			continue
		}
		ok := true
		for k, c := range region {
			if rs[k+1] != 0xE0000+c {
				ok = false
				break
			}
		}
		if ok {
			return n
		}
	}
	return 0
}

func checkType(a Arg, v any) string {
	switch a.Type {
	case "string":
		if _, ok := v.(string); !ok {
			return fmt.Sprintf("%q must be a string.", a.Name)
		}
	case "integer":
		n, ok := v.(json.Number)
		if !ok || strings.ContainsAny(n.String(), ".eE") {
			return fmt.Sprintf("%q must be an integer.", a.Name)
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			return fmt.Sprintf("%q must be true or false.", a.Name)
		}
	case "object":
		if _, ok := v.(map[string]any); !ok {
			return fmt.Sprintf("%q must be an object.", a.Name)
		}
	case "array":
		if _, ok := v.([]any); !ok {
			return fmt.Sprintf("%q must be a list of strings.", a.Name)
		}
	}
	return ""
}
