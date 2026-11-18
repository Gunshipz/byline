# Byline

Byline is a writing skill for Claude Code. It writes email and Slack messages in your own voice, traces every fact back to its source, and checks each message before it goes out. Your voice lives in `~/.byline/` on your own computer.

## Install

You need Claude Code 2.1.295 or later (`claude --version`).

In a terminal:

```
claude plugin marketplace add gunshipz/byline
claude plugin install byline@byline
```

Or inside Claude Code: `/plugin marketplace add gunshipz/byline`, then `/plugin install byline@byline`.

Then restart Claude Code. Nothing else to install: Byline's helper program ships inside the plugin.

## Using it

Ask Claude Code to draft, reply to or polish a message, or type one of these:

- `/byline:byline`: write a message (the default).
- `/byline:byline polish`: tighten a draft you already have.
- `/byline:byline check`: list the problems in someone else's text, without changing it.
- `/byline:byline review`: margin comments on someone else's draft.
- `/byline:byline flag`: stop Byline from ever writing a phrase again.
- `/byline:byline setup`: learn your writing voice.

## What it checks

Before an email or Slack message of 20 or more words goes out through a send tool Byline knows, Claude Code holds it until Byline's structure check has passed on that exact text in the last hour. Every Slack post to a person also waits for your yes. Change one word and the check runs again.

Out of the box, Byline knows the Outlook and Teams tools in Claude's Microsoft 365 connector, the two most-used open-source Gmail servers, and the most-used open-source Slack server. Claude's own Gmail and Slack connectors don't publish their tool names, so during `/byline:byline setup` Byline adds whichever send tools you have connected. To cover another one at any time, ask Claude Code to add it to Byline's settings.

## First run

Byline offers to learn your voice as soon as you send your first message, whatever it is. If you say yes, it reads about 4,000 words of your own sent email and Slack messages, through whatever email and Slack tools you've connected to Claude Code, and shows you the profile it built before saving anything; if you say no, it drops the question for the rest of that conversation. If you haven't connected any, paste some of your messages, or point it to a folder of `.txt` or `.eml` files. If it finds fewer than 1,000 words (a new hire, for example), it says how many it found and asks before saving. It also leaves out messages that read like an assistant drafted them, such as status reports with headings or bullet lists, and tells you how many. The AI model reads the messages to build the profile; nothing is saved anywhere except your voice folder, `~/.byline/`.

## Privacy and usage tracking

Byline has no usage tracking. Its helper makes no network calls and sends nothing anywhere.

It keeps two local logs under `~/.byline/logs/`: a line per tool call, and a line per send-check decision. Neither ever holds message text. See `docs/security-runbook.md` for what each log holds and how to read it.

## Known limitations

- Windows needs Git for Windows installed for Byline's checks to run.
- A draft that only appears in chat, and is never sent or saved, isn't checked.
- A send tool Byline doesn't know goes through unchecked until you add it to Byline's settings.
- If Byline's helper is missing or broken, only the send tools it knows are held for your approval.

## Updating

Ask Claude Code: `/byline:byline update` checks your version and installs the newest one for you, then tells you to restart. To do it yourself instead: quit Claude Code, then run `claude plugin update byline@byline`.

## Turning it off

`claude plugin uninstall byline@byline`. Your voice folder, `~/.byline/`, stays until you delete it.

## License

MIT. Byline started as a modified version of [humanizer](https://github.com/blader/humanizer). See `LICENSE` and `THIRD-PARTY-NOTICES`.
