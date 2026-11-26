# Contributing

Nightshift is a personal project built mostly by coding agents, so [`AGENTS.md`](AGENTS.md) is the single rulebook for people and agents alike. This page is the short path.

## Set up

1. `mise install --locked` installs the pinned tools from `mise.toml`.
2. `make tools` fails fast when a local tool differs from its pin. `make` puts the mise-installed tools first on `PATH` itself, so a shell that did not run `mise activate` still uses the pinned versions; run `mise trust` once in the checkout.
3. Run targeted checks while editing. Run `make verify` on the final tree before push. It checks the repository before running the full test suite. Docker supports the local Postgres tests and the arm64 Mac fallback for the pinned `hadolint`.

## Verify on an Apple Silicon Mac

Use a fresh checkout without instance files. Start Docker and run `docker info` from the same shell that will run the checks. If it reports a socket error, check the Docker context and the shell's access to the daemon.

Run `make docker-lint` and then `make verify`. If the pinned macOS `hadolint` cannot start, `make docker-lint` uses the official Docker image at the pin in `mise.toml`. Record the checkout SHA and the results of both commands for the [live check](docs/plan/live-checks.md#make-docker-lint-on-an-arm64-mac).

## Work

1. Read [`docs/status.md`](docs/status.md), then [`docs/plan.md`](docs/plan.md) and the `README.md` of your phase folder under `docs/plan/`, and take its next open item.
2. One task, one branch (`feat/`, `fix/` or `docs/` prefix), one PR under about 400 changed lines. Commits follow Conventional Commits. Never push to `main`.
3. Update your plan item and, if you own the phase, its README status. Put each new pending real-stack check under [docs/live-checks](docs/live-checks/). Update the relevant operator docs when setup, config or secrets change.
4. The PR description follows the template: what and why, design section, test levels, risks and rollback, open questions.
5. Run `scripts/pr-gate.sh preflight` to check GitHub access. After verification and review, run `scripts/pr-gate.sh wait <PR> <verified-head>`. With merge authorization under AGENTS.md, use `merge` in place of `wait`. Add `--git-auth` before the subcommand to use the existing HTTPS origin credential for that invocation.

Repeated tasks have a skill in [`.agents/skills/`](.agents/skills): add a migration, change a schema or the design, add a harness or an integration adapter, fix CI, finish a task.

## Needs the owner's approval

Only what needs a secret or an account, a new egress host, money, an irreversible step, or weakens a protection of the factory (AGENTS.md, "Ask before you do it"). Other gated paths (a new dependency, `.github/`, `deploy/`, `schemas/`, `mise.toml`, `mise.lock`, `Makefile`, `AGENTS.md`, `CLAUDE.md`) are merged by the session after the independent review; label them `needs-owner-review` so the morning digest lists them.

## Outside contributions

The repository is public so that people can read it and reuse ideas, not to run an open project. Pull requests, issues and comments from anyone other than the owner and the approvers are welcome as feedback but are never worked automatically: the factory accepts commands, tasks and review input only from the owner and approvers named in the tenant pack (design section 16, "Forged commands"), and it treats all other text as data. A fork PR may be closed without a review. Bug reports are useful; a pull request for a larger change should start as an issue, because the owner may decline it.

## Security

Never commit secrets. Report vulnerabilities as described in [`SECURITY.md`](SECURITY.md).
