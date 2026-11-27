---
name: add-harness-adapter
description: Add or change a model harness (claude, codex, copilot, api, fake) under internal/execution/harness. Use when a new CLI or API provider must run roles, or an existing adapter's flags, environment or error mapping change.
---

# Add a harness adapter

Models are reached only through the `Harness` interface (AGENTS.md rule 3). Read `internal/execution/harness/AGENTS.md` and design section 12.

1. **Put it in its own subpackage** `internal/execution/harness/<name>/`. Nothing outside `internal/execution/harness/` imports a provider SDK or CLI wrapper; `cmd/nightshift/work.go` only wires it.
2. **Implement `Run`.** Return output already checked by `Validating` against the role's schema. Free text is never a result.
3. **Explicit everything.** Fixed tool list, explicit permissions, no reliance on auto-discovered instructions: the runner passes `SystemFile` and `AddDirs` (design section 12).
4. **Credentials only through `RunRequest.Env`**, only what this provider needs. An API-key run never sees the subscription token, and the reverse. Keys live in `deploy/secrets/harness/` (gated; see `docs/secrets.md`).
5. **Map errors.** Limit hit (`Usage.LimitHit`), timeout and invalid output each become a distinct result for the router.
6. **Test without a model (T1).** A stub binary in `t.TempDir()` asserts the argument list and the three error mappings. Stub tests that write and run a script can fail with `text file busy`: run them serially (see PR #77). Real-CLI contract tests are T8 and opt-in.
7. **Docs.** Add a line to the provider table in design section 12, and mention setup in `docs/runbook.md` ("Harness credentials") when a person must configure something.

A new Go module or npm package is a gated change: label the PR `needs-owner-review`; it merges under the merge rule (AGENTS.md).
