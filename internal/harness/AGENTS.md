# internal/harness

The only place that talks to a model. Everything else uses the `Harness` interface.

- A harness runs one role turn in a workspace and returns JSON that `Validating` has checked
  against the role's schema in `schemas/`. Free text is never a result.
- Provider SDKs and CLIs live in a subpackage (`claude`, `fake`, later `codex`, `copilot`,
  `api`). Nothing outside `internal/harness/` imports them.
- Credentials come in through `RunRequest.Env` and only what that provider needs. An API-key
  run must never see the subscription token, and the reverse.
- Run the CLI with explicit permissions and a fixed tool list. The agent user has no
  GitHub, chat or tracker token and no Postgres.
- `fake` replays the `agent` steps of a scenario file; use it in every test. Tests never
  call a real model. Contract tests against a real CLI are opt-in (T8).
- A new adapter needs a stub-binary test of its argument list and its error mapping (limit
  hit, timeout, invalid output), plus a line in the provider table of design section 12.
