# internal/control/

Control plane (ADR 0026, design file `docs/design/02-control.md`): what happens next to a task, and every side effect. Agents propose; this code decides and acts.

| Package | Owns |
|---|---|
| `flow` | the task state machine, CAS transitions, budgets (its own `AGENTS.md`) |
| `dispatch` | ticker, conductor stages, Lead, gates, release |
| `doors` | chat events and commands, inbox doors, adding tasks |
| `periodic` | jobs the ticker runs per tenant: morning digest, health tasks |
| `outbox` | delivery of side effects: enqueue with the change, quiet hours, retries, chat sender, River client |
| `mask` | masking of secrets and token shapes in text that leaves the host |
| `train` | the merge train: one pull request per repository at a time |
| `deploy` | the deploy steps of a task through a journal |
| `settings` | operational switches in the book: pause, review mode, SLA auto-merge, parallelism, weekly share |
| `jobserver` | the jobs API over the book, the trusted side of `execution/jobsapi` |
| `trustedgit` | the Git operations only `nightshift` runs: mirrors, applying a patch, rebase, push, revert |
| `chatapi`, `codehost`, `trackerapi` | contracts of chat, code host and tracker; adapters in `internal/adapters` |

## Rules

- May import every plane but experience, evolution and testing; execution only through `harness`, `jobsapi`, `gitops` and `deployapi`. `internal/testing/archtest` enforces it.
- Task state changes only through `flow.Mover` (compare-and-set plus event in one transaction).
- Push, PR, merge and messages are decided in code (here or in `cmd/nightshift`), never by an agent; the calls go through the contracts `chatapi`, `codehost` and `trackerapi`, implemented in `internal/adapters`.
- A new external call is an interface in the package of its consumer, with its adapter and twin (skill `add-integration-adapter`).
