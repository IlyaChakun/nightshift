# internal/execution/

Execution plane (ADR 0026, design file `docs/design/03-execution.md`): where agent work runs, in the worker, behind the job protocol.

| Package | Owns |
|---|---|
| `harness` | the `Harness` interface and its adapters (`claude`, `codex`, `fake`), `supervise`; see `harness/AGENTS.md` |
| `runner` | the worker loop: claim a job, prepare the worktree, run the role, run checks, return the patch |
| `jobsapi` | the data-only job protocol between `nightshift` and the worker |
| `gitops` | the worker's half of Git: checkouts and diffs; only control calls its raw `Run` and `RunUntrimmed` (`trustedOnly` in archtest); `internal/control/trustedgit` pushes |
| `toolchain` | installs a project's pinned tools in its own service |
| `skills` | vendors external skills by commit SHA |
| `deployapi` | the deploy target contract: `Deployer` and the values it exchanges |

## Rules

- Imports execution, assurance and customization only; `internal/testing/archtest` enforces it. Control reaches it only through `harness`, `jobsapi`, `gitops` and `deployapi`.
- The worker roots never reach the book or a Postgres client, and the worker gets no GitHub, chat or tracker token.
- The `fake` harness stays in the testing plane: only a composition root or a test may import it.
- A new model provider is a `harness/<name>` subpackage (skill `add-harness-adapter`); a new tool is a stack pack before it is code.
