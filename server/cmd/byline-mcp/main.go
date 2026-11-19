package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"

	bembed "byline/embed"
	"byline/internal/draftcheck"
	"byline/internal/factlint"
	"byline/internal/gate"
	"byline/internal/lint"
	"byline/internal/mcp"
	"byline/internal/rules"
	"byline/internal/version"
	"byline/internal/voice"
)

// loadSet is rules.LoadPersonalFile, every checker mode's entry point
// (gate finding: this duplicated server/internal/mcp/checkers.go's
// currentRules, so both now call the one shared function). If the
// personal file fails to load, this prints LoadPersonalFile's warning
// once on stderr, since stdout and stderr are the CLI's own.
func loadSet(stderr io.Writer) (*rules.Set, error) {
	set, warning, err := rules.LoadPersonalFile()
	if warning != "" {
		fmt.Fprintln(stderr, "byline: "+warning)
	}
	return set, err
}

// extraModeHelp names any mode a copy's own file adds beyond the ones
// dispatched here, for the "modes:" help and unknown-mode text -- for
// example ", setup-quick". Empty by default.
var extraModeHelp string

// modeList names every mode run dispatches on, for "help" and an unknown
// mode (gate finding M10).
func modeList() string {
	return "modes: (none: MCP server), tool <name>, version, generate, draftcheck, factlint, lint, voicecheck, hook" + extraModeHelp + ", dev split"
}

// extraMode, if set, runs a mode a copy's own file adds beyond the ones
// dispatched here, returning ok=false to fall through to the
// unknown-mode message. nil by default.
var extraMode func(args []string, stdout io.Writer) (ok bool, code int)

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "version" {
		fmt.Fprintf(stdout, "byline-mcp %s\n", version.Version())
		return 0
	}
	if len(args) >= 1 && args[0] == "help" {
		fmt.Fprintln(stdout, modeList())
		return 0
	}
	if len(args) >= 1 && args[0] == "generate" {
		if len(args) != 1 {
			fmt.Fprintln(stderr, "byline: generate takes no arguments")
			return 2
		}
		p, err := rules.Generate()
		if p != "" {
			fmt.Fprintln(stdout, p)
		}
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	if len(args) >= 1 && args[0] == "dev" {
		if len(args) >= 2 && args[1] == "split" {
			core, _ := fs.Sub(bembed.Files, "rules")
			return rules.RunSplitCLI(args[2:], core, stdout, stderr)
		}
		fmt.Fprintln(stderr, "unknown mode")
		return 2
	}
	if len(args) >= 1 && args[0] == "draftcheck" {
		return draftcheck.RunCLI(args[1:], stdout, stderr)
	}
	if len(args) >= 1 && args[0] == "factlint" {
		set, err := loadSet(stderr)
		if err != nil {
			fmt.Fprintf(stderr, "byline: %v\n", err)
			return 2
		}
		return factlint.RunCLI(args[1:], set, stdout, stderr)
	}
	if len(args) >= 1 && args[0] == "lint" {
		set, err := loadSet(stderr)
		if err != nil {
			fmt.Fprintf(stderr, "byline: %v\n", err)
			return 2
		}
		return lint.RunCLI(args[1:], set, stdout, stderr)
	}
	if len(args) >= 1 && args[0] == "voicecheck" {
		set, err := loadSet(stderr)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		return voice.RunCLI(args[1:], set, stdout, stderr)
	}
	if len(args) >= 1 && args[0] == "hook" {
		hookMode := ""
		if len(args) >= 2 {
			hookMode = args[1]
		}
		if hookMode == "pre-tool" {
			for _, a := range args[2:] {
				if a == "--extra" {
					hookMode = "pre-tool-extra"
					break
				}
			}
		}
		// The launcher always exits 0: every decision rides in the printed
		// JSON, never the exit code.
		gate.Run(hookMode, os.Stdin, os.Stdout)
		os.Exit(0)
	}
	if len(args) >= 1 && extraMode != nil {
		if ok, code := extraMode(args, stdout); ok {
			return code
		}
	}
	if len(args) >= 1 {
		fmt.Fprintf(stderr, "byline: unknown mode %q\n%s\n", args[0], modeList())
	} else {
		fmt.Fprintln(stderr, modeList())
	}
	return 2
}

// isTerminal reports whether f is a character device -- a real terminal,
// not a pipe, redirected file or /dev/null -- so main (gate finding M14)
// can tell a person running byline-mcp by hand from every real launcher,
// which always pipes stdin.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// afterServe, if set, runs right after the MCP server's Serve loop
// returns and before the process exits -- for a copy whose start hook
// kicked off background work that needs to finish first. nil by
// default.
var afterServe func(srv *mcp.Server)

func main() {
	// No arguments: every real launcher starts byline-mcp this way and
	// talks to it over stdio, so this bypasses run()'s plain text modes
	// entirely rather than route JSON-RPC bytes through an io.Writer
	// meant for them.
	if len(os.Args) == 1 {
		// Gate finding M14: run by hand with no arguments, this used to
		// print nothing and just wait, so a person checking the install
		// saw a hang. Every real launcher pipes stdin, so this only fires
		// for a person at a real terminal.
		if isTerminal(os.Stdin) {
			fmt.Fprintln(os.Stderr, modeList())
		}
		srv := mcp.NewServer()
		srv.Serve(os.Stdin, os.Stdout)
		if afterServe != nil {
			afterServe(srv)
		}
		os.Exit(0)
	}
	if os.Args[1] == "tool" {
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: byline-mcp tool <name>")
			os.Exit(2)
		}
		os.Exit(mcp.RunToolMode(os.Args[2], os.Stdin, os.Stdout))
	}
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
