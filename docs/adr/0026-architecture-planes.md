---
status: accepted
date: 2026-11-27
---

# Architecture planes and the layered design

## Context and Problem Statement

`docs/design.md` grew to 23,666 words, above its 10,000-word budget, and every agent had to read the whole file to find one subsystem. The packages under `internal/` show no structure above the package, so nothing stops a door from calling the database directly or the core from importing an adapter. The owner asked on 2026-11-27 for one structure that the documents, the packages and the tests share, so that a new memory layer, door or tool has an obvious place.

## Considered Options

- Planes: Experience, Control, Execution, Knowledge, Governance and assurance, Data foundation, with an Evolution loop, Observability (a plane since the owner's decision of 2026-11-27) and cross-cutting Security and Customization
- Five horizontal layers stacked from doors down to storage
- Flat packages with the design split by topic only

## Decision Outcome

Chosen option: "Planes", because each plane owns one kind of responsibility and its data, which matches how work moves through the factory: a door records an event, control decides, execution runs, assurance judges, and everything lands in the book.

Rules:

- **Docs.** `docs/design.md` is the overview: the vision, the plane map and the decisions that span planes. Each plane has one file `docs/design/NN-<plane>.md`. Section numbers stay unique across files, so "design section N" keeps pointing to one place.
- **State.** Each plane owns its state and the rules for changing it. Others use its authorized commands, scoped queries or an explicit persistence contract. A shared physical store does not imply a shared right to write. The worker has no access to the book. A read model is never a source of authorization. Transactions that the book and the task flow need (a result, its state change and its outbox row) stay atomic.
- **Dependencies.** A narrow interface owned by the consumer stands at each boundary of behavior, authority or replaceable infrastructure; an interface is not added only because a call crosses planes. Stable value types, protocol messages and pure functions may be imported directly where the import direction allows. `cmd/nightshift` and any other entrypoint are composition roots that wire concrete adapters. There is no central interfaces package. Imports form an acyclic graph checked statically; runtime rights (who may write, merge or release) are checked separately by integration and negative tests. The core (`internal/` outside `internal/adapters/`) should not import an adapter; today `dispatch` and `deployer` do, and the dependency audit decides how they change.
- **Adapters** belong to the plane they serve; there is no separate integrations plane.
- **Evolution** is a loop, not a floor: it reads findings and metrics from the book and changes the factory only through pull requests that pass the same gates.
- **Customization** resolves along one axis: core, packs, instance, tenant, project, task.
- **Observability** is a plane of its own (owner, 2026-11-27): signals, metric definitions, read models and health. It reads state and decides nothing; Control and Experience may use it.
- **Folder names** (owner, 2026-11-27): `internal/<plane>/<package>` with `experience`, `control`, `execution`, `knowledge`, `assurance`, `evolution`, `data`, `customization`, `observability`, `testing`. Composition roots are `cmd/nightshift` and the adapter selector `internal/adapters`; a service entry such as `internal/deployer` may become one (stream `docs/plan/architecture/`, A6).
- **Knowledge scopes** (owner's review of 2026-11-27): factory, instance, tenant, project and task are scopes, an axis apart from the memory type. Tenant knowledge reaches the factory scope or another tenant only through a person's reviewed PR of a tenant-neutral artifact. Knowledge owns meaning, Data foundation the records, Assurance the evidence (design section 28).
- **Capabilities** are a composition map (design section 29), not a plane: no package folder, runtime or role of their own.
- **Planned items** in the plane map are marked as planned or direction, with a link to the research or plan item. Objective management and product intelligence are direction (design section 26).
- **Order of work.** The plane map comes first, then this doc split, then a dependency audit of today's imports, then architecture tests that enforce these rules, and only then package moves, one plane at a time.

### Consequences

- Good: an agent reads the overview and one plane file instead of the whole design.
- Good: a new tool has a home and a rule: it implements an interface owned by its consumer.
- Good: write rights stay with the owner of the state even though all state shares one Postgres.
- Bad: one design change can touch two files, and the term and section checks read several files.
- Bad: today's packages do not all fit one plane (`dispatch` runs queue work for several planes) until the dependency audit and refactoring.

## Pros and Cons of the Options

### Five horizontal layers

- Good: simple to draw; a strict top-to-bottom rule.
- Bad: forces a central contracts layer and puts the task state machine next to storage, which the code does not do.

### Flat packages, design split by topic

- Good: no code change.
- Bad: nothing in the code shows or checks the structure, so the docs and the packages drift apart again.
