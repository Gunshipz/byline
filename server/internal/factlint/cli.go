package factlint

import (
	"fmt"
	"io"
	"os"

	"byline/internal/rules"
)

// RunCLI is the `byline-mcp factlint <draft> <source>` mode: it prints the
// flag count, one line per flag, and exits 1 when there are any, 0 when
// clean, or 2 on a usage or read error.
func RunCLI(args []string, set *rules.Set, stdout, stderr io.Writer) int {
	if len(args) != 2 {
		fmt.Fprintln(stderr, "usage: byline-mcp factlint <draft> <source>")
		return 2
	}
	draftPath, srcPath := args[0], args[1]

	draft, err := os.ReadFile(draftPath)
	if err != nil {
		fmt.Fprintf(stderr, "byline: %v\n", err)
		return 2
	}
	source, err := os.ReadFile(srcPath)
	if err != nil {
		fmt.Fprintf(stderr, "byline: %v\n", err)
		return 2
	}

	flags := Run(set, string(draft), string(source))

	plural := "s"
	if len(flags) == 1 {
		plural = ""
	}
	fmt.Fprintf(stdout, "%s vs source: %d source-fidelity flag%s\n", draftPath, len(flags), plural)
	for _, f := range flags {
		// A literal quote wrap, not Go's %q: factlint.mjs's console.log
		// interpolates f.val raw, with no escaping, into the template
		// string, so a val holding a backslash or a control character
		// must print unescaped too, to match it byte for byte.
		fmt.Fprintf(stdout, "  [%s] \"%s\"\n", f.Type, f.Val)
	}

	if len(flags) > 0 {
		return 1
	}
	return 0
}
