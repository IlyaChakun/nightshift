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
| test | a package takes 30 s or more | a Postgres container per test; use `pgtest.Start` (one container per binary, database from a template) |
| any | `lint` and `test` show "skipped" | the PR changed only docs paths (`changes` job); expected. `ci-ok` is the aggregate; it fails only on a failed or cancelled job |
| any | run cancelled | a newer push cancelled it (`concurrency`); push once, wait for the last run |

## Main is red

A red `main` blocks every merge, so it is fixed first and by one session.

1. Look for a fix before you write one: the open PRs and the last commits on `main`. If a fix is open, wait for it and do not open a second.
2. If none is open, open the fix PR at once, as a draft with a title that names the failing check, so the next session finds it. Tell the coordinator, where there is one.
3. Look at how the repo solved the same failure before (`git log` on the file, an existing ignore or allowlist entry) and follow it.
4. After the fix merges, merge `origin/main` into every open branch of yours.

## Rules

- Fix the cause, never skip a test or a check, never push an empty commit to retrigger.
- Postgres tests run only in CI: the first CI log is the test result, read it.
- If the same cause appears twice, add a linter rule or a check (`scripts/repo-check.sh`) instead of a note.

## CI is slow

Measure first; queue time is usually about 2 s, so the slow part is a job step.

1. Run times: `gh api "repos/<owner>/<repo>/actions/workflows/verify.yml/runs?per_page=60"` and take `updated_at - run_started_at` (p50 and p90 by event).
2. Jobs and steps: `gh api repos/<owner>/<repo>/actions/runs/<id>/jobs` gives `started_at`/`completed_at` per job and step; the biggest step is the target.
3. Packages: in the `make test` log, each `ok <package> <seconds>` line is one package; the slowest one sets the job time. Run `go test -race -count=1 -v ./<pkg>/` and look at the slowest tests.
4. Cache: the cache step logs "Cache hit"; a full compile after a hit means the key or path is wrong.

Known baseline (2026-11): run p50 about 75 s, `test` job about 65 s, `lint` about 41 s, `checks` about 45 s. A change that adds 30 s or more to a job needs a cause in the PR.
