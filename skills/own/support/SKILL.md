---
name: support
description: Handle a problem report (something broken, not a wish) before any task is opened: look for reasons not to open one, reproduce it from main, set a priority. Use when the owner reports a bug, an error or a wrong result.
---

# Support

A problem report is not a task yet. Your job is to find out whether it needs one, and to hand the owner a finding, not a worry.

## Steps

1. **Look for reasons not to open a task.** In this order, and stop at the first that holds:
   - a duplicate: the search hits in your prompt show a task, PR or commit about the same thing; answer with the link and open nothing;
   - expected behaviour: the design, the PRD or a decision says it works this way; quote the line and its file;
   - already fixed on `main`: a commit or test covers it.
   Each is a plain answer (`blocker_or_summary`, no spec) that says what you checked.
2. **Reproduce from `main`.** In your worktree, never on production or on user data. Write the smallest failing test or run the command that shows the failure. Use the project's own test command (`verify` in `.nightshift/project.yaml`). Do not install anything beyond what the project's setup did.
3. **Say what you saw.** `reproduced: true` only when you ran the steps and saw the failure; the evidence is the test name and its message, or the command and its output. If you could not reproduce it, `reproduced: false`, and the evidence says what you tried and what stopped you. Never write a finding from reading the code alone.
4. **Set the priority.** P0 production is down or data is lost. P1 a main flow is broken. P2 it works with a workaround. P3 cosmetic. When unsure between two, take the higher and say why in the evidence.
5. **Ask only what changes the result.** One question, with options, when you cannot tell expected behaviour from a bug or lack a step to reproduce. The failing steps come from the owner's words first; do not invent them.
6. **Return the spec.** A fix task: title `fix: ...`, the goal is the outcome, the first criterion is the failing reproduction turned green (`given` the steps, `when` they are run, `then` the right result), and add `report` (`priority`, `reproduced`, `steps`, `evidence`). The owner sees [Create task] and [Not needed].

## Rules

- A failing test you wrote in the worktree stays there: only documents leave your worktree. Put its name and path in the evidence; the Builder writes the real test.
- If the problem is in the factory's own repository, the same steps apply.
- A report with `reproduced: false` is allowed; the card says NOT reproduced in front of the owner. Do not soften it.
