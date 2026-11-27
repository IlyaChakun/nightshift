---
status: accepted
date: 2026-11-27
---

# The conductor stays one state machine; self-contained components leave dispatch

## Context and Problem Statement

`internal/control/dispatch` holds the `Conductor`: about 110 methods in 25 files, one file per stage (lead, build, gate, verify, review wait, dependencies, revert, preview teardown). Plan item A6 asks to split `dispatch` by responsibility, not by file count. The stages call each other on the same tick: red CI and a returned review start Builder at once (`checks` and `gate` call `building`), `queued` starts Builder, `gate` and `acceptance` start the Verifier. All stages share the same helpers (`move`, `budget`, `modelOf`, `appendEvent`, `postCard`, `ThreadOf`, `mirror`) and write one task row through `flow.Mover`. Some parts of the package do not use the `Conductor` at all and already have their own dependencies: the provider router, the tracker projection, and the checks of Lead's reply and the Verifier's verdict.

## Considered Options

- One `Conductor` for the task state machine with one file per stage; components with their own dependencies move to packages of their own
- Stage structs with narrow dependencies, each in its own package, a shared kernel of helpers, and a `Conductor` that delegates to them
- Stage structs inside `dispatch`, embedded in the `Conductor`

## Decision Outcome

Chosen option: "One `Conductor` with stage files", because a stage is not a boundary of authority or of replaceable infrastructure (ADR 0026 puts an interface only at such a boundary): every stage writes the same task state under the same rules, in the same transactions. Stage packages would need a kernel that exports about fifteen helpers and a "next stage" interface or a second pass of the ticker, and the second pass changes behavior: Builder would start one tick later after red CI.

What leaves `dispatch`, one package per PR, behavior unchanged:

- `control/routing`: the provider router (`Router`, `Choice`, limits, `Spend`, `Traceparent`); the conductor's `requeueAfterLimit` stays.
- `control/tracking`: the tracker projection (`Projector`, `TrackerSender`, `TrackerTarget`).
- The checks of Lead's reply and the Verifier's verdict (`CheckLeadReply`, `CheckExpect`, `CheckCorrection`, `Decide`): pure functions that the evals call too; the card texts stay with the conductor.

The stage files, the `Ticker` and the seams that `control/doors` calls stay in `dispatch`. A seam is exported only when a door must apply the conductor's own rule; each one is reviewed in the last A6 PR.

### Consequences

- Good: the state machine reads in one place, and the same-tick hand-offs stay as they are.
- Good: each component that leaves has its own small set of dependencies and its own tests.
- Bad: `dispatch` stays the largest package; a stage that grows its own dependencies (a new door, a new store) has to leave the same way, through its own ADR or plan item.

### Confirmation

`internal/testing/archtest` lists each new package in its plane; `go list -deps` shows that none of them imports `dispatch`.

## Pros and Cons of the Options

### Stage structs in their own packages

- Good: each stage names the dependencies it uses.
- Bad: a shared kernel package with many exported helpers, which ADR 0026 rules out as a central package.
- Bad: cross-stage calls need an interface or a second tick, which changes when Builder starts.

### Stage structs inside dispatch

- Good: narrower fields per stage without new packages.
- Bad: the stages still call each other, so each struct needs a pointer back to the others; the split is cosmetic and every test rig changes.
