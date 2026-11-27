# Go style

How we write Go in Nightshift. Tools enforce what they can (`.golangci.yml`, run by `make verify`);
this page holds only the rules no linter checks, as short reminders for people and agents.
When a rule here starts to matter often, move it into a linter or an L4 rubric (design section 9).

Sources, in order of precedence: [Effective Go](https://go.dev/doc/effective_go),
[Go Code Review Comments](https://go.dev/wiki/CodeReviewComments),
[Google Go Style Guide](https://google.github.io/styleguide/go/guide)
([decisions](https://google.github.io/styleguide/go/decisions),
[best practices](https://google.github.io/styleguide/go/best-practices)),
[Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md) where Google is silent.

## Enforced by tools

| Rule | Linters |
|---|---|
| gofmt and goimports, no hand formatting | `golangci-lint fmt` |
| Doc comments on exported names, naming, error strings, early return | `revive` (defaults), `staticcheck` |
| Errors are checked, wrapped with `%w`, compared with `errors.Is`/`As` | `errcheck`, `errorlint`, `nilerr`, `nilnesserr`, `nilnil`, `errname` |
| `context.Context` first, never stored in a struct, passed to HTTP and logs | `containedctx`, `noctx`, `fatcontext`, `sloglint` |
| Logs: `log/slog`, key-value pairs, `snake_case` keys, static messages, no global logger | `sloglint`, `forbidigo` |
| Bodies, rows and spans are closed or ended | `bodyclose`, `sqlclosecheck`, `rowserrcheck`, `spancheck` |
| No dead parameters, needless conversions or assignments | `unparam`, `unconvert`, `wastedassign`, `unused` |
| Our enums are switched exhaustively; `default:` says the rest is ignored on purpose | `exhaustive` |
| Current idioms (`min`, `slices`, `errors.AsType`, `t.Context()`) | `modernize`, `usetesting`, `usestdlibvars` |
| No `init()`, no `fmt.Print` or `log.Fatal` outside `main` writers | `gochecknoinits`, `forbidigo` |
| Every `//nolint` names a linter and says why | `nolintlint` |
| Complexity and duplication stay low | `gocognit` (20), `dupl` |

## Rules no tool checks

**Keep it simple.** Write the obvious code first. Add an abstraction, option, generic or package
only when a second real caller needs it, not for a future one. Delete dead code instead of keeping
it "just in case"; git keeps history. Prefer the standard library; a little copying is better than
a new dependency.

**Interfaces.** Accept interfaces, return concrete types. Define an interface in the package that
uses it, with the methods that package calls. One implementation and no test double means no interface.
Exception: contracts that the design names for a later phase (the chat, code host and tracker contracts,
the harness types, policy checks) may exist before their first caller; they change with the design.

**Names.** Short and clear in the caller's context: `config.Load`, not `config.LoadConfig`. No
`util`, `common` or `helpers` packages. Initialisms keep their case (`ID`, `URL`, `SHA`).

**Errors.** Add context that the caller lacks: `fmt.Errorf("book: append event %d: %w", id, err)`.
Handle an error once: return it or log it, not both. Export a sentinel error only when callers
branch on it. No `panic` outside `main` and impossible states.

**State and concurrency.** No mutable package state; read-only lookup tables and sentinel errors
are fine. Dependencies come in through struct fields or constructors. Every goroutine has an owner
that can stop it and wait for it. Prefer synchronous APIs; let the caller add concurrency.

**Comments.** A comment says why: a constraint, an invariant, a trade-off, a link to the design
section or ADR. It does not retell what the next line does. Doc comments are full sentences that
start with the name. No commented-out code and no `TODO` without an issue. A stale comment is a
bug: fix or delete it in the same change.

**Tests.** Table-driven, with `got ..., want ...` messages and `t.Context()`. Plain `testing`,
no assertion libraries. Test behavior through the public API of the package.

## Deliberately not enabled

These linters were tried on this code base and add noise without catching real defects:
`wrapcheck` and `err113` (flag every unwrapped or ad hoc error, including in tests),
`gochecknoglobals` (flags read-only tables and sentinel errors), `thelper` (flags `t.Run` bodies),
`varnamelen`, `exhaustruct`, `mnd`, `lll`, `wsl`, `nlreturn`, `godot` (style taste, not defects),
`funlen` and `cyclop` (`gocognit` measures the same better), `paralleltest`, `testpackage`, `ireturn`.
