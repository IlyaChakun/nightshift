---
status: accepted
date: 2026-11-25
---

# Research before building: skills, a tool and a finding

## Context and Problem Statement

Agents tend to write code for a need that a standard library, a dependency we already have or a good open-source library covers, or to add a library without weighing it. A new dependency is already blast radius `critical`, but the owner sees only that it exists, not the options behind it. The owner wants the "write or take" question answered every time, by the agents, in a repeatable form, before coding starts.

## Considered Options

- Two own skills (`research-options`, `build-vs-buy`), a deterministic `nightshift deps check`, a stop for Builder and an L2 finding when a record is missing
- One line in Builder's instructions ("prefer existing code and libraries")
- A separate research role with open web access
- A new `research` field in the spec schema, checked by the gate

## Decision Outcome

Chosen option: "skills, a tool and a finding", because it triggers at the decision point, keeps every checkable fact (licence, advisories, age) in code, and adds no role, service or schema field. The record is an ADR, so doc-lint already demands at least two options and the owner reads it where decisions already live.

### Consequences

- Good: no new egress; deps.dev, OSV and the registries are already allowed (design section 16).
- Good: a missing record is a finding, not a reviewer's memory.
- Bad: an ADR per new dependency is ceremony for tiny libraries; the size limit and a short ADR form keep it light.
- Bad: the L2 finding is a new gate; the owner approved it in chat on 2026-11-25.

## Pros and Cons of the Options

### One line in Builder's instructions

- Good: free.
- Bad: nothing triggers it and nothing records the choice.

### A separate research role

- Good: a clean context for research.
- Bad: web access next to code breaks the isolation of section 16; a skill and a web research job without code do the same.

### A `research` field in the spec schema

- Good: structured and checkable.
- Bad: a schema change needing owner review, for something an ADR already holds.
