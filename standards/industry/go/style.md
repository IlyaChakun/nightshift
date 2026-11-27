# Go style

How a project writes Go. The project's `.golangci.yml` enforces what tools can; its base is
`standards/industry/go/files/.golangci.yml`, which `nightshift standards sync` copies when the
project has none. This page holds the rules no linter in that base checks, as short reminders for
people and agents. When a rule here starts to matter often, enable a linter for it.

Sources, in order of precedence: [Effective Go](https://go.dev/doc/effective_go),
[Go Code Review Comments](https://go.dev/wiki/CodeReviewComments),
[Google Go Style Guide](https://google.github.io/styleguide/go/guide)
([decisions](https://google.github.io/styleguide/go/decisions),
[best practices](https://google.github.io/styleguide/go/best-practices)),
[Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md) where Google is silent.

## Enforced by tools

| Rule | Linters |
|---|---|
| gofmt and goimports, no hand formatting | `gofmt`, `goimports` |
| Vet checks, simplifications, no dead code or ineffective assignments | `govet`, `staticcheck`, `unused`, `ineffassign`, `wastedassign` |
| Errors are checked, wrapped with `%w`, compared with `errors.Is`/`As`, never dropped for `nil` | `errcheck`, `errorlint`, `nilerr` |
| The caller's `context.Context` is passed on, also to HTTP requests | `contextcheck`, `noctx` |
| Response bodies are closed | `bodyclose` |
| Common security mistakes | `gosec` |
| Current idioms (`min`, `slices`, `strings.Cut`) | `modernize` |
| Every `//nolint` names a linter and says why | `nolintlint` |

## Rules no tool checks

**Keep it simple.** Write the obvious code first. Add an abstraction, option, generic or package
only when a second real caller needs it, not for a future one. Delete dead code instead of keeping
it "just in case"; git keeps history. Prefer the standard library; a little copying is better than
a new dependency.

**Interfaces.** Accept interfaces, return concrete types. Define an interface in the package that
uses it, with the methods that package calls. One implementation and no test double means no interface.

**Names.** Short and clear in the caller's context: `config.Load`, not `config.LoadConfig`. No
`util`, `common` or `helpers` packages. Initialisms keep their case (`ID`, `URL`, `SHA`).

**Errors.** Add context that the caller lacks: `fmt.Errorf("store: save order %d: %w", id, err)`.
Handle an error once: return it or log it, not both. Export a sentinel error only when callers
branch on it. Check JSON encoding errors. No `panic` outside `main` and impossible states.

**Context and logs.** `context.Context` is the first parameter and is never stored in a struct.
Log with `log/slog` key-value pairs and static messages; no global logger, no `fmt.Print` or
`log.Fatal` outside `main`. No `init()`.

**State and concurrency.** A switch over the project's own enum lists every case, or its
`default:` says the rest is ignored on purpose. No mutable package state; read-only lookup tables
and sentinel errors are fine. Dependencies come in through struct fields or constructors. Every goroutine has an owner
that can stop it and wait for it. Prefer synchronous APIs; let the caller add concurrency.

**Comments.** A comment says why: a constraint, an invariant, a trade-off, a link to the design
section or ADR. It does not retell what the next line does. Doc comments are full sentences that
start with the name. No commented-out code and no `TODO` without an issue. A stale comment is a
bug: fix or delete it in the same change.

**Tests.** Table-driven, with `got ..., want ...` messages and `t.Context()`. Plain `testing`,
no assertion libraries. Test behavior through the public API of the package.
