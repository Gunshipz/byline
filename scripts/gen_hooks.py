# Writes byline/hooks/hooks.json (Claude Code's native format) from the send-tool list in
# server/internal/state/settings.go, so the blocking matcher always equals DefaultSendTools.
import json, re, sys
root = sys.argv[1]
src = open(f"{root}/server/internal/state/settings.go").read()
block = re.search(r"var DefaultSendTools = \[\]string\{(.*?)\n\}", src, re.S).group(1)
tools = re.findall(r'"(mcp__[^"]+)"', block)
assert all(re.fullmatch(r"mcp__[A-Za-z0-9_-]+", t) for t in tools)
names = "|".join(t[len("mcp__"):] for t in tools)
launcher = 'sh "${CLAUDE_PLUGIN_ROOT}/bin/byline-hook" hook '
def h(mode, timeout, block=False):
    e = {"type": "command", "command": launcher + mode, "timeout": timeout}
    if block:
        e["onFailure"] = "block"
    return e
doc = {"hooks": {
    "SessionStart": [{"hooks": [h("session-start", 10)]}],
    "UserPromptSubmit": [{"hooks": [h("prompt", 10)]}],
    "PostToolUse": [{"matcher": "^Read$", "hooks": [h("read", 10)]}],
    "PreToolUse": [
        {"matcher": f"^mcp__(plugin_.+_)?({names})$", "hooks": [h("pre-tool", 30, True)]},
        {"matcher": "^mcp__.*", "hooks": [h("pre-tool --extra", 30)]},
        {"matcher": "^Skill$", "hooks": [h("pre-tool", 30)]},
    ],
}}
open(f"{root}/hooks/hooks.json", "w").write(json.dumps(doc, indent=2) + "\n")
print(len(tools), "send tools")
