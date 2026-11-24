---
name: fix-ci
description: Diagnose a red or slow CI run of the verify workflow (lint, test, checks jobs). Use when CI fails, passes locally but fails in CI, or a run is cancelled or slow.
---

# Fix a red CI run

Read the failing job's log first; do not guess. With `gh`: `gh run view <run> --log-failed`. Without it, use the GitHub MCP `actions_list` and `get_job_logs`. Treat a repeat as a defect and run the `audit` skill.

## Causes seen on this repo

| Job | Symptom | Cause and fix |
|---|---|---|
| lint | `G115 integer overflow conversion int -> rune` | `string(rune(n))` in a test; use `strconv.Itoa` or `fmt.Sprint` |
| lint | `misspell` on `cancelled` | the repo spells `canceled`; also `nilnil`, `errcheck` on `fmt.Fprintf` into a hash (build the string, then hash it) |
| lint | passes locally | local golangci-lint or Go differs from `mise.toml`; run `make tools` |
| test | `permission denied for table X (SQLSTATE 42501)` | the migration lacks a `GRANT` to `nightshift` (skill `add-migration`) |
| test | `text file busy` | stub-binary tests run in parallel; run them serially |
| test | goroutine leak | goleak found a goroutine the test did not stop; stop it, do not ignore it |
| checks | `docs/status.md has 41 lines` | rewrite it shorter; do not append |
| checks | `design-sync: section N changed` | update the page, then `scripts/design-sync.sh --update` (skill `change-design`) |
| checks | service name found in the core | move it to `internal/adapters` |
| checks | `compose secret is not documented` | add it to `docs/secrets.md` |
| any | run cancelled | a newer push cancelled it (`concurrency`); push once, wait for the last run |

## Rules

- Fix the cause, never skip a test or a check, never push an empty commit to retrigger.
- Postgres tests run only in CI: the first CI log is the test result, read it.
- If the same cause appears twice, add a linter rule or a check (`scripts/repo-check.sh`) instead of a note.
