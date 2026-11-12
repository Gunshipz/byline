# Contributing to Byline

## Setup

Go 1.22 or later. Nothing else: `byline-mcp` is written against the Go standard library only, so there's no Node, Python or extra tool to install.
## Tests

```
cd server && go test ./...
```

Some tests read `BYLINE_PRIVATE_FIXTURES`, the folder of private fixtures (real writing samples, never in this repo), and skip silently when it's unset. Nothing sets it for you, so set it by hand for every release run.

A test that means the shipped version reads it with `version.Version()`. Never write a version number into a test.

After changing anything under `skills/byline/`, `rules/` or `templates/`, run:

```
sh server/scripts/embed.sh
```

and commit the refreshed copies under `server/embed/`. `TestEmbed_CommittedCopiesInSync` fails until you do; that's the test catching a forgotten embed, not a bug in your change.
## Where things live

- `skills/byline/`: the skill Claude Code loads: `SKILL.md`, `skill.json` (the one source of the version), and `references/`.
- `rules/`: the shared writing rules and stance everyone starts from.
- `templates/`: the blank voice-folder files a new person's setup copies from.
- `server/`: the Go module. `cmd/byline-mcp/` is the single program that runs as the MCP server, the hook launcher's target, and the command-line tool; `internal/` holds its packages, one per concern: rules, lint, factlint, draftcheck (including `MessageText`, the one way both the send gate and the receipts read a message), voice, gate (the send gate), state, mcp, version, paths, evlog (the two local logs) and textutil, plus content, which holds only tests (scans over the shipped content, including the privacy scan). `embed/` is the committed copy of `skills/byline/`, `rules/` and `templates/` that `//go:embed` compiles into the binary; `embed.sh` leaves `skills/byline/evals/` out on purpose. `scripts/` holds `build.sh` and `embed.sh`; `testpackaging/` holds the tests over the packaging (docs, hooks, launchers, build).
- `bin/`: the two launcher scripts and the five platform binaries. The binaries are committed, because Claude Code installs a plugin straight from this repo.
- `.claude-plugin/`, `.mcp.json`, `hooks/hooks.json`: how Claude Code finds and wires in the plugin.
- `docs/security-runbook.md`: what the two local logs hold.

## Building

```
sh server/scripts/build.sh bin
```

builds `byline-mcp` for macOS (Apple chip and Intel), Linux (x64 and arm64) and Windows x64 into `bin/`, next to the two launchers.

## How Byline works

- **One program, many modes.** `byline-mcp help` lists them. With no arguments it's the MCP server, over stdin and stdout only, with no network port. `tool <name>` runs one tool, with its JSON arguments on stdin. `hook <event>` is what the Claude Code hooks run, through `bin/byline-hook`, which always exits 0 and passes on only the program's JSON reply. `lint`, `factlint`, `draftcheck` and `voicecheck` run the checks on the command line, `generate` rewrites the voice folder's brief, and `dev split` did the one-time split of the owner's old rules into core and personal rules.
- **The send gate.** Before a send tool runs (the send tools in `state.DefaultSendTools`, plus any the person adds in settings), `hook pre-tool` reads the outgoing text. A message of 20 or more words, or 2 or more sentences and 12 or more words, needs a structure check (`byline_draftcheck`) that passed in the last 60 minutes, and every Slack post to a person asks first. Each passing check writes a receipt to `~/.byline/receipts/` holding the checked copy's content words. The gate lets a message through when at least 90% of one receipt's words are in it and it has no more new words than 8, or 15% of its length if that's more. Both sides read text through `draftcheck.MessageText`, which drops HTML markup, style blocks and link syntax, and cuts the sign-off block from the end. A block starts only at one of the person's saved `signature_lines` (or a closing line such as "Thanks,") and is 30 words or fewer.
- **The person's files** live in `~/.byline/` (`BYLINE_HOME`): the voice files, personal rules (`my-rules.json`, including `signature_lines`), settings, receipts, and the two local logs in `logs/` (see `docs/security-runbook.md`).
- **No usage tracking.** Byline's program makes no network calls.
- **Where it ships.** This repo is both the Claude Code marketplace and the plugin: `.claude-plugin/marketplace.json` lists one plugin whose source is the repo root, and `.claude-plugin/plugin.json`, `.mcp.json`, `hooks/hooks.json`, `skills/` and `bin/` are what Claude Code loads.
## Rules for changes

- No personal names, aliases, emails, channel or account ids, or project names anywhere in the shared part of the repo (everything outside `~/.byline/` and the private fixtures). The exceptions are the copyright lines in `LICENSE` and `THIRD-PARTY-NOTICES`, and the made-up people and companies in `skills/byline/evals/files/`, which the evals need to read like real mail. Those use `.example` addresses and must never be a real person's name or messages.
- A rule id is never changed or reused. Retire a rule with its `retired` field instead of deleting or renumbering it.
- Bump `version` in `skills/byline/skill.json` and add a `CHANGELOG.md` line in the same change.
- Raise `requires_helper` only when the skill needs something an older helper build refuses to do; don't bump it on every release.

## Release checklist

1. Work on a branch.
2. Bump `version` in `skills/byline/skill.json` and `.claude-plugin/plugin.json`, and add a `CHANGELOG.md` entry headed with the new version. Raise `requires_helper` only when the skill needs something an older helper refuses to do.
3. Run `sh server/scripts/embed.sh`, then `sh server/scripts/build.sh bin`.
4. Run `(cd server && go test ./...)` and expect exit 0. If you keep private fixtures, also run `(cd server && BYLINE_PRIVATE_FIXTURES=<folder> go test ./internal/content/ -run TestSharedPart_NoScrubTerms -v)` and expect PASS, not SKIP.
5. Run `claude plugin validate .` and expect no errors.
6. Merge, then tag the release: `claude plugin tag .`.
