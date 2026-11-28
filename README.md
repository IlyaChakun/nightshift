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
  you(["Owner<br/>chat, terminal, MCP"])
  subgraph exp["Experience: doors, digest"]
    mcpdoor["mcpdoor"]
  end
  subgraph ctl["Control: task flow"]
    direction TB
    c1["doors · chatapi · trackerapi"]
    c2["dispatch · flow · routing · tracking"]
    c3["leadreply · train · deploy · trustedgit"]
    c4["jobserver · outbox · periodic · settings<br/>codehost · mask"]
  end
  subgraph exe["Execution: worker, agents"]
    direction TB
    e1["runner · harness · jobsapi · deployapi"]
    e2["gitops · skills · toolchain"]
  end
  subgraph asr["Assurance: gates, review"]
    a1["check · policy · rubric<br/>standards · deps"]
  end
  subgraph kno["Knowledge: search, memory"]
    k1["search · memory"]
  end
  subgraph obs["Observability: read models, health"]
    o1["summary"]
  end
  subgraph data["Data foundation"]
    d1[("book<br/>Postgres, River queue")]
  end
  subgraph evo["Evolution loop"]
    v1["learn"]
  end
  subgraph cus["Customization: packs, tenants"]
    u1["config · onboard · stack"]
  end
  adp["Adapters<br/>github · chat · tracker · deploy"]
  you --> exp --> ctl
  ctl <--> exe
  ctl --> asr
  ctl --> kno
  exp & ctl -. read .-> obs
  ctl & asr & kno --> data
  data -. findings .-> evo
  evo -. pull requests .-> asr
  cus -. config .-> ctl & exe
  ctl <--> adp
```

Each box is a plane folder `internal/<plane>/` and lists its packages. Security is cross-cutting:
the worker runs agents with no tokens and no access to the book, and egress goes through a proxy.

The target factory, layer by layer. Solid boxes run today; dashed boxes are planned (phase in
[`docs/plan.md`](docs/plan.md)) or an owner idea not yet in the plan.

```mermaid
flowchart TB
  subgraph L1["1 · People and doors"]
    direction LR
    owner(["Owner, teams"]) --- doors["Slack · terminal · tracker<br/>remote MCP door"]
    webui["web UI"]:::plan
  end
  subgraph L2["2 · Control: one task flow"]
    direction LR
    lead["Lead: clarify, spec"] --> flow["dispatch · gates · merge train<br/>release on a person's button"]
    obj["objectives → tasks"]:::plan
  end
  subgraph L3["3 · Execution: worker, any model"]
    direction LR
    roles["roles: Lead · Builder · Verifier"] --> hz["harnesses: claude · codex · fake"]
    hzp["copilot · api · open models"]:::plan
    gw["MCP gateway for roles,<br/>tenants and projects"]:::idea
  end
  subgraph L4["4 · Assurance and knowledge"]
    direction LR
    asr["checks · policy · rubric · review"]
    kno["search · memory"]
    knp["hybrid search · product intelligence"]:::plan
  end
  subgraph L5["5 · Customization: packs per tenant"]
    direction LR
    packs["stack packs + standards<br/>go · python · ts · js · java"]
    skills["role skills: grilling · to-spec<br/>web-dev · mobile-dev · …"]
    axis["config: core → packs → instance<br/>→ tenant → project → task"]
  end
  subgraph L6["6 · Tenants"]
    direction LR
    solo["solo tenant<br/>personal projects"]
    team["team tenant, own instance"]:::plan
    multi["multi-repo tasks"]:::plan
  end
  subgraph L7["7 · Data, observability, self-improvement"]
    direction LR
    book[("Postgres: book, queue, search")] --> obs["read models · digest · health"]
    book -. findings .-> learn["learn: lessons, fix ladder"]
    evo["evals polygon · autonomy ladder<br/>cross-tenant promotion"]:::plan
  end
  L1 --> L2 --> L3 --> L4
  L5 -. configures .-> L2 & L3 & L4
  L6 -. selects packs .-> L5
  L4 --> L7
  L7 -. improvement PRs through the same gates .-> L2
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
