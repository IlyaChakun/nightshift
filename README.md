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

The target factory, layer by layer from left to right. Solid boxes run today; dashed boxes are
planned (phase in [`docs/plan.md`](docs/plan.md)) or an owner idea not yet in the plan. Lessons
from layer 5 come back as pull requests through the same gates.

```mermaid
flowchart LR
  subgraph L1["1 · People"]
    direction TB
    owner(["Owner, teams"])
    doors["Slack · terminal<br/>tracker · MCP door"]
    webui["web UI"]:::plan
  end
  subgraph L2["2 · Control"]
    direction TB
    lead["Lead: clarify, spec"]
    flow["gates · merge train<br/>release on a button"]
    obj["objectives → tasks"]:::plan
  end
  subgraph L3["3 · Execution"]
    direction TB
    roles["Builder · Verifier"]
    hz["models: claude · codex"]
    hzp["copilot · api<br/>open models"]:::plan
    gw["MCP gateway"]:::idea
  end
  subgraph L4["4 · Assurance, knowledge"]
    direction TB
    asr["checks · policy<br/>rubric · review"]
    kno["search · memory"]
    knp["hybrid search<br/>product intelligence"]:::plan
  end
  subgraph L5["5 · Data, learning"]
    direction TB
    book[("Postgres<br/>book · queue")]
    obs["digest · health"]
    learn["learn: lessons"]
    evo["evals · autonomy ladder"]:::plan
  end
  subgraph T["0 · Tenants, packs"]
    direction TB
    solo["solo tenant"]
    team["team tenant"]:::plan
    packs["stacks: go · python<br/>ts · js · java"]
    skills["role skills"]
  end
  owner ~~~ doors ~~~ webui
  lead ~~~ flow ~~~ obj
  roles ~~~ hz ~~~ hzp ~~~ gw
  asr ~~~ kno ~~~ knp
  book ~~~ obs ~~~ learn ~~~ evo
  solo ~~~ team ~~~ packs ~~~ skills
  T ~~~ L1 --> L2 --> L3 --> L4 --> L5
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
