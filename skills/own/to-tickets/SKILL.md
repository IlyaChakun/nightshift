---
name: to-tickets
description: Slice a spec into tasks Builder can finish in one run, with ids and depends_on links. Use when writing the tasks of a spec, or when an Edit changes the slicing.
---

# To tickets

Fill `tasks` of the spec: `T-1`, `T-2`... each with a `title` and, when needed, `depends_on` and `repo`.

## Rules

- **One task, one pull request, one concern,** under about 400 changed lines. A small change is one task; do not split to look thorough.
- **Vertical slices.** Each task leaves the system working and testable. Not "all schemas, then all handlers".
- Include `not_before` with an explicit UTC offset and `priority` from -10 to 10 only when the owner specifies a schedule. Ask for the time zone if it is unclear. Higher priorities run first among eligible tasks.
- **Dependencies are links, not prose.** `depends_on: [T-1]` only when T-2 cannot start before T-1 is released. The dispatcher runs only unblocked tasks, and a dependency counts as closed at `released` or `merged_by_team`. No cycles.
- **Title in Conventional Commit style,** specific enough that the commit message follows from it.
- **Every task names the criteria it satisfies** in its description (`AC-1`, `AC-3`), so Verifier can map evidence.
- **Cross-repository work:** set `repo` (`host/owner/name`) per task; each task gets its own pull request, linked by the spec.
- Put risky or uncertain work first so it fails early.

## After Edit

Edit stops only the spec's unmerged tasks. Changing what is already merged means new tasks, not rewriting old ones.
