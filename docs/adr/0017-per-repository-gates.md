---
status: accepted
date: 2026-11-25
---

# A repository declares which checks block a merge

## Context and Problem Statement

A repository's checks are spread over several places: the stack packs list tools, standards cards carry rubrics, `project.yaml` names `verify` and `tests`, and the review policy decides when a person looks (design sections 8, 11, 17). Nothing says in one place which checks and review layers must pass before a PR merges, which only warn, and how that depends on the paths touched or the blast radius. The owner wants to tune linters, code review, formal and informal checks, and unit and integration tests per repository. Tools such as CodeRabbit (check modes `off`, `warning`, `error`), Danger (`warn`, `fail`), reviewdog (`-fail-level`) and Trunk (hold the line, a logged skip) all have such a switch. Entire's agent reviewers are set in its web UI and, in the pages we read, are not described as merge gates (research of 2026-11-25).

## Considered Options

- A `gates` file in the repository that maps each check to `block`, `warn` or `off`, with `when` rules, a floor that cannot be lowered, and owner approval for any weakening.
- Extend each pack and standards card with its own severity switch.
- A settings screen in the control panel, stored in the book.

## Decision Outcome

Chosen option: "a `gates` file in the repository", because the project already keeps its other rules there (`blast-radius.yaml`), history and review come from git, the file reads the same for a tool, a rubric and a review layer, and no new service or table is needed. Without the file every check blocks, so existing projects behave as before.

### Consequences

- Good: one place to answer "what must pass here", and `nightshift check --plan` shows it for a diff without running anything.
- Good: weakening a gate is a normal critical change: the diff shows it and the owner's Approve is required.
- Bad: one more file beside packs, standards and blast radius; it stays small because it only links existing names.
- Bad: a `warn` gate can hide a real defect; the summary lists every warning and a `warn` check never applies to the factory floor.

## Pros and Cons of the Options

### Severity switch on each pack and standards card

- Good: no new file.
- Bad: the answer is spread over many files and cannot depend on paths or blast radius without repeating the rules.

### Settings screen in the control panel

- Good: easy to change by hand.
- Bad: changes leave git history and PR review, which the factory relies on for every protection; it adds a new writer to the rules.
