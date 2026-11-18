# Changelog

What changed in each version of Byline, in plain English.

## 1.0.4 (18 Nov 2026)

- Docs no longer talk about usage tracking. Byline never had any; the wording just said so, and it's gone now.

## 1.0.3 (18 Nov 2026)

- You can now type `/byline:byline update` to update Byline itself. It says the version you're on, runs the update, and tells you to restart Claude Code.
- Byline's own commands — `/byline:byline update`, `setup`, `flag`, `check` and `review` — are no longer mistaken for a request to write a message.

## 1.0.2 (18 Nov 2026)

- Setup now starts on your very first message, whatever it says, instead of waiting for you to ask for a draft or tell Claude to set it up. You still get asked, and get to say no.
- The session-start message no longer tells you to ask Claude to set up Byline; it just says setup will start with your first message (or, if files are missing, that something needs fixing).
- Setup won't save a voice profile built from fewer than 1,000 words of your own writing unless you've said it's fine to go ahead with less, which Byline asks about by name (so a new hire without that much writing yet can still get set up).
- Setup now skips any message of yours that reads like it was drafted by an assistant (a status report with headings or bullet lists, or one that trips 2 or more of the general rules at once), so a sample like that can no longer switch off the rules meant to catch AI writing. It also only switches a general phrase off once your own messages use it in 2 different places, not just 1.

## 1.0.0 (12 Nov 2026)

- First public release. Byline writes email and Slack messages in your own voice, checks each one before it goes out, and turns phrases you flag into rules.
