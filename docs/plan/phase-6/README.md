# Phase 6 — Verifier, preview, production

Owned by the session working on this phase; another session asks it and does not edit this file. Rules and limits: `docs/plan.md`.

Done when: a Slack task ends with an evidence card and a torn-down preview on `compose-local`; Merge deploys; a failed smoke rolls back on its own.

## Status

- Audit #332: [A01r](A01r-resume-evidence.md) is built with Postgres and a forge twin. Its live restart check is pending. A01b follows for the host head and final rebased revision.

- Last done (2026-11-26): the `Deployer` interface and twin (P1), deploy steps, preview gate, release and automatic rollback in the train (P2), `compose-local` with its recipe lint, run by the `deployer` service under profile `deploy` (P4; #278, #291, #292, #298, #312). Stubs and local git only, none against a real Docker engine.
- Last done (2026-11-26, after a live check on a real engine): the recipe lint is an allowlist (#330) and a release whose stack never becomes healthy rolls back on its own (#353). Both are tested on stubs and a real `docker compose config` render; a Mac re-run (its entry in `docs/plan/live-checks.md` is still pending) found that the render reads other files before the lint, so the recipe is now one file, checked before the render (merged, live check pending: `docs/plan/live-checks.md`).
- Last done (2026-11-26, P6a): the conductor sweeps ended tasks (released, rolled back, merged by the team, canceled, rejected) for previews, tears each down under its own time limit and retries a failed one on the next sweep; T4 scenarios `deploy-release` and `deploy-rollback` run preview, acceptance, release, a failed smoke and the automatic rollback on the twin. Merged, [live check pending](../../live-checks/P6a-preview-teardown.md). Still open in P6: teardown N hours after acceptance, the preview limit, the revert PR.
- Last done (2026-11-26, P4b): the CI job `engine` runs `make engine-test` on the runner's Docker engine: nine escape recipes are refused with no container created, and a plain recipe comes up on loopback, passes its smoke, answers HTTP and goes away.
- Next: P6 revert PR and frozen train on rollback, teardown N hours after acceptance and the preview limit; then P3 `script` and `external`, P5 live acceptance.
- Mac, when back: `docker compose --profile deploy up -d --build` (the alpine `docker-cli` packages are unverified), a preview on 127.0.0.1, the token readable by the deployer, `deploy/init-secrets.sh` rerun.

## Scope and open work

Built: Verifier role, verdict and the review leg on `fake`; P1 (`integration.Deployer`, the `deploy/fake` twin, `compose-local` in `capabilities` of `schemas/project.json`); P2 steps `up`, `smoke`, `release`, `rollback` through `book.deploy_steps` (`internal/deploy`), the preview gate in the conductor, release through the train with the automatic rollback, on the twin (the full T4 scenario file is open). Open, in order (design section 17 "Deploy targets", ADR 0014): P3 adapters `script` and `external`; P4 built, its T6 escapes run on a real engine in CI (P4b); P5 live acceptance through playwright-cli on the preview, hidden scenarios, #deployments card; P6 revert PR and frozen train on rollback, preview limits and teardown after acceptance (P6a, teardown of ended tasks, is built), the polygon run in `deploy/upgrade.sh` (its live check and tag switch are built). `compose-ssh`, `kubernetes` and `load` wait for their first tenant. Done: on `compose-local` a Slack task ends with an evidence card and a torn-down preview; Merge deploys; a failed smoke rolls back on its own. Latent: after `revert_merged` the old merged `pr_number` would end the task at once (outside-merge watch); clear it at the `rolled_back` to `building` move.
