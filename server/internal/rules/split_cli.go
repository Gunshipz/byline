package rules

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

const splitUsage = "usage: byline-mcp dev split --rules <rules.json> --stance <stance.md> --out <dir> [--extras <extras.json>]"

// RunSplitCLI is the master's "dev split" mode: it reads today's
// rules.json and stance.md, splits them against core (Byline's shipped
// rules) with Split, and writes the result as a fresh my-rules.json and
// my-stance.md in --out.
func RunSplitCLI(args []string, core fs.FS, stdout, stderr io.Writer) int {
	fset := flag.NewFlagSet("split", flag.ContinueOnError)
	fset.SetOutput(io.Discard)
	rulesPath := fset.String("rules", "", "")
	stancePath := fset.String("stance", "", "")
	outDir := fset.String("out", "", "")
	extrasPath := fset.String("extras", "", "")
	if err := fset.Parse(args); err != nil || *rulesPath == "" || *stancePath == "" || *outDir == "" {
		fmt.Fprintln(stderr, splitUsage)
		return 2
	}

	oldBytes, err := os.ReadFile(*rulesPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	old, err := ParseCore(oldBytes)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	coreRules, err := LoadCore(core)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	stanceBytes, err := os.ReadFile(*stancePath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	var signatureLines, knownNames []string
	var voiceBaseline json.RawMessage
	if *extrasPath != "" {
		extrasBytes, err := os.ReadFile(*extrasPath)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		var e struct {
			SignatureLines []string        `json:"signature_lines"`
			KnownNames     []string        `json:"known_names"`
			VoiceBaseline  json.RawMessage `json:"voice_baseline,omitempty"`
		}
		if err := json.Unmarshal(extrasBytes, &e); err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		signatureLines, knownNames = e.SignatureLines, e.KnownNames
		if len(e.VoiceBaseline) > 0 && string(e.VoiceBaseline) != "null" {
			if err := ValidateBaseline(e.VoiceBaseline); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			voiceBaseline = e.VoiceBaseline
		}
	}

	if insideGitRepo(*outDir) {
		fmt.Fprintln(stderr, "--out is inside a git repository; personal rules must stay outside it")
		return 1
	}
	for _, name := range []string{"my-rules.json", "my-stance.md"} {
		p := filepath.Join(*outDir, name)
		if _, err := os.Stat(p); err == nil {
			fmt.Fprintf(stderr, "%s already exists; move it away first\n", p)
			return 1
		}
	}

	my, notes := Split(old, coreRules)
	my.SignatureLines = signatureLines
	my.KnownNames = knownNames
	if voiceBaseline != nil {
		my.VoiceTarget = setObjectKey(my.VoiceTarget, "baseline", voiceBaseline)
	}

	if err := writeSplitResult(*outDir, my, stanceBytes); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	fmt.Fprintf(stdout, "add_rules: %d\n", len(my.AddRules))
	fmt.Fprintf(stdout, "add_strings: %d rules\n", len(my.AddStrings))
	offCount := 0
	for _, offs := range my.SwitchOff {
		offCount += len(offs)
	}
	fmt.Fprintf(stdout, "switch_off: %d strings\n", offCount)
	fmt.Fprintf(stdout, "replace_examples: %d rules\n", len(my.ReplaceExamples))
	fmt.Fprintf(stdout, "voice_target: %d keys\n", len(my.VoiceTarget))
	for _, n := range notes {
		fmt.Fprintf(stdout, "note: %s\n", n)
	}
	return 0
}

// setObjectKey returns vt with key's value set to raw: overwritten in
// place if key is already present, appended otherwise.
func setObjectKey(vt Object, key string, raw json.RawMessage) Object {
	for i, kv := range vt {
		if kv.Key == key {
			vt[i].Value = raw
			return vt
		}
	}
	return append(vt, KV{Key: key, Value: raw})
}

// insideGitRepo reports whether out, or any of its parent folders, holds a
// .git entry.
func insideGitRepo(out string) bool {
	dir, err := filepath.Abs(out)
	if err != nil {
		dir = filepath.Clean(out)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

// writeSplitResult writes my-rules.json (two-space JSON, mode 0600),
// my-stance.md (stance's bytes, mode 0600) and state/rules-saved (now, UTC)
// into outDir.
func writeSplitResult(outDir string, my *MyRules, stance []byte) error {
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return err
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(my); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "my-rules.json"), buf.Bytes(), 0o600); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(outDir, "my-stance.md"), stance, 0o600); err != nil {
		return err
	}

	stateDir := filepath.Join(outDir, "state")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}
	saved := time.Now().UTC().Format("2006-01-02T15:04:05Z") + "\n"
	return os.WriteFile(filepath.Join(stateDir, "rules-saved"), []byte(saved), 0o600)
}
