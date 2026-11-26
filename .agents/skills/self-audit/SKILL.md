---
name: self-audit
description: Check your own work against reality before you report a task, item or phase as done. Use before every report, PR description and plan tick; the answers go into the PR and the report.
---

# Self-audit

A review of a diff checks the diff against the ask. This skill checks the promise against the reality: what the change, a prompt, a schema, the design or the plan says the factory does, against what the code does when it runs. The owner should never have to ask "is it really done?"; ask yourself first, in writing.

## When

Before you write "done", tick a plan item, mark a phase closed, open a PR for review, or answer the owner's "is X good?". For a question about existing work, run the same steps on the thing asked about.

## Steps

1. **Claims.** List what you are about to claim, one line each. For every claim name the evidence you ran or read in this session: a command and its output, a test, a file and line. A claim without evidence is cut or marked **unverified**. A number, duration or count you did not measure is labelled an estimate (`~`); take durations from timestamps. A number taken from another agent's report is checked against its source or marked "not checked".
2. **Promises.** Find every place the change, or the thing you audit, promises behaviour to someone: role prompts and skills (`roles/*.md`, `skills/`), schemas, `docs/design.md`, `README.md`, `docs/runbook.md`, `docs/setup.md`, error and help texts. For each promise find the code that keeps it. A command, tool, field, key or flag named in a text must exist and be read. Run `go test ./cmd/nightshift -run 'TestPrompts|TestRole|TestSchemaFields|TestDesign|TestAgents'` and read what it says; a new allowlist entry in `promises_test.go` needs a plan item, never silence it to pass.
3. **Not run live.** List what you did not run on the real thing: the real stack (`make drill`, `nightshift doctor`), a real harness, a real GitHub or chat call, the worker image, an upgrade, a restore. A unit test on a fake does not count as live. Say which claims lean on a fake. A phase or an item is not ticked as done while its Done criterion has only fake-backed evidence; tick it "built" in words, and leave a plan item for the live check.
4. **Neighbours.** Name what else reads or writes what you changed: callers, other roles, the migration, the schema's other consumers, the docs that quote it, the design page (`docs/design.html`), the plan and status. Open each and check it still says the truth. Ask what breaks for a tenant, a fresh install, a restart, a red `main`.
5. **The skeptical owner.** Write the three questions the owner would ask on reading your report ("is it really live?", "what did you not check?", "what happens when it fails?") and answer them with evidence, or move them to the report as open items.
6. **Fix or declare.** A gap you can close in this PR: close it. One you cannot: a plan item and a line in `docs/status.md`. A gap you did not look for does not count as absent.

## Output

Put this block in the PR description and at the end of your report:

```text
Self-audit
- Claims and evidence: <claim: command or file that shows it>
- Promises checked: <texts and the code that keeps them; gaps found and what was done>
- Not verified: <what was not run live, and why>
- Neighbours checked: <what else was opened>
- Skeptical questions: <question: answer or open item>
```

"Not verified: nothing" is acceptable only when the live checks are listed above it. The basis line of AGENTS.md ("Evidence") follows the block.
