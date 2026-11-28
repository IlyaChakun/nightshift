# Where the MVP validation day went (2026-11-28)

The Mac validation (`docs/plan/mvp-validation/README.md`) ran from 12:07 to 18:42 Zurich time (395 min) and had not passed V4 by then. The checks are not slow: CI takes about 2 minutes and one Mac round about 25. The day went to ten defects found one at a time, each costing a serial fix and a Mac round (7 rounds, 6 failed), to a green fix that sat unmerged for 104 minutes, and to 57 minutes of waiting for owner answers.

## Sources

- Merge times: `git log origin/main` and `GET /repos/.../pulls/<n>` for #470–#487.
- CI: `GET /repos/.../actions/runs?created=>=2026-11-28T09:00:00Z`, 105 runs, duration = `updated_at - run_started_at` (queue time not included).
- Thread messages with timestamps: "Let the factory build itself" (Mac) and "Let the worker reach previews" (fixes).

All times are Zurich (UTC+1). Durations are differences of these timestamps; where two activities overlap, the one that finished later is counted.

## Timeline

| Time | What happened | Bucket |
|---|---|---|
| 12:07–12:10 | Mac round 1: V1 fails (tenant has no Verifier, fails closed), V2 fails (worker cannot reach a preview on 127.0.0.1) | Mac run |
| 12:10–12:21 | Auto mode refuses the coordinator's approval of a config change; the owner is asked and answers | owner |
| 12:22–12:32 | Mac round 2: V1 passes | Mac run |
| 12:32–12:45 | #474 (preview proxy, coded since 12:08) opened, CI green | fix |
| 12:45–13:26 | #474 waits for the owner's choice of egress shape (asked 12:36) | owner |
| 13:26–13:35 | Decision recorded, CI, merge | fix |
| 13:35–13:59 | Mac round 3: drill fails on subnet overlap (#475 fixed in parallel, 13:36–13:50); V2 fails three ways; V3 fails with 409 | Mac run |
| 13:59–14:30 | #476 and #477 coded, reviewed, merged | fix |
| 14:30–14:56 | Mac round 4: V1, V3, V2 pass | Mac run |
| 14:56–15:01 | V4 setup, the owner invites the bot | owner |
| 15:01–15:05 | Mac round 5: V4 reaches the PR; the card has no screenshots | Mac run |
| 15:05–15:58 | #478 (4 review rounds), #479, #480 coded and merged; the owner pressed Merge instead of Approve (15:26), so the release path stayed untested | fix |
| 15:59–16:25 | Mac round 6: upgrade, V1, V3 pass; V4 acceptance dies on `chown` | Mac run |
| 16:25–16:46 | #483 coded, reviewed twice, CI green | fix |
| 16:46–18:30 | #483 green and not merged; `main` moved at 17:14 (#485) and 17:33 (#486), the thread acted only when the owner asked (17:28, 18:24) | green, not merged |
| 17:26–18:33 | Mac session offline; the coordinator's message at 18:32:48 brought it back in 14 s | offline (hidden behind #483) |
| 18:33–18:42 | Mac round 7: upgrade, V4 idea sent | Mac run |

## Buckets

| Bucket | Minutes | Share |
|---|---|---|
| Fixing: code, independent review, CI | 127 | 32% |
| Green fix not merged, then Mac offline | 107 | 27% |
| Mac runs, upgrades included | 103 | 26% |
| Waiting for the owner | 57 | 14% |

CI is a small part of "fixing": 56 `verify` runs, median 2.0 min, longest 2.7 min, none failed; `codeql` median 1.9 min.

## Defects that only the Mac found

| # | Defect | Fix | Would a real-container CI run catch it |
|---|---|---|---|
| 1 | A tenant without a Verifier fails closed, `upgrade.sh` expects `released` | config, preflight in #474 | yes, preflight |
| 2 | The worker cannot reach a preview on the host's 127.0.0.1 | #474 | yes |
| 3 | Preview subnets inside Docker's address pools; the drill inherits the instance subnet | #475 | partly, depends on the runner's networks |
| 4 | A retried job attempt gets 409 from the toolchain service | #476 | yes, with a worker kill step |
| 5 | The deployer's git refuses a mirror owned by another uid | #477 | yes |
| 6 | A failing preview retries forever | #477 | yes |
| 7 | Agents get no proxy to the preview | #477 | yes |
| 8 | Acceptance under plan mode saves no screenshots | #479 | no, needs a real model |
| 9 | Merge pressed on GitHub does not release a verified solo task | #480 | no, a T4 scenario gap |
| 10 | The evidence folder needs `CAP_CHOWN`, which the worker lacks | #483 | yes |

Seven of ten come from the difference between the fake harness in a test process and the real worker container: its uid, its capabilities, its networks, its proxies. The existing CI `engine` job runs only `TestEngine` in the deployer package; `make drill` and the sandbox task run only inside `deploy/upgrade.sh` on the Mac.

## Speedups, ranked by time they would have saved today

Saved minutes are estimates from the timeline above, not measurements.

1. **Merge the moment a fix is green, in the same turn** (~104 min). #483 was green at 16:46 and merged at 18:30. Cause: the owning thread ended its turn and waited for a scheduled check, against the `finish-task` rule; and seven README PRs (#481–#487) moved `main` during the freeze, each one making the branch stale. Fix: turn on GitHub auto-merge on the PR once review is done (the gates stay the same, GitHub merges when they pass), and keep the freeze for docs PRs that do not record results. Cost: a `finish-task` change, no code.
2. **A compose smoke in CI** (~2–3 Mac rounds, ~100–150 min). Start the real stack from `deploy/docker-compose.yml` with the `fake` harness, take one task from intake to a preview and a release, and probe from the worker as the agent uid: the preview URL, the evidence folder, the mirror, the proxies. Add `make drill`. It would have caught defects 2, 4, 5, 6, 7 and 10 before the first Mac round. Cost: a plan item, and CI time for building the worker image (not measured), so run it only when `deploy/`, the runner or the worker image change, plus nightly.
3. **Run every independent V stage per round and stop at the end, not at the first failure** (~1 round, ~50 min). Round 1 stopped after V1 and V2; V3's 409 showed up only in round 3. Cost: a line in the stream's rules; the "same failure twice stops" rule stays.
4. **Ask the owner everything a run needs before it starts** (~50 min). The Verifier provider, the egress shape, the bot invite and the sandbox recipe were each asked while a run waited. Put them in V0 as one batch. Cost: none.
5. **A short path for upgrades between fixes** (~10–15 min per round). Each fix reruns the whole `upgrade.sh` (drill and sandbox task) and every V stage after the touched one; round 6 took 22 min before V4 began. Rebuild only the changed service and rerun the stage that failed, and run the full upgrade once on the final tag. Cost: weaker evidence per round; the final full pass keeps the guarantee.
6. **Wake the Mac thread right after each merge** (hidden today, ~1 h otherwise). The session was offline 17:26–18:33 and came back 14 s after a message. Cost: one line in the coordinator's loop.

## Not checked

- Why the Mac session went offline at 17:26 (sleep, app, or limit): not visible in the thread.
- What the fix thread did between 16:46 and 17:14 (review or idle): no message in that span.
- CI queue time and the cost of building the worker image in CI.
- Whether the README PRs #481–#487 had an owner exception to the freeze.
