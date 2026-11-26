# Phase 6 — Verifier, preview, production

Owned by the session working on this phase; another session asks it and does not edit this file. Rules and limits: `docs/plan.md`.

Done when: a Slack task ends with an evidence card and a torn-down preview on `compose-local`; Merge deploys; a failed smoke rolls back on its own.

## Status

- Last done (2026-11-26): the `Deployer` interface and twin (P1), deploy steps, preview gate, release and automatic rollback in the train (P2), `compose-local` with its recipe lint, run by the `deployer` service under profile `deploy` (P4; #278, #291, #292, #298, #312). Stubs and local git only, none against a real Docker engine.
- Next: P3 `script` and `external`, P5 live acceptance, P6 revert PR and preview teardown, the T4 deploy scenario.
- Mac, when back: `docker compose --profile deploy up -d --build` (the alpine `docker-cli` packages are unverified), a preview on 127.0.0.1, the token readable by the deployer, `deploy/init-secrets.sh` rerun.

## Scope and open work

Built: Verifier role, verdict and the review leg on `fake`; P1 (`integration.Deployer`, the `deploy/fake` twin, `compose-local` in `capabilities` of `schemas/project.json`); P2 steps `up`, `smoke`, `release`, `rollback` through `book.deploy_steps` (`internal/deploy`), the preview gate in the conductor, release through the train with the automatic rollback, on the twin (the full T4 scenario file is open). Open, in order (design section 17 "Deploy targets", ADR 0014): P3 adapters `script` and `external`; P4 built on stubs, its T6 escapes on a real engine open; P5 live acceptance through playwright-cli on the preview, hidden scenarios, #deployments card; P6 revert PR and frozen train on rollback, preview limits and teardown, the polygon run in `deploy/upgrade.sh` (its live check and tag switch are built). `compose-ssh`, `kubernetes` and `load` wait for their first tenant. Done: on `compose-local` a Slack task ends with an evidence card and a torn-down preview; Merge deploys; a failed smoke rolls back on its own. Latent: after `revert_merged` the old merged `pr_number` would end the task at once (outside-merge watch); clear it at the `rolled_back` to `building` move.
