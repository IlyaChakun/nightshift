# Testing and evals

> Part of the [Nightshift design](../design.md). How the factory is tested without models, network or tokens, and how evals measure it on reference tasks.

## 20. Testing the factory and evals

Two questions, kept apart: **does the factory code work** (state machine, gates, merge train, isolation), checked by deterministic tests without models on every PR at $0; and **do the agents work well** (code, documents, defects caught), checked by evals on the polygon, which cost money and run only when something that shapes agent behaviour changes.

### Factory code tests: no models, $0

| Level | What it checks | How | When |
|---|---|---|---|
| **T1. Unit** | State transitions and CAS, blast radius, review policy, masking, scheduler, parsers (SARIF, YAML, harness output) | `go test`, table tests, `go test -fuzz` for parsers | `make verify` |
| **T2. Properties** | Invariants under random events: the task budget holds, a repeated decision is idempotent, an agent cannot lower the blast radius, only `owner` or `approvers` decide; `solo`: no merge without a required Approve, one merge per repository at a time; `team`: the factory never merges or deploys, `handed_off` ends only on a GitHub event or cancel, no self-Approve under four-eyes | [rapid](https://github.com/flyingmutant/rapid) | `make verify` |
| **T3. Integration** | Migrations, River, outbox, `grafana_ro` rights, the jobs API (a Builder token gets no hidden scenarios) | real Postgres | CI |
| **T4. Scenarios** | The whole flow for both profiles, from chat to release and rollback (`solo`) or to `handed_off` and the team's merge (`team`) | twins of Slack, Linear, GitHub, local git and the `fake` adapter; book events compared with a golden file (`-update`) | CI; locally `make e2e` |
| **T5. Failures** | Restart mid-train, 429, 5xx, timeouts, a worker dying mid-job, an expired token, a full disk | fault injection in twins, `kill -9` in T4 | CI at night and on core changes |
| **T6. Sandbox escape** | No read of `/run/secrets`, `/run/worker-secrets` or others' variables; no way past `egress`, to the local network or `169.254.169.254`; no write to protected paths; no Postgres, no hidden scenarios | attack scripts from `testdata/escape/` in the real compose; every attempt must fail | CI on `deploy/`, runner and egress changes, and weekly |
| **T7. Twin contracts** | The twins behave like the real services | the T4 scenarios weekly against free test accounts; a mismatch is a finding | at night |
| **T8. Harness contract** | The adapter understands the current `claude`: `structured_output`, permission denial, session continuation, telemetry, limits | 3 tiny runs on `claude-api` | on a version change in `mise.toml` and weekly, cents |

A `fake` scenario (`testdata/scenarios/*.yaml`) lists the steps: `chat`, `agent` (role and result fixture, optional patch), `click`, `ci`, `github_review`, `expect` (state, status, golden events). The suite covers the happy paths of both profiles, Builder returns, disputes, Request changes, rebase conflicts, tracker cancels, rollback after smoke and an exhausted budget. Every factory bug adds a scenario.

### Evals: agent quality

**The polygon** (`evals/tasks/`) uses the [Harbor](https://github.com/laude-institute/harbor) task format (instruction, hidden tests, reference solution) with our own runner, because the worker has no Docker socket. Tasks cover the stacks of the connected packs. **Regression** tasks should almost always pass; **capability** tasks show what agents still do poorly and move to regression once mastered. We start with 10 and grow the set from escapes.

**Graders by cost:** code first (hidden tests, linters, diff size, protected paths, cost), then an LLM judge with a rubric only where code cannot check. One judge checks one dimension and may answer "I don't know".

| Level | What we evaluate | Cost |
|---|---|---|
| **E1. A role on frozen inputs** | one role on saved artifacts (Verifier on diffs with planted bugs, an L4 rubric on a weak PRD); precision and recall | cents per case |
| **E2. A role on a task** | a role solves a polygon task end to end, graded by hidden tests | the most expensive |
| **E3. The whole factory** | Lead, Builder, Verifier on twins, idea to verdict | weekly |
| **E4. Production** | every real task: escapes, returns, the owner's comments, rollbacks, cost | $0, the data is in the book |

**Judge calibration.** The owner labels 20–30 examples 👍/👎 in chat; below 80% agreement a judge only warns. Weekly, Verifier in audit mode reads 3 failed eval transcripts: agent or grader at fault?

### How to save on evals

1. Factory logic is tested by T1–T8; core Go changes trigger no evals.
2. `evals/selection.yaml` maps changed paths to sets: a skill or role prompt runs E1 and E2 of that role, a rubric only its E1, a model or harness version the fast set.
3. Frozen inputs: Verifier is evaluated without rerunning Builder.
4. Results are cached by the hash of task, role config, skill and standard SHAs, model and harness version.
5. Sets: smoke (2 tasks) on a PR that changes skills or prompts, fast (5) before merging it, full (10) weekly, holdout monthly.
6. Candidate and current version run on the same tasks, one attempt each; a candidate that lost 2 of the first 3 stops. Three attempts only on contested tasks and before rollout; regression counts pass^k.
7. Judges run on `claude-cheap` through `api`, at night through the Message Batches API (50% off), with the shared prefix cached.
8. A polygon task takes at most 30 minutes and 300k tokens; a stuck run is a failure.
9. Evals get `caps.evals` ($60 a month) of the credit pool; over it only smoke runs and the owner gets a card.
10. Production (E4) is the main eval; the polygon checks a change before production, not instead of it.
11. In the personal instance the role under test may run on `claude-sub` (unmodified `claude`, one session, inside `weekly_share`), because the owner's evals are the owner's own work; batch judges and anything in CI use an API key, and the work instance never uses a personal subscription. Anthropic's terms assume "ordinary, individual usage" ([legal and compliance](https://code.claude.com/docs/en/legal-and-compliance)) and `claude -p` spends the owner's chat limits ([Agent SDK and your plan](https://support.claude.com/en/articles/15036540-use-the-claude-agent-sdk-with-your-plan)); if evals crowd out the owner's work or the terms change, sets move to `claude-api`.

Estimates before measuring: E1 cents per case, an E2 task about $0.5–3, the weekly full set about $10–30; the cost dashboard measures the real numbers.

**Where it lives.** `evals/tasks/<id>/` holds `task.yaml` (id, set, level, role, prompt, the grading `test` command), starting files in `repo/` and hidden tests in `hidden/`. A run makes a throwaway local git repository from `repo/`, lets the harness solve the prompt, copies `hidden/` in afterwards and runs `test`. No GitHub, sandbox repository or book is involved. Results go to `./eval-runs/<candidate>/<task>.json` (schema `eval-result`). Run: `nightshift eval run --set fast --role builder --candidate <branch>`; on a developer machine add `--providers ./providers.local.yaml --provider <name>`, and the key comes from the provider's `key_env`. `caps.evals` applies locally too when the providers file sets it.
