package lint

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"byline/internal/rules"
)

type jsonResult struct {
	File  string      `json:"file"`
	Count int         `json:"count"`
	Hits  []Violation `json:"hits"`
}

// RunCLI is the master's `byline-mcp lint <draft> [--json]`. It returns 0
// when clean, 1 when there are violations, 2 on a usage error or an
// unreadable draft.
func RunCLI(args []string, set *rules.Set, stdout, stderr io.Writer) int {
	// M10: --json is accepted in any position; any other flag-shaped
	// argument is a usage error, not silently ignored or read as the file.
	var file string
	asJSON := false
	for _, a := range args {
		if strings.HasPrefix(a, "--") {
			if a != "--json" {
				fmt.Fprintf(stderr, "byline: unknown flag %q\n", a)
				return 2
			}
			asJSON = true
			continue
		}
		if file != "" {
			fmt.Fprintf(stderr, "byline: unexpected argument %q\n", a)
			return 2
		}
		file = a
	}
	if file == "" {
		fmt.Fprintln(stderr, "usage: byline-mcp lint <draft> [--json]")
		return 2
	}

	b, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintf(stderr, "byline: %v\n", err)
		return 2
	}

	hits := Run(set, string(b))

	if asJSON {
		out := hits
		if out == nil {
			out = []Violation{}
		}
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		enc.Encode(jsonResult{File: file, Count: len(hits), Hits: out})
	} else {
		printText(stdout, file, hits, set)
	}

	if len(hits) > 0 {
		return 1
	}
	return 0
}

// printText is lint.mjs:176-181's human-readable report.
func printText(w io.Writer, file string, hits []Violation, set *rules.Set) {
	plural := "s"
	if len(hits) == 1 {
		plural = ""
	}
	fmt.Fprintf(w, "%s: %d violation%s\n", file, len(hits), plural)
	for _, h := range hits {
		fmt.Fprintf(w, "  L%d  [%s] fam%d %s: \"%s\"\n", h.Line, h.RuleID, h.Family, h.Name, h.Match)
	}
	if len(hits) == 0 {
		return
	}

	famName := map[int]string{}
	for _, f := range set.Families {
		famName[f.ID] = f.Name
	}
	var order []string
	seen := map[string]bool{}
	counts := map[string]int{}
	for _, h := range hits {
		name := famName[h.Family]
		if !seen[name] {
			seen[name] = true
			order = append(order, name)
		}
		counts[name]++
	}
	fmt.Fprintf(w, "  families: %s\n", familyCountsJSON(order, counts))
}

// familyCountsJSON is a compact JSON object of family name to count, in
// the order names were given (first-hit order), matching
// JSON.stringify(byFam) in lint.mjs.
func familyCountsJSON(order []string, counts map[string]int) string {
	var b []byte
	b = append(b, '{')
	for i, name := range order {
		if i > 0 {
			b = append(b, ',')
		}
		key, _ := json.Marshal(name)
		b = append(b, key...)
		b = append(b, ':')
		b = append(b, []byte(fmt.Sprintf("%d", counts[name]))...)
	}
	b = append(b, '}')
	return string(b)
}
