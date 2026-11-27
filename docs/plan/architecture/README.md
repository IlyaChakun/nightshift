# Stream — architecture planes in code

Owned by the session working on this stream; another session proposes an item as a new file here. Rules and limits: `docs/plan.md`. Source: the owner, 2026-11-27: the factory's layers must show in the Go packages, with interfaces between them and architecture tests ("Architecture enforcement & package migration" plan, option A: `internal/<plane>/<package>`). Planes and rules: ADR 0026.

Done when: every production package lives under `internal/<plane>/` (or is a named composition root), the architecture test runs in `make verify` with no unapproved exception, the worker side reaches no book or server package, and each replaceable component (harness, retrieval, deploy target, door, persistence) has a contract test.

## Status

- Last done: A7 part 5, the doors contract `internal/control/dispatch/doors_contract_integration_test.go`: chat, cli and mcp approve a spec with one move recorded by their door and the same effect; a repeated approval and one on an older card change nothing (cli and mcp refuse with `ErrDoor`). Part 4 (#449): the deploy target suite `internal/execution/deployapi/deploytest`, also on a real engine (`make engine-test`).
- Next: A7, the retrieval and persistence suites.
- Open: none.

## Rules for this stream

- One coherent boundary or component per PR, under ~400 changed lines; moves are mechanical and change no behavior. Rollback is a revert.
- Static rules and runtime rights are different claims: an import test never proves isolation or authorization. Those need integration or negative tests.
- No empty packages for planned features, no central `interfaces`, `common` or `utils` package, no new service or runtime dependency to move files.
- Before each move: check open PRs of other threads on the same paths, and every consumer of the path (imports, `go:embed`, `sqlc.yaml`, generated code, Makefile, Dockerfile, scripts, `.golangci.yml`, CI, docs and `AGENTS.md`).

## Items

- [x] A1. `docs(arch): package inventory and dependency policy` — reproduce the production import graph with `go list -json -deps ./...` (test packages separately); classify every package by plane with file evidence; list ambiguous ones; propose the allowed plane-to-plane matrix and each current violation as an exact exception (source, target or path, rule, plan item, removal condition). Known on 2026-11-27: `dispatch` imports 17 internal packages and `runner` for `PatchName`; `runner` → `jobsapi` → `book/bookdb`; `integration` acts as a central contracts package; `dispatch` and `deployer` import concrete adapters. No code change
- [x] A2. `test(arch): architecture test ratchet` — `internal/testing/archtest` (test only) reads the `go list` graph and the one canonical policy from A1; checks direct and transitive reachability, fails on an unknown package, a new violation, a broad wildcard or an unused exception, and names the violating path. Fixtures prove a forbidden edge fails and an allowed one passes. depguard in `.golangci.yml` keeps the fast vendor and adapter rules, generated from or checked against the same policy. Runs in `make verify`; no new workflow
- [x] A3. `docs(design): settle the plane vocabulary before moves` — the concept part of the owner's review: folder names (`customization` or `configuration`, `testing` or `evaluation`, `observability` as its own plane); factory memory and tenant memory as separate parts of knowledge; product intelligence and objective management in the map; the vision as the whole product life cycle; observability and capabilities plane files. Only adopted decisions go into the design and ADR 0026 (or a successor); research stays proposals
- [x] A4. `refactor(arch): extract the job protocol, patch identity and trusted Git operations` — data-only job protocol (request, result, artifact descriptor, errors) that the worker client and the server both use, so `runner` no longer reaches `book/bookdb`; `PatchName` moves into that protocol; worker checkout split from trusted push, PR and merge. One boundary per PR; wire formats, schema validation, transactions and worker isolation unchanged
- [x] A5. `refactor(arch): move packages into plane folders` — one plane or component batch per PR, starting with the least coupled one from A1. Each plane folder gets a short `AGENTS.md` (responsibility, forbidden dependencies, extension points, link to its design file) and the `CLAUDE.md` shim. Policy, exceptions and docs change in the same PR; no new exception
- [x] A6. `refactor(arch): split integration and dispatch by consumer` — move each interface of `integration` to the package that consumes it (a stable contract shared by several consumers may stay shared); split `dispatch` by responsibility (orchestration, scheduling, routing, decisions, release and recovery), not by file count. Transactions and canonical state unchanged. The conductor stays one state machine with a file per stage; the router, the tracker projection and the reply checks leave (ADR 0027)
- [ ] A7. `test(arch): contract and negative tests per component` — one contract suite per replaceable component (harness: cancel, result, errors, usage; retrieval: tenant and visibility filters, provenance; deploy: idempotence, exact revision, rollback; doors: same command semantics, stale or duplicate approval rejected; persistence: CAS, result plus state plus outbox in one transaction, lease fencing on real Postgres) and runtime negative tests (worker cannot reach the book or trusted credentials, cross-tenant retrieval fails); then remove the remaining exceptions one by one
