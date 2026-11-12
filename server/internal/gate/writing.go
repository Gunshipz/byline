// writing.go is the pre-tool check that stops the polish pass or a send
// until the three writing files have actually been read in this
// conversation, with a conversation-log fallback for when the session's
// own read marks were lost (a resumed or compacted session).
package gate

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
)

// transcriptTailBytes is how much of a conversation transcript's end
// transcriptReads looks at, so a huge log never makes the gate slow.
const transcriptTailBytes = 20 << 20

// transcriptReads scans transcriptPath for genuine Read tool_use entries
// and returns which of the required files they opened, in requiredFiles
// order, without duplicates. nil when there's no log to read.
//
// It reads at most the last transcriptTailBytes of the file and splits
// lines with bytes.Split, never bufio.Scanner, whose 64 KiB token limit
// would stop the scan at the first long line. A line that doesn't even
// contain "Read" is skipped before any JSON work. message.content is
// decoded as []json.RawMessage and each item decoded on its own, so one
// odd item never drops the rest of the line. Any error skips that line;
// nothing here ever panics.
func transcriptReads(transcriptPath string) []string {
	if transcriptPath == "" {
		return nil
	}
	info, err := os.Stat(transcriptPath)
	if err != nil {
		return nil
	}
	f, err := os.Open(transcriptPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var buf []byte
	if info.Size() <= transcriptTailBytes {
		buf, err = io.ReadAll(f)
		if err != nil {
			return nil
		}
	} else {
		buf = make([]byte, transcriptTailBytes)
		if _, err := f.ReadAt(buf, info.Size()-transcriptTailBytes); err != nil {
			return nil
		}
	}

	found := map[string]bool{}
	for _, line := range bytes.Split(buf, []byte("\n")) {
		if !bytes.Contains(line, []byte("Read")) {
			continue
		}
		var entry struct {
			Message struct {
				Content []json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &entry) != nil {
			continue
		}
		for _, raw := range entry.Message.Content {
			var item struct {
				Type  string `json:"type"`
				Name  string `json:"name"`
				Input struct {
					FilePath string `json:"file_path"`
				} `json:"input"`
			}
			if json.Unmarshal(raw, &item) != nil {
				continue
			}
			if item.Type != "tool_use" || item.Name != "Read" {
				continue
			}
			if key := requiredKey(item.Input.FilePath); key != "" {
				found[key] = true
			}
		}
	}

	var out []string
	for _, rf := range requiredFiles {
		if found[rf.key] {
			out = append(out, rf.key)
		}
	}
	return out
}

// missingReads reports which required files slug hasn't been recorded
// reading yet, skipping any file that isn't on disk (nothing to wait for
// there). Only when something is still missing does it fall back to
// scanning the conversation transcript, record whatever real Read calls
// it finds, and check again.
func missingReads(slug, transcriptPath string) []string {
	stillMissing := func() []string {
		var out []string
		for _, f := range requiredFiles {
			if !fileExists(f.path()) {
				continue
			}
			if !hasRead(slug, f.key) {
				out = append(out, f.key)
			}
		}
		return out
	}

	missing := stillMissing()
	if len(missing) == 0 {
		return missing
	}
	for _, key := range transcriptReads(transcriptPath) {
		recordRead(slug, key)
	}
	return stillMissing()
}

// isPolishSkill reports whether input is a Skill call invoking Byline's
// polish pass: the skill's last ":"-separated segment is "byline" and the
// first whitespace-separated word of args is "polish", case-insensitive.
func isPolishSkill(input map[string]any) bool {
	skill := str(input["skill"])
	if i := strings.LastIndex(skill, ":"); i >= 0 {
		skill = skill[i+1:]
	}
	if skill != "byline" {
		return false
	}
	fields := strings.Fields(strings.ToLower(str(input["args"])))
	return len(fields) > 0 && fields[0] == "polish"
}

// writingGated reports whether toolName's call is one checkTool holds for
// the three writing files: a send, or the polish pass. Nothing else is
// gated, including Write, Edit, Agent and Task, which today's hooks gate
// but no settings route to.
func writingGated(toolName string, input map[string]any, isSend bool) bool {
	return isSend || (toolName == "Skill" && isPolishSkill(input))
}
