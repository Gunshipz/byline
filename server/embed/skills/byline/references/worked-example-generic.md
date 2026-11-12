# Worked example

A first draft, what the checks caught, what no check caught, and the final version. Every fact traces to the source. Never paste this into output.

## The source

```text
From your manager: Can you give me a short read on the intake pilot before Thursday's review? I need to know whether it's working and what you'd change.

Your notes: The pilot ran 6 weeks with two teams. Requests routed: 212. Median time to first response fell from 3 days to 1 day. 18 requests went to the wrong team, and all 18 came from the free-text category. Survey: 9 of 14 requesters said the form was easier than email. I want to drop the free-text category and keep the pilot to the same two teams for 4 more weeks. I'm not sure the survey sample is big enough to mean much.
```

## The plan for the message

```json
{"reader":"your manager","intent":"inform","questions":["is the pilot working","what would you change"],"budget":120,"sectionMap":{},"killList":[]}
```

## First draft

```text
Hi,

Here's the thing: the pilot is working. Over six weeks the two teams routed 212 requests, and response time dropped dramatically, from 3 days to 1, highlighting the need to roll it out everywhere. Requesters love it — 11 of 14 said it beats email.

The one weak spot? Misrouted requests, and that's what keeps the whole thing honest.

I'd suggest we expand to every team next quarter. Happy to walk you through it before Thursday.
```

## What the checks caught

The phrase check found 6 problems: a forced reaction phrase ("Here's the thing"), a setup-colon-payoff ("Here's the thing: the pilot is working."), intensifier padding ("dropped dramatically"), an em dash, a trailing significance clause ("highlighting the need to roll it out everywhere"), and a concierge closer ("Happy to walk you through it"). The fact check found 1 problem: the number 11 isn't in the source, which says 9.

## What no check caught

The recommendation to expand to every team contradicts the source, which keeps two teams for 4 more weeks. "Requesters love it" invents a reaction the source doesn't report. The caveat about the small survey and the plan to drop the free-text category were dropped. "The one weak spot?" answers its own question instead of letting the reader judge it. "Keeps the whole thing honest" grades the argument rather than reporting it. The lesson: the phrase fixes were the easy part, and only tracing each claim to the source finds the worst errors.

## Final version

```text
Hi,

The pilot is working on speed. Over six weeks the two teams routed 212 requests, and the median time to a first response fell from 3 days to 1.

The problem is routing: 18 requests went to the wrong team, and all 18 came from the free-text category. I'd like to drop that category and keep the pilot to the same two teams for 4 more weeks.

9 of 14 requesters said the form was easier than email, though I'm not sure a sample that small tells us much.
```
