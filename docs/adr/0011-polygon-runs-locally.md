---
status: accepted
date: 2026-11-25
---

# Run the polygon locally in throwaway git repositories

## Context and Problem Statement

The polygon needs a place where the factory solves reference tasks and where hidden tests grade the result. The design named a separate `nightshift-polygon` repository on GitHub. Evals must run without Slack, GitHub or the book (design section 20), and the owner decided on 2026-11-25 not to create a new GitHub repository and not to use the sandbox repository for evals for now, and to migrate later if needed. Where do polygon tasks live and run?

## Considered Options

- Tasks in `evals/tasks/` of this repository; each run makes a throwaway local git repository, the harness solves the task there, the hidden tests grade it, the repository is removed.
- A separate `nightshift-polygon` GitHub repository with real branches and PRs per run.
- The existing sandbox repository on GitHub.

## Decision Outcome

Chosen option: "local throwaway repositories", because it needs no token, no network and no new repository, costs nothing besides model calls, and cannot leak a polygon task into a real project.

- A task is a directory: `task.yaml`, `repo/` (starting files) and `hidden/` (grading files). Hidden files are copied in after the agent finished, so the agent cannot read or change them. Links and special files in a task are refused.
- The grader is the task's `test` command (a code grader). LLM judges and Python graders are added with the E1 rubric evals.
- `nightshift eval run` takes a fake harness with a scenario (for tests) or the `claude` harness with an explicit `--auth`. The subscription stays limited as in AGENTS.md.
- A result follows `schemas/eval-result.json`, carries a cache key over the task files and the candidate, and is written to `./eval-runs/`.
- Migration to a hosted repository later changes only where `repo/` comes from, not the task format or the results.

### Consequences

- Good: runs work on a laptop, in CI and in the worker without secrets.
- Good: the task format stays portable.
- Bad: no check that the factory's push and PR path works on a real host; the live smoke test of the whole flow still needs the owner's sandbox.
- Bad: tasks solved by a real agent need its toolchain on the machine that runs them.

## More Information

Design sections 10 (item 6) and 20 ("Where it lives"); ADR 0010 for how a worker gets toolchains.
