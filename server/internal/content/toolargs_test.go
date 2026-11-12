package content

import (
	"regexp"
	"strings"
	"testing"

	"byline/internal/mcp"
)

// toolArgRowRe matches one row of install-and-hosts.md's "Per-tool
// arguments" table: a tool name, then either an argument name or the
// literal "(none)", then "required"/"optional" or "–" for a no-argument
// row.
var toolArgRowRe = regexp.MustCompile("^\\|\\s*`([a-z_]+)`\\s*\\|\\s*(?:`([a-z_]+)`|\\(none\\))\\s*\\|\\s*(required|optional|–)\\s*\\|")

// TestInstallAndHosts_ToolArgumentTableMatchesTools fails if
// install-and-hosts.md's "Per-tool arguments" table drops out of sync
// with mcp.Tools()'s real input schemas: a missing tool, a missing
// argument, a required/optional flag that doesn't match, or a stale row
// naming a tool or argument mcp.Tools() no longer has.
func TestInstallAndHosts_ToolArgumentTableMatchesTools(t *testing.T) {
	s := readFile(t, referencePaths[0])

	type row struct {
		required bool
		hasArg   bool
	}
	got := map[string]map[string]row{}
	for _, line := range strings.Split(s, "\n") {
		m := toolArgRowRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		tool, arg, requiredWord := m[1], m[2], m[3]
		if got[tool] == nil {
			got[tool] = map[string]row{}
		}
		if arg == "" {
			continue // a "(none)" row only proves the tool is in the table
		}
		got[tool][arg] = row{required: requiredWord == "required", hasArg: true}
	}

	for _, tl := range mcp.Tools() {
		toolRows, ok := got[tl.Name]
		if !ok {
			t.Errorf("install-and-hosts.md's table is missing tool %q", tl.Name)
			continue
		}
		for _, arg := range tl.Args {
			r, ok := toolRows[arg.Name]
			if !ok {
				t.Errorf("install-and-hosts.md's table is missing %s's argument %q", tl.Name, arg.Name)
				continue
			}
			if r.required != arg.Required {
				want := "optional"
				if arg.Required {
					want = "required"
				}
				t.Errorf("install-and-hosts.md's table marks %s's %q wrong, want %q", tl.Name, arg.Name, want)
			}
		}
		if len(toolRows) != len(tl.Args) {
			t.Errorf("install-and-hosts.md's table has %d row(s) for %s's arguments, want %d (mcp.Tools() names: %v)",
				len(toolRows), tl.Name, len(tl.Args), argNames(tl.Args))
		}
	}

	want := make(map[string]bool, len(mcp.Tools()))
	for _, tl := range mcp.Tools() {
		want[tl.Name] = true
	}
	for tool := range got {
		if !want[tool] {
			t.Errorf("install-and-hosts.md's table names %q, which mcp.Tools() doesn't have", tool)
		}
	}
}

func argNames(args []mcp.Arg) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = a.Name
	}
	return out
}
