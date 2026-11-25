# Nightshift

Nightshift is an autonomous software factory. You describe a task in chat; the Lead, Builder and
Verifier agents produce the spec, the code and the evidence behind deterministic gates, and a change
reaches production only when a human presses the button. One `docker compose up` starts it.

- Design: [`docs/design.md`](docs/design.md)
- Plan and progress: [`docs/plan.md`](docs/plan.md), [`docs/status.md`](docs/status.md)
- Rules for agents and contributors: [`AGENTS.md`](AGENTS.md)
- Skills for agents that work on this repo (add a migration, change a schema or the design, fix CI, finish a task): [`.agents/skills/`](.agents/skills); Claude Code finds them through `.claude/skills`, which links there

## Run it

Follow [`docs/setup.md`](docs/setup.md): secrets, model credentials, Slack, a sandbox
repository, the GitHub App, config, start and a first task. An agent can lead you through it
with the [`set-up-instance`](.agents/skills/set-up-instance/SKILL.md) skill.

Operations (backups, restore, rotation): [`docs/runbook.md`](docs/runbook.md).

Status: [`docs/status.md`](docs/status.md).
