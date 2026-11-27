# internal/execution/

Execution plane (ADR 0026, design file `docs/design/03-execution.md`): where agent work runs, in the worker, behind the job protocol.

| Package | Owns |
|---|---|
| `harness` | the `Harness` interface and its adapters (`claude`, `codex`, `fake`), `supervise` (its own `AGENTS.md`) |
| `runner` | the worker loop: claim a job, prepare the worktree, run the role, run checks, return the patch |
| `jobsapi` | the data-only job protocol between `nightshift` and the worker |
| `gitops` | the worker's half of Git: checkouts and diffs; `internal/trustedgit` (control) pushes |
| `toolchain` | installs a project's pinned tools in its own service |
| `skills` | vendors external skills by commit SHA |
| `deployer` | the `deployer` service, the only one that runs a deploy target |

## Rules

- Imports execution, assurance, customization and `integration` only; `internal/archtest` enforces it. Control reaches it only through `harness`, `jobsapi` and `gitops`.
- The worker roots never reach the book or a Postgres client, and the worker gets no GitHub, chat or tracker token.
- A new model provider is a `harness/<name>` subpackage (skill `add-harness-adapter`); a new tool is a stack pack before it is code.
