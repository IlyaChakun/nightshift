# Architecture inventory: packages, planes and dependencies

Item A1 of `docs/plan/architecture/`. It measures the Go import graph of `main` at `c084044` (2026-11-27) and proposes a policy for the A2 architecture test. Planes and rules: ADR 0026. Folder names follow ADR 0026: `internal/<plane>/<package>`.

Summary: 48 Go packages, no build tags in production code, so one graph covers every build. Most edges already point the way ADR 0026 wants. Five violations (E1–E3, E5, E6) needed an exact exception (A4 removed E1 and E2), and four packages span two planes and must be split before they move.

## How it was measured

```bash
go list -f '{{.ImportPath}}|{{join .Imports " "}}|{{join .TestImports " "}} {{join .XTestImports " "}}' ./...
go list -f '{{.ImportPath}}: {{join .Imports " "}}' ./internal/... ./cmd/...   # third-party imports
grep -rl '^//go:build' --include=*.go internal cmd schemas deploy             # only deploy/*_test.go
```

Test-only edges are listed apart; they are allowed to reach fakes and `testutil`. A2 must repeat the measurement with `go list -deps` for transitive reachability.

## Packages by plane

| Plane | Packages (under `internal/` unless noted) |
|---|---|
| Composition roots | `cmd/nightshift` (imports 38 internal packages), `adapters` (picks the implementation a tenant names) |
| Experience | `mcpdoor`, `adapters/chat/slackapp`, `adapters/chat/inbox` |
| Observability | `summary` (owner, 2026-11-27) |
| Control | `flow`, `dispatch`, `train`, `settings`, `deploy`, `adapters/github` |
| Execution | `runner`, `harness`, `harness/claude`, `harness/codex`, `harness/supervise`, `toolchain`, `skills`, `deployer`, `adapters/deploy/composelocal`, `adapters/deploy/remote` |
| Knowledge | `search`, `memory` |
| Governance and assurance | `check`, `policy`, `standards`, `rubric`, `deps` |
| Evolution | `learn` |
| Data foundation | `book`, `book/bookdb` |
| Customization | `config`, `onboard`, `stack`, `schemas/` (embedded JSON Schemas) |
| Testing | `evals`, `testutil/golden`, `testutil/pgtest`, fakes (`harness/fake`, `adapters/*/fake`), `deploy/` (compose tests) |
| Shared contracts (to split in A6) | `integration` |

### Packages that span two planes

| Package | Why | Split in |
|---|---|---|
| `jobsapi` | Protocol types and the worker's `Client` (Execution) live with the server that claims rows in the book (`server.go`, `memory.go` use `bookdb`) | A4: split, the server is `jobserver` |
| `gitops` | The worker's checkout and diff live with the trusted `Push`, `PushRebase` and `Revert` | A4: split, the trusted half is `trustedgit` |
| `dispatch` | 41 files: River jobs, outbox, Lead, gates, doors, reviews, release; imports 16 internal packages | A6 |
| `integration` | One file of interfaces for every external system: `Tracker`, `DocsTarget`, `Chat`, `Reactor`, `PullRequests`, `ExternalChecks`, `MergeLog`, `PlanLog`, `Deployer`, `RepoHealth` | A6 |

One choice waits for A3. `stack` loads stack packs (Customization), but its `Tools` method returns `check.Tool`.

## Proposed policy

An edge is a production import from a package of the row's plane to one of the column's plane. "contracts" means `integration` until A6 moves each interface to its consumer. "port" means only the interface package (`harness`, not `harness/claude`).

| From \ To | Exp. | Control | Exec. | Know. | Assur. | Evol. | Data | Cust. | contracts |
|---|---|---|---|---|---|---|---|---|---|
| Experience | yes | yes, commands | no | no | no | no | read only | yes | yes |
| Control | no | yes | protocol and port only | yes | yes | no | yes | yes | yes |
| Execution | no | no | yes | no | yes | no | **no** | yes | yes |
| Knowledge | no | no | no | yes | no | no | yes | yes | no |
| Assurance | no | no | port only (`harness`) | no | yes | no | no | yes | no |
| Evolution | no | no | port only | yes | yes | yes | yes | yes | no |
| Data | no | no | no | no | no | no | yes | no | no |
| Customization | no | no | no | no | yes (`check.Tool`, see A3) | no | no | yes | no |

Observability may import itself, Data, Customization, the contracts and only `flow` from Control; Experience and Control may import Observability.

Further rules for A2:

- Only a composition root imports a concrete adapter (`adapters/**` other than fakes). Tests may import fakes.
- No package reachable from the packages `nightshift work` wires (`runner`, `jobsapi`, `gitops`, `toolchain`, `stack`, `config`, `harness` and its adapters) may import `book`, `book/bookdb` or `github.com/jackc/pgx`; each importing edge is its own violation. This is a static claim; the worker's network and secrets hold the runtime one (A7).
- The vendor SDK rules of depguard (`.golangci.yml`, rule `core`) stay as they are; A2 checks them against the same policy file.
- An unknown package fails until it is classified.

## Exceptions on day one

Each one is an exact edge with the item that removes it. A2 fails on any other violation and on an exception that no longer matches.

| # | Edge | Evidence | Rule broken | Removed by |
|---|---|---|---|---|
| E1 | `dispatch` → `runner` | `internal/dispatch/build.go:27`, only `runner.PatchName` | Control imports an Execution implementation | A4: `PatchName` moves into the job protocol (removed) |
| E2 | `runner` → `jobsapi` → `book/bookdb`, `pgx` | `internal/jobserver/server.go`, `memory.go` (before A4 under `jobsapi`); `runner` uses only protocol types and `Client` | Execution reaches the book | A4: protocol and client apart from the server in `jobserver` (removed) |
| E3 | `dispatch` → `adapters/chat/inbox` | `internal/dispatch/doors.go` | Core imports a concrete adapter | A6 |
| E5 | `deployer` → `adapters/deploy/composelocal`, `adapters/deploy/remote` | `internal/deployer/deployer.go` | Core imports a concrete adapter | A6: `deployer` is the entry of its own service, so it may become a composition root (named in A3) |
| E6 | `stack` → `check` | `internal/stack/stack.go:107` | Customization depends on Assurance | A5: move `stack` or the tool type (decided in A3) |

Not exceptions:

- `adapters/chat/inbox` → `book/bookdb` (Experience) and `summary` → `book/bookdb` (Observability) read the book, and a static import cannot tell a read from a write. That the inbox does not change task state directly is a runtime claim for A7.

- `mcpdoor` → `dispatch` (Experience to Control) and `summary` → `flow` (Observability reads the task states of `flow`, its only Control port) point the allowed way.
- `check` → `rubric` → `harness` and `evals` → `harness` use the port only.
- `train` → `gitops` is Control using the trusted Git half. Since A4 it imports `trustedgit`, which builds on the worker half `gitops`.

## What was not verified

- Transitive reachability: the measurement lists direct edges, and reachability above is followed by hand. A2 computes it with `go list -deps`.
- Runtime claims (who may write, merge or release) are not measured here.
- Line numbers point to `c084044` and move with later changes.
