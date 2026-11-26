# cmd/

`cmd/nightshift` is the only binary. It parses commands and calls `internal/`; no business logic lives here.

## Where things are

- `main.go` wires the subcommands. Each group has its own file: `serve.go`, `work.go`, `check.go`, `skills.go`, `tenant.go`, `task.go`, `eval*.go`, `setup*.go`, `doctor.go`, `memory.go`, `digest.go`, and so on. List them with `ls cmd/nightshift`.
- A new subcommand is a new file plus a test. Update `README.md` or `docs/runbook.md` when a person runs it.

## Checks that read the repo

- `promises_test.go`: a prompt, role or doc that names a command, skill, role key or schema field must name one that exists. A gap allowed there needs an open plan item (`docs/plan.md`).
- `design_test.go`: the compose example and the subcommands in `docs/design.md` equal the code.
- Run them: `go test ./cmd/nightshift -run 'TestPrompts|TestRole|TestSchemaFields|TestDesign'`.

## Rules

- A flag or key that no code reads is a defect; remove it or add a plan item.
- Tests here use the synthetic tenants in `testdata/tenants/`, never a real project.
- Run `make verify` before every push; see `internal/AGENTS.md` for test rules.
