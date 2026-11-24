# Nightshift

Nightshift is an autonomous software factory. You describe a task in chat; the Lead, Builder and
Verifier agents produce the spec, the code and the evidence behind deterministic gates, and a change
reaches production only when a human presses the button. One `docker compose up` starts it.

- Design: [`docs/design.md`](docs/design.md)
- Plan and progress: [`docs/plan.md`](docs/plan.md), [`docs/status.md`](docs/status.md)
- Rules for agents and contributors: [`AGENTS.md`](AGENTS.md)

## Run it

Phase 1 runs the book, the chat bot and backups on one machine with Docker:

1. Secrets: follow [`docs/secrets.md`](docs/secrets.md). The script generates the
   passwords; you paste two chat tokens.
2. Config: copy [`testdata/instances/personal/`](testdata/instances/personal) to `deploy/config/`
   and add `deploy/config/tenants/<name>/tenant.yaml`, starting from
   [`testdata/tenants/example-solo/tenant.yaml`](testdata/tenants/example-solo/tenant.yaml).
3. `cd deploy && docker compose up -d --build`.

Operations (backups, restore, rotation): [`docs/runbook.md`](docs/runbook.md).

Status: phase 1 (book and core); see `docs/status.md`.
