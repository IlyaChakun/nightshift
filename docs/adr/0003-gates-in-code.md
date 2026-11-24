---
status: accepted
date: 2026-11-24
---

# Enforce gates and side effects in factory code

## Context and Problem Statement

Agents can be wrong or manipulated through prompt injection. A merge, a release, a chat message or a tracker change must never depend on an agent following instructions. Where do the gates live?

## Considered Options

- Gates and all side effects in `nightshift` Go code; agents return JSON validated against `schemas/`
- Gates in GitHub only (branch protection, rulesets) with agents holding tokens
- Agents with side-effect tools guarded by prompts and skills

## Decision Outcome

Chosen option: "gates and side effects in `nightshift` code", because only code can guarantee that no merge happens without checks and the required Approve, and the home plan cannot rely on rulesets for private repositories.

### Consequences

- Good: the worker holds no GitHub, chat or tracker tokens; a compromised agent cannot merge or post.
- Good: decisions are bound to SHAs and recorded in the append-only book; T2 properties test the invariants.
- Bad: `nightshift` must implement push, PR, merge train and outbox itself.
- Neutral: in the `team` profile repository rules stay the gate and the factory only hands over PRs.

## Pros and Cons of the Options

### GitHub rules only

- Good: no custom code.
- Bad: rulesets for private repositories need a paid plan, and agents would hold tokens.

### Prompt-guarded tools

- Good: simplest to build.
- Bad: "on trust"; one injection or mistake can merge or deploy.
