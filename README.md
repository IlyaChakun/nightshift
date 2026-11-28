# Nightshift

Nightshift runs coding tasks from chat or the terminal. The tenant config selects which agents plan,
build and review each change. The review policy controls when a person must approve a pull request.
Docker Compose starts the configured instance from the `deploy/` directory.

## Where it is going

Nightshift turns an idea into a tested change that a person releases. You decide twice: you
approve the spec before any code is written, and you approve the release after the checks and
the preview. Agents do the work between those points behind deterministic gates.

```mermaid
flowchart TB
  idea(["Idea in chat or the terminal"])
  lead["Lead asks questions<br/>and writes a spec"]
  spec{{"You: approve the spec"}}
  build["Builder writes the code<br/>and opens a pull request"]
  checks["Checks, Verifier review,<br/>preview and acceptance"]
  ship{{"You: approve the release"}}
  prod(["Release and smoke check"])
  mem[("Memory")]
  learn["Self-learning"]

  idea -- task --> lead -- spec --> spec
  spec -- approved --> build -- pull request --> checks
  spec -. edit .-> lead
  checks -. findings, fix .-> build
  checks -- green, with evidence --> ship
  ship -- merge --> prod
  ship -. request changes .-> build
  mem == context ==> lead
  mem == context ==> build
  checks -. findings .-> learn
  prod -. smoke results .-> learn
  learn -- improvement pull request,<br/>same checks --> checks
```

Solid arrows move work forward, dotted arrows send work back or carry results to self-learning,
and thick arrows bring context. An improvement is a pull request like any other. The full flow, with every return and escalation, is design section 7
([`docs/design/02-control.md`](docs/design/02-control.md)); the planes behind it are the
architecture map in [`docs/design.md`](docs/design.md#26-vision-and-plane-map). What memory
holds and who may change it: [`docs/design/04-knowledge.md`](docs/design/04-knowledge.md#28-knowledge-scopes-factory-and-tenant-memory).

**Next milestone: the MVP** ([validation plan](docs/plan/mvp-validation/README.md)).

- **Works today:** the path above from the terminal, through preview and acceptance, to a
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
