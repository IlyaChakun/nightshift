---
status: accepted
date: 2026-11-26
---

# Keep one writer for Verifier findings

## Context and Problem Statement

Jobs API validates each finding and records it with its job and location. The conductor also inserted a second record for each returned Verifier finding, so Builder could receive duplicate feedback with incomplete fields. Source: [audit #337](https://github.com/IlyaChakun/nightshift/pull/337).

## Considered Options

- Write findings at Jobs API completion and let the conductor act on the stored result.
- Move all Verifier finding validation and persistence to the conductor.

## Decision Outcome

Jobs API remains the writer. New Verifier findings use the same validated fields and provenance as other job output. Line numbers use the book's integer range. A line outside that range is omitted and the finding stays available. The conductor transitions the task and records risk in its own transaction. Retrying the transition keeps the stored findings available. Existing records remain readable.

### Confirmation

`TestReturnedVerifierFindingHasOneRow` completes a real Jobs API request and compares the finding before and after dispatch. The Verifier return and replay scenarios check delivery and retry.
