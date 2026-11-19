# Install and hosts

Byline runs in Claude Code, as a plugin.

## How Byline's tools are reached

Byline's helper comes inside the plugin. Each capability in SKILL.md's Tools table is an MCP tool of that name, called directly. If a tool call isn't available, fall back to running `<launcher> tool <name>` from the command line, with the tool's JSON arguments on stdin; it prints the JSON result. The launcher is `bin/byline-mcp`, two folders above this skill's base directory.

### Per-tool arguments

| Tool | Argument | Required | What it is |
|---|---|---|---|
| `byline_status` | (none) | – | no arguments; reports whether Byline is set up and current, and anything wrong with its files |
| `byline_brief` | (none) | – | no arguments; returns the brief, writing profile and sample messages |
| `byline_lint` | `text` | required | the text to check against banned strings and structural patterns |
| `byline_factlint` | `draft` | required | the drafted text whose claims are being checked |
| `byline_factlint` | `source` | required | the source material the draft's claims must trace back to |
| `byline_draftcheck` | `draft` | required | the drafted comm to check against its spec |
| `byline_draftcheck` | `spec` | required | the spec object: reader, budget, section map, kill list, questions |
| `byline_voicecheck` | `text` | required | the text to score against the person's voice target |
| `byline_flag` | `phrase` | optional | the phrase being flagged; give this or `test`, not both |
| `byline_flag` | `bad` | required | an example sentence using the phrase badly |
| `byline_flag` | `good` | required | an example sentence saying it well instead |
| `byline_flag` | `rule_id` | optional | an existing rule's id to extend, instead of creating a brand-new rule |
| `byline_flag` | `family` | optional | the rule family (1 to 6) a brand-new rule belongs to |
| `byline_flag` | `name` | optional | a name for a brand-new rule |
| `byline_flag` | `fix` | optional | the fix text for a brand-new rule |
| `byline_flag` | `test` | optional | in place of `phrase`, for a pattern with no fixed phrase: one sentence that says how to spot it; always makes a brand-new rule, so give `family`, not `rule_id` |
| `byline_calibrate` | `corpus` | required | the person's own messages, separated by lines of `---` |
| `byline_setup_save` | `profile` | required | the writing profile text to save |
| `byline_setup_save` | `exemplars` | required | the sample messages text to save |
| `byline_setup_save` | `real_voice` | required | the real-voice sample text to save, scored for the baseline |
| `byline_setup_save` | `signature_lines` | optional | the person's own sign-off block, so checks can cut it out |
| `byline_setup_save` | `accept_fewer_words` | optional | saves below the minimum word count anyway; pass only after telling the person how many words were found and getting their yes |
| `byline_settings_read` | (none) | – | no arguments; reads Byline's saved settings |
| `byline_settings_save` | `send_tools` | optional | the list of tool names Byline's send checks cover |
| `byline_settings_save` | `exempt_channels` | optional | channel ids exempt from the send check |
| `byline_settings_save` | `extra_draft_phrases` | optional | extra wording that marks a request as a message to write; it bans nothing |

## Claude Code

Install with two commands in a terminal: `claude plugin marketplace add gunshipz/byline`, then `claude plugin install byline@byline`, then restart Claude Code. Inside Claude Code, `/plugin marketplace add gunshipz/byline` and `/plugin install byline@byline` do the same. Nothing else to install: the helper is inside the plugin. Byline's checks block sends here. The gate counts only a real read of each of the three files at the paths the status check gives; naming a path doesn't count.

## Reading sent mail and Slack for setup

Use the email and Slack tools already connected in this app, such as a Gmail, Outlook or Microsoft 365, or Slack connector. Search for messages the person sent: their sent mail folder, and Slack messages from their own user. If a tool can change a message's read state, never let it, and ask the person once before opening their mail. Never connect or sign in to anything on the person's behalf.

If nothing is connected, or it can't find their sent messages, ask the person to paste some of their own messages, or to point you to a folder of `.txt` or `.eml` files (most mail apps can save a message as an `.eml` file). Read those with the app's file reader.

Keep only the person's own words from what comes back: drop quoted replies, forwarded text, other people's messages in a thread, and signatures. Stop at about 4,000 words of the person's own words.

## Updating

Quit Claude Code first, then run `claude plugin update byline@byline` in a terminal. If its output says the plugin isn't installed, this copy wasn't installed the usual way: reinstall with the two commands under Claude Code above. Otherwise its own output says whether it found a newer version or this was already the newest. Reopen Claude Code after.

## Supported machines

macOS on Apple chip and Intel, Windows x64 (with Git for Windows installed), and Linux on x64 and arm64.
