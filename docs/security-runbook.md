# Byline security runbook

Two local, append-only logs, both under `BYLINE_HOME/logs/`, never synced or sent anywhere.

## tool-calls.jsonl

One line per tool call Byline's MCP server handles: UTC time, the tool's name, its outcome
(`ok`, `error`, or `rejected` when the argument check refused it), how long it took, and the
sorted names of the arguments it received.

Never holds: an argument's value, message text, or file contents.

## gate-decisions.jsonl

One line per PreToolUse decision the send gate actually makes: UTC time, the hook event, the
tool, the decision
(`allow`, `ask`, `deny`, or `none` when the gate had no opinion), and a short reason code.

Never holds: the reason text shown to the person, message text, or an argument value.

Both logs are capped at 5 MiB; past that, the file rolls to `.1` (replacing any older one)
and starts fresh.

## Spotting trouble

- Several `deny` or `ask` lines in `gate-decisions.jsonl`, or several `rejected` lines in
  `tool-calls.jsonl`, clustered close together.
- A tool name in either log that isn't one of Byline's own or a known send tool.

## If the helper is missing or crashes

`hooks/hooks.json` sets `onFailure: "block"` only on the hook matching the send tools Byline
knows: if the helper can't run or times out there, Claude Code blocks that call. The hook matching every
other MCP tool call, and the one matching the Skill tool, set no `onFailure`, so a helper
that can't run or times out there doesn't block the call.

Inside the helper itself, a crash while deciding a direct send is caught and turned into an
explicit `ask`; a crash while deciding the broader per-tool check is also caught, but prints
no decision at all. Either way the process always exits 0, so `onFailure` can only ever fire
from the helper not running or timing out, never from a crash inside it.

## Turning Byline off at once

`claude plugin uninstall byline@byline`. Your voice folder, `~/.byline/`, stays until you
delete it.

## Questions

Open an issue at https://github.com/gunshipz/byline/issues.
