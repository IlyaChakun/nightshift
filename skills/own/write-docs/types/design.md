# Design doc or spec

A design doc says what a system is, why it is built that way and what was rejected. A spec is a mini design for one feature or task.

Budget: whole system 5,000 words typical, 10,000 max; spec 800 typical, 1,500 max. Source: Google's design docs run "10-20ish pages" for a larger project and "1-3 page" for a mini design doc ([Design Docs at Google](https://www.industrialempathy.com/posts/design-docs-at-google/), read 2026-11-26).

## Sections

1. **Context and scope**: the problem and the boundary, in a few sentences.
2. **Goals and non-goals**: lists; a non-goal is something a reader could expect and will not get.
3. **Design**: the structure, the flow, the data. A diagram where flow is the point.
4. **Alternatives considered**: each with the reason it lost. One line each; a long comparison goes to an ADR.
5. **Cross-cutting concerns**: security, cost, operations, testing, only where they change the design.

## Keep it short

- Decisions live in ADRs; the design names the outcome in one line and links the ADR.
- Steps for a person go to the runbook, schemas and configs to their files: link, do not copy.
- No history: the design reads as current; past versions are in git.
- Over the max: split by subsystem into linked files, each with its own summary.
