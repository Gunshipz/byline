package draftcheck

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const usage = "usage: byline-mcp draftcheck <draft> <spec.json>   |   byline-mcp draftcheck --selftest"

// RunCLI implements "byline-mcp draftcheck <draft> <spec.json>" and
// "byline-mcp draftcheck --selftest". Exit 0 on pass, 1 on any failed
// check, 2 on a usage error, an unreadable draft or bad spec JSON.
func RunCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "--selftest" {
		return Selftest(stdout)
	}
	if len(args) < 2 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	draftPath, specPath := args[0], args[1]
	draftBytes, err := os.ReadFile(draftPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	specBytes, err := os.ReadFile(specPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	var spec Spec
	if err := json.Unmarshal(stripBOM(specBytes), &spec); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	r := check(string(draftBytes), spec)
	Render(stdout, draftPath, r)

	if r.Pass {
		p, err := WriteReceipt(draftPath, specPath, r.Text, time.Now())
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		fmt.Fprintf(stdout, "\nall checks passed — receipt %s\n\n", filepath.Base(p))
		return 0
	}

	failed := 0
	for _, c := range r.Checks {
		if !c.Pass {
			failed++
		}
	}
	fmt.Fprintf(stdout, "\n%d FAILED — do not show this to the user yet\n\n", failed)
	return 1
}
