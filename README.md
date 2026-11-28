# Nightshift

Nightshift runs coding tasks from chat or the terminal. The tenant config selects which agents plan,
build and review each change. The review policy controls when a person must approve a pull request.
Docker Compose starts the configured instance from the `deploy/` directory.

## Where it is going

Nightshift is a governed, knowledge-driven, self-improving software factory: it turns an authorized
idea into a reviewed, tested change that a person releases with one button. It is built in planes,
each owning one responsibility and its state
([ADR 0026](docs/adr/0026-architecture-planes.md)):

```mermaid
flowchart LR
  subgraph CUS["Customization"]
    direction TB
    u1["config · onboard · stack"]
  end
  subgraph EXP["Experience"]
    direction TB
    x1["mcpdoor"]
    x2["adapters: chat · tracker"]
  end
  subgraph CTL["Control"]
    direction TB
    c1["doors · chatapi · trackerapi"]
    c2["dispatch · flow · routing · tracking"]
    c3["leadreply · train · deploy · trustedgit"]
    c4["jobserver · outbox · periodic<br/>settings · codehost · mask"]
    c5["adapters: github · deploy"]
  end
  subgraph EXE["Execution"]
    direction TB
    e1["runner · harness<br/>jobsapi · deployapi"]
    e2["gitops · skills · toolchain"]
  end
  subgraph ASK["Assurance, knowledge"]
    direction TB
    a1["check · policy · rubric<br/>standards · deps"]
    k1["search · memory"]
  end
  subgraph DAT["Data, health, learning"]
    direction TB
    d1[("book<br/>Postgres, River queue")]
    o1["summary"]
    v1["learn"]
  end
  x1 ~~~ x2
  c1 ~~~ c2 ~~~ c3 ~~~ c4 ~~~ c5
  e1 ~~~ e2
  a1 ~~~ k1
  d1 ~~~ o1 ~~~ v1
  CUS ~~~ EXP --> CTL --> EXE --> ASK --> DAT
```

Each column is one or more plane folders `internal/<plane>/` with their packages; a task moves
from left to right, and Customization configures every column. Security is cross-cutting:
the worker runs agents with no tokens and no access to the book, and egress goes through a proxy.

The target factory. Solid boxes run today; dashed boxes are planned (phase in
[`docs/plan.md`](docs/plan.md)) or an owner idea not yet in the plan. Tenant config lives in
files and is loaded at start; memory has four scopes, each with its own home and writer
([design section 28](docs/design/04-knowledge.md)); self-learning turns findings into rules
([design section 10](docs/design/06-evolution.md)).

```mermaid
flowchart LR
  subgraph CFG["Tenants: config files"]
    i1["instance.yaml<br/>trust domain, models, egress"]
    i2["tenants/name/tenant.yaml<br/>people, chat, tracker, budgets"]
    i3["secrets per tenant<br/>not in git"]
    i4["project repo .nightshift/<br/>stack, skills, standards"]
    i5["team tenant, multi-repo"]:::plan
  end
  subgraph PPL["People"]
    p1(["Owner, teams"])
    p2["Slack · terminal<br/>tracker · MCP door"]
    p3["web UI"]:::plan
  end
  subgraph CTL["Control"]
    c1["Lead: clarify, spec"]
    c2["gates · merge train<br/>release on a button"]
    c3["objectives → tasks"]:::plan
  end
  subgraph EXE["Execution"]
    e1["Builder · Verifier"]
    e2["models: claude · codex"]
    e3["copilot · api<br/>open models"]:::plan
    e4["MCP gateway"]:::idea
  end
  subgraph ASR["Assurance"]
    a1["checks · policy<br/>rubric · review"]
    a2["every miss → a finding"]
  end
  subgraph MEM["Memory by scope"]
    m1["factory: roles, skills, standards<br/>nightshift repo"]
    m2[("tenant: memory page, findings,<br/>task outcomes, notes<br/>Postgres book, per tenant")]
    m3["project: AGENTS.md, LEARNINGS.md,<br/>ADRs, PRDs · project repo"]
    m4["task: harness session,<br/>transcripts"]
    m5["search: full text"]
    m6["RAG: vector search, pgvector<br/>finds past cases by meaning"]:::plan
  end
  subgraph LRN["Self-learning"]
    l1["findings clustered<br/>into recurring problems"]
    l2["nightly learn: root cause"]:::plan
    l3["fix ladder: tool rule, test,<br/>skill, LEARNINGS"]:::plan
    l4["trial → active rule<br/>checked on the eval polygon"]:::plan
  end
  i1 ~~~ i2 ~~~ i3 ~~~ i4 ~~~ i5
  p1 ~~~ p2 ~~~ p3
  c1 ~~~ c2 ~~~ c3
  e1 ~~~ e2 ~~~ e3 ~~~ e4
  a1 ~~~ a2
  m1 ~~~ m2 ~~~ m3 ~~~ m4 ~~~ m5 ~~~ m6
  l1 ~~~ l2 ~~~ l3 ~~~ l4
  CFG -- loads --> CTL
  PPL --> CTL --> EXE --> ASR
  CTL <-- context --> MEM
  ASR -- findings --> LRN
  LRN -. PRs through the same gates .-> MEM
  classDef plan stroke-dasharray: 5 5
  classDef idea stroke-dasharray: 2 4
```

The design starts at [`docs/design.md`](docs/design.md): the vision, the plane map and one file per
plane in [`docs/design/`](docs/design/).

## Where things are

- Plan and progress: [`docs/plan.md`](docs/plan.md) (one folder per phase in `docs/plan/`), [`docs/status.md`](docs/status.md)
- Rules for agents and contributors: [`AGENTS.md`](AGENTS.md)
- Code: one binary in [`cmd/nightshift/`](cmd/nightshift/); the core in [`internal/`](internal/AGENTS.md), one folder per plane (`internal/control/`, `internal/execution/` and so on). An architecture test in [`internal/testing/archtest/`](internal/testing/archtest/) runs with `make test` and fails when a package crosses a plane boundary
- Skills for agents that work on this repo (add a migration, change a schema or the design, fix CI, finish a task, let the factory work on itself): [`.agents/skills/`](.agents/skills); Claude Code finds them through `.claude/skills`, which links there

## Run it

Follow [`docs/setup.md`](docs/setup.md): secrets, model credentials, Slack, a sandbox
repository, the GitHub App, config, start and a first task. An agent can lead you through it
with the [`set-up-instance`](.agents/skills/set-up-instance/SKILL.md) skill.

Operations (backups, restore, rotation): [`docs/runbook.md`](docs/runbook.md).

## Test a checkout

From the repository root, run `mise install --locked`, then `mise exec -- make verify`.
This runs the format and lint checks, Go tests, security scanners and repository checks.
For the Go tests alone, run `mise exec -- make test`. Start Docker before running either
command for Postgres integration coverage. With no working Docker provider or configured
test Postgres, the database tests can skip. The test helper also skips them in Go's short mode.

For the separate backup, restore and isolation exercise, run `mise exec -- make drill`
after initializing the local files in setup section 1. The drill recreates the disposable
`nightshift-drill` Compose project and its volumes. [Drill procedure](docs/runbook.md#drill-do-the-backups-and-the-isolation-really-work)
