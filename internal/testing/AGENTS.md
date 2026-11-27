# internal/testing/

Testing and evaluation (ADR 0026, design file `docs/design/10-testing.md`): how the factory proves its own work without models, network or tokens.

| Package | Owns |
|---|---|
| `evals` | which evals a change selects (`evals/selection.yaml`) and the local eval runs |
| `testutil/golden` | golden-file comparison (`-update`, then review the diff) |
| `testutil/pgtest` | one throwaway Postgres per test binary, the only way a test gets Postgres |
| `archtest` | the plane policy (`policy_test.go`) checked on the `go list` graph; no exported mutable package variables (`globals_test.go`) |

Twins (`*/fake`) and the tests of `deploy/` belong to this plane too but stay where they are.

## Rules

- Production code of another plane never imports testing; a composition root or a test may.
- A new package under `internal/` outside the trees in `treesAllowed` gets its plane in `archtest/policy_test.go`, or `go test ./...` fails.
