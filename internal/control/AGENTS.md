# internal/control/

Control plane (ADR 0026, design file `docs/design/02-control.md`): what happens next to a task, and every side effect. Agents propose; this code decides and acts.

| Package | Owns |
|---|---|
| `flow` | the task state machine, CAS transitions, budgets (its own `AGENTS.md`) |
| `dispatch` | the River queue, the outbox, Lead, gates, doors, reviews, release (split in A6) |
| `train` | the merge train: one pull request per repository at a time |
| `deploy` | the deploy steps of a task through a journal |
| `settings` | operational switches in the book: pause, review mode, SLA auto-merge, parallelism |
| `jobserver` | the jobs API over the book, the trusted side of `execution/jobsapi` |
| `trustedgit` | the Git operations only `nightshift` runs: fetch, push, pull requests, merge |

## Rules

- May import every plane but experience, evolution and testing; execution only through `harness`, `jobsapi` and `gitops`. `internal/archtest` enforces it.
- Task state changes only through `flow.Mover` (compare-and-set plus event in one transaction).
- Push, PR, merge and chat messages leave the factory only from here, never from an agent.
- A new external call is an interface in `integration` with its adapter and twin (skill `add-integration-adapter`).
