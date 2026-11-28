# Nightshift

Nightshift runs coding tasks from chat or the terminal. The tenant config selects which agents plan,
build and review each change. The review policy controls when a person must approve a pull request.
Docker Compose starts the configured instance from the `deploy/` directory.

## Where it is going

Nightshift turns an idea into a tested change that a person releases. You decide twice: you
approve the spec before any code is written, and you approve the release after the checks and
the preview. Agents do the work between those points behind deterministic gates. The map below shows the
whole factory by layer.

```mermaid
flowchart TB
  subgraph P["People and doors"]
    direction LR
    who(["Owner · live<br/>Team · planned: phase 8"])
    doors["Slack · CLI: live<br/>tracker · MCP door: pending live check"]
    appr{{"Human approval right<br/>spec Approve · release Approve or Merge<br/>as the tenant's review policy says"}}
  end

  subgraph C["Tenants and customization"]
    direction LR
    src["Sources<br/>instance.yaml + providers.yaml: trust, egress, providers<br/>tenants/name/tenant.yaml sets behaviour: rules, people, review, budgets, roles, chat language<br/>project .nightshift/: stack, instructions, skills, verify<br/>factory packs: stacks, standards, roles, skills"]
    res["Config resolution<br/>core → packs → instance → tenant → project → task"]
    out["Run settings<br/>tenant rules and rights · chat language for people<br/>models and effort per role · project instructions and skills<br/>stack pack and standards chosen by the project's stack<br/>artifacts always in English"]
    src --> res --> out
  end

  subgraph K["Control: the centre · live"]
    direction LR
    ctl["task state machine · dispatcher · merge train"]
    gh["GitHub adapter: branches, PRs, merge"]
    dep["deployer: preview, release, rollback"]
  end

  jobs[/"Jobs API: the worker's only way to factory state"/]

  subgraph W["Worker · isolated"]
    direction LR
    roles["Lead · Builder · Verifier<br/>no tokens, no book access"]
    hz["harnesses: claude live · codex pending live check<br/>copilot, api planned"]
    prov["providers and models<br/>subscription · API keys · open models"]
  end

  subgraph G["Governance and assurance"]
    asr["checks L1–L4 · policy · rubric · Verifier verdicts"]
  end

  subgraph M["Knowledge"]
    direction LR
    scopes["Scopes and their homes<br/>factory rules: nightshift repo, by PR<br/>building Nightshift: LEARNINGS.md, .agents/skills, never shipped<br/>instance: instance.yaml, by the operator<br/>tenant: the book keeps knowledge and results:<br/>memory page, findings, task outcomes; written only by nightshift<br/>project: AGENTS.md, LEARNINGS.md, ADRs, by PR in its repo<br/>task: harness session, handoff, transcripts"]
    search["Search<br/>full text: pending live check<br/>vectors + RRF: planned, phase 9<br/>pgvector column and RRF code exist;<br/>embedder, backfill, quality eval to come"]
    sel["Allowed context<br/>filtered by role and tenant"]
    scopes --> search --> sel
  end

  subgraph D["Data and observability"]
    direction LR
    book[("Postgres: book · River queue · outbox · documents")]
    obs["Observability: read models · health<br/>morning digest to Slack"]
  end

  subgraph L["Self-improvement"]
    direction LR
    l1["results and findings"] --> l2["clustering · built"] --> l3["fix proposal<br/>planned: phase 7"] --> l4["PR + same checks<br/>planned: phase 7"] --> l5["updated rule, test or skill"]
  end

  P -- commands, approvals --> K
  C -- config, tenant rules --> K
  K <-- jobs with run settings / results --> jobs
  jobs <-- jobs / results --> W
  M -- allowed context, RAG --> jobs
  K <-- verdict requests / verdicts --> G
  K -- state --> D
  gw["MCP gateway · idea<br/>shared tools for roles,<br/>tenants and projects"] -. tools .-> W
  G <-- findings / proposals as PRs --> L
  L -. tenant lesson stays in its tenant;<br/>factory gets only a tenant-neutral,<br/>human-reviewed PR .-> M
```

Each box says its status: **live** runs and passed a live check; **pending live check** is merged
and tested on fakes, with its check queued in [`docs/plan/live-checks.md`](docs/plan/live-checks.md);
**planned: phase N** is an item in [`docs/plan.md`](docs/plan.md); **idea** is not in the plan yet.
Arrow labels name what moves; dotted arrows are not built yet or are rules rather than calls.
Plane map and packages: [design section 26](docs/design.md#26-vision-and-plane-map) and
[ADR 0026](docs/adr/0026-architecture-planes.md). Memory scopes:
[section 28](docs/design/04-knowledge.md#28-knowledge-scopes-factory-and-tenant-memory). One task
from idea to release, with every return and escalation: [section 7](docs/design/02-control.md).

**Next milestone: the MVP** ([validation plan](docs/plan/mvp-validation/README.md)).

- **Works today:** the path from an idea in the terminal, through preview and acceptance, to a
  release, on the owner's machine (2026-11-28); isolation of the preview and recovery after a
  worker dies passed there too.
- **Being checked on a real stack:** the same path started from Slack with an evidence card and
  your approval, an automatic rollback with a revert pull request, and the morning digest.
- **Direction:** self-learning that turns repeated findings into rules and skills (phase 7),
  team tenants (phase 8), search by meaning (phase 9) and tasks across several repositories
  (phase 10); see [`docs/plan.md`](docs/plan.md).

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
