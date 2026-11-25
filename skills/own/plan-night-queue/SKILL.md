---
name: plan-night-queue
description: Write tasks by day so the factory runs them unattended at night, and read what happened in the morning. Use when the owner wants to queue work for later, asks what the factory will do overnight, or a task stopped on a model limit.
---

# Plan the night queue

The factory works while tasks are queued. A model limit is not a failure: the job returns to the queue, the book keeps the limit and its reset, and the task goes on with the next provider of the role's chain or waits for the limit to clear (design section 12, ADR 0012). What a task needs to run unattended is a spec that does not need the owner.

## Write a task that can run without you

1. **One outcome, one acceptance check**, in one line of at most 120 characters: the line is the pull request title and Builder's whole task. A task that needs more words goes through Lead, which writes a spec you approve by day.
2. **Name the check the task must pass** ("`make verify` is green and `greet.sh` prints hello"), so Builder and the Verifier have something to run.
3. **Keep it small.** A task that touches gated paths (`schemas/`, `deploy/`, secrets, egress, review policy) stops at the owner's decision and wastes the night. Put those in a day batch.
4. **Check for hidden decisions.** If the task needs a new dependency, a design choice or a secret, settle it first: the factory parks such a task until morning and takes the next one.

## Queue it

```sh
nightshift task add <tenant> --not-before 2026-11-26T22:00:00+01:00 --priority 5 "feat: add greet.sh that prints hello"
```

- `--not-before` is an RFC 3339 time with a zone: the task stays queued until then. Leave it out to start at once.
- `--priority` is -10 to 10; a higher one is advanced first when several tasks are ready. Equal priorities go by age.
- Order a night by priority, not by the clock: put the task you most want finished at 5, filler at 0.
- A tenant that works from Slack, the terminal or any Claude client reaches the same queue through Lead; `task add` is the direct door.

## Read it in the morning

- The 09:00 summary lists what was done, what waits for you and the review queue.
- `nightshift status` lists open tasks and their states. A task in `building` with no new job has no usable provider: every provider of its role's chain is at a limit, and the next job starts when the earliest one clears (a limit without a stated reset waits 15 minutes, doubling to 6 hours).
- An `escalated` task needs you: its last event says why (budget, blocked, a verdict). A limit alone never escalates a task.

## Do not

- Do not queue a task you would not merge without looking: the owner's Approve and the review policy still gate the merge; the night only does the work.
- Do not raise `--priority` to 10 for everything; priority is only a tie-break among ready tasks.
- Do not put secrets, tokens or customer data in the task text. It is stored in the book and the pull request.
