# Architecture inventory: packages, planes and dependencies

Item A1 of `docs/plan/architecture/`. It measures the Go import graph of `main` at `c084044` (2026-11-27) and proposes a policy for the A2 architecture test. Planes and rules: ADR 0026. The folder names here are today's plane names; A3 may rename them before any package moves.

Summary: 48 Go packages, no build tags in production code, so one graph covers every build. Most edges already point the way ADR 0026 wants. Seven current violations need an exact exception, and four packages span two planes and must be split before they move.

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
| Experience | `mcpdoor`, `adapters/chat/slackapp`, `adapters/chat/inbox`, `summary` |
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
| `jobsapi` | Protocol types and the worker's `Client` (Execution) live with the server that claims rows in the book (`server.go`, `memory.go` use `bookdb`) | A4 |
| `gitops` | The worker's checkout and diff live with the trusted `Push`, `PushRebase` and `Revert` | A4 |
| `dispatch` | 41 files: River jobs, outbox, Lead, gates, doors, reviews, release; imports 16 internal packages | A6 |
| `integration` | One file of interfaces for every external system: `Tracker`, `DocsTarget`, `Chat`, `Reactor`, `PullRequests`, `ExternalChecks`, `MergeLog`, `PlanLog`, `Deployer`, `RepoHealth` | A6 |

Two choices wait for A3. `summary` is either Experience (it presents) or Observability (it builds read models). `stack` loads stack packs (Customization), but its `Tools` method returns `check.Tool`.

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

Further rules for A2:

- Only a composition root imports a concrete adapter (`adapters/**` other than fakes). Tests may import fakes.
- No package reachable from `runner` may reach `book`, `book/bookdb` or `github.com/jackc/pgx`. This is a static claim; the worker's network and secrets hold the runtime one (A7).
- The vendor SDK rules of depguard (`.golangci.yml`, rule `core`) stay as they are; A2 checks them against the same policy file.
- An unknown package fails until it is classified.

## Exceptions on day one

Each one is an exact edge with the item that removes it. A2 fails on any other violation and on an exception that no longer matches.

| # | Edge | Evidence | Rule broken | Removed by |
|---|---|---|---|---|
| E1 | `dispatch` → `runner` | `internal/dispatch/build.go:27`, only `runner.PatchName` | Control imports an Execution implementation | A4: `PatchName` moves into the job protocol |
| E2 | `runner` → `jobsapi` → `book/bookdb`, `pgx` | `internal/jobsapi/server.go:211`, `memory.go:52`; `runner` uses only protocol types and `Client` | Execution reaches the book | A4: protocol and client apart from the server |
| E3 | `dispatch` → `adapters/chat/inbox` | `internal/dispatch/doors.go` | Core imports a concrete adapter | A6 |
| E5 | `deployer` → `adapters/deploy/composelocal`, `adapters/deploy/remote` | `internal/deployer/deployer.go` | Core imports a concrete adapter | A3 decides: `deployer` is the entry of its own service, so it may become a composition root; else A6 |
| E6 | `stack` → `check` | `internal/stack/stack.go:107` | Customization depends on Assurance | A3: move `stack` or the tool type |
| E7 | `dispatch` → `summary` | found by the A2 test | Control imports Experience | A6 |

Not exceptions:

- `adapters/chat/inbox` → `book/bookdb` (formerly E4) and `summary` → `book/bookdb`: Experience may read the book, and a static import cannot tell a read from a write. That the inbox does not change task state directly is a runtime claim for A7.

- `mcpdoor` → `dispatch` and `summary` → `flow` point the allowed way (Experience to Control).
- `check` → `rubric` → `harness` and `evals` → `harness` use the port only.
- `train` → `gitops` is Control using the trusted Git half. Once A4 splits `gitops`, the rule names that half.

## What was not verified

- Transitive reachability: the measurement lists direct edges, and reachability above is followed by hand. A2 computes it with `go list -deps`.
- Runtime claims (who may write, merge or release) are not measured here.
- Line numbers point to `c084044` and move with later changes.
