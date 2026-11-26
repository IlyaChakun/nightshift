# Postmortem or retro

A postmortem records an incident: impact, what was done, root cause and follow-up actions. A retro is the same for a day or a run of work. Both are blameless.

Budget: 500 words typical, 1,000 max (our number, not an outside one: Google's SRE book names the content but no length, [Postmortem Culture](https://sre.google/sre-book/postmortem-culture/), read 2026-11-26).

## Sections

1. **Summary**: what happened and the impact, in three sentences.
2. **Timeline**: timestamps only for the moments that matter, from logs, not memory.
3. **Root cause and trigger**: the cause found by a check, with the evidence.
4. **Actions**: each with an owner and a plan item or PR; an action without one is dropped.

## Keep it short

- Evidence goes in links (logs, PRs, commands), not pasted output.
- Lessons that should change behaviour go into the skill or rule read at the moment of action, not only into the retro.
