# Why one long thread is the wrong unit of work (2026-11-26)

## Summary

The owner saw the thread "Build factory phases 2 onward" at the top of the usage statistics and idle. Both are explained, but not by context rot.

- It is idle because it ended its turn at 2026-11-25 20:54 UTC with nothing to wake it. It has been silent since (about 15 hours at the time of writing). Its own plan items were still open.
- It leads the statistics because it did the most work (phases 2 to 5, PR #44 to #257 in 27 hours), not because each unit of work cost more. Cache reads per output token are about the same as in other threads (table below).
- Quality did not fall with age in any way I could measure. The misses were of three other kinds: it stopped without a wake (five idle gaps), it called things done too early, and many threads edited the same shared files at once.

Verdict on the owner's idea: yes with changes. Short threads that end at a merged item are right, for cost, liveness and ownership, not for context. A hard "never in parallel" is too strict: a phase tail that waits for the Mac blocks the next phase for hours. The rules below make phases sequential by default and parallelism an exception that is named.

## What was measured

Source: the session records (`get_session`) and the thread's messages, read 2026-11-26 12:30 to 12:50 UTC. `cost_usd` is the record's own field; on a subscription it is notional, so use it only to compare threads.

| Thread | Wall clock | cost_usd | Cache reads | Output tokens | Cache reads per output token |
|---|---|---|---|---|---|
| Build factory phases 2 onward | 27 h | 91.8 | 558 M | 1.40 M | ~400 |
| Configure and run many tenants (Opus) | 4.7 h | 53.4 | 121 M | 0.43 M | ~280 |
| Evaluate Temporal and factory techniques (Opus) | 10 h | 52.1 | 192 M | 0.51 M | ~375 |
| Finish phase 5 (running) | 4 h | 32.5 | 182 M | 0.54 M | ~337 |
| Start phase 6 (running) | 4 h | 19.3 | 103 M | 0.34 M | ~299 |

The long thread's record shows at least one compaction and, at the end, 104k of 500k tokens in use. Its active API time is reported as about 3 hours of the 27 (`duration_api_ms` of its last result event; the meaning of that field was not verified). Most of the 27 hours it was waiting, not working.

## Where it lost time

From the thread's messages. Five gaps with no work, each ended by the coordinator, or by nobody:

- 00:21 to 01:15: shown as blocked with no question. The thread said it was not blocked.
- 01:18 to 02:46: idle; the coordinator had to list the next items.
- 09:05 to 13:15: idle for 4 h 10 min; the coordinator sent the owner's goals again.
- 15:13 to 15:41: idle. The owner's rule "never end a turn just to wait for CI" came right after.
- 20:54 to now: the coordinator had told it to merge what was open and write the next step, and promised a wake after the limit reset. The thread merged #255 and #257 and ended with "nothing further to act on". No wake existed; `list_triggers` held only the retro and the weekly audit (the project file `retro/2026-11-26-why-not-finished.md`). The earlier retro already added the rule "stop on a limit only with `send_later`"; this one adds why the thread was long enough to need it.

Two limit stops on 2026-11-24 (19:32 and 20:25, `seat_org_spend_limit`) cost the rest of that evening.

## Where it was wrong

- Announced #103 as merged before it was (09:01); corrected itself at 09:04.
- Marked phases 1 and 2 done in #125 without the restart and restore checks. The audit thread found it at 17:16 on 25 November; `plan.md` still said phase 3.
- Defects found later by someone else, from early code as well as late code: the dependency check that ran git as root (#168, phase 4), the task text cut at 120 characters and a return job that lost the previous patch (phase 3, found live), and three more issues in the outside-merge watch (#213). That is the pattern of work checked only on fakes, not of a thread that got worse with age.

I did not read the full transcript, only its reports and events. A context-rot effect that never shows in the reports cannot be excluded; it is not demonstrated.

## Today's collisions between parallel threads

- `main` went red when two PRs, each under the word limit alone, merged within minutes: #321 added a line to `AGENTS.md`, #319 a plan line (#327 fixed it). The `main` ruleset has `strict_required_status_checks_policy: false`, so a PR need not be up to date to merge (file `security/main-ruleset.json` in the project files; the live setting was not checked).
- A sweep thread pushed into another thread's PR and broke its CI (#321 text). The rule now exists.
- `docs/design.html` on `main` held conflict markers after #219 (#231 fixed them); two PRs took migration 00015 (#297); ADR numbers were taken twice; #219, #253 and #272 conflicted in `status.md`, `decisions.md`, `plan.md` and `design.html` ([idle PRs](2026-11-26-idle-prs.md)).
- At 12:32 UTC nine sessions were in state "working" (`list_threads`), against a cap of three build threads. I did not check which of the nine change the repository.

The plan split (#323) removes most of the `plan.md` conflicts. What remains is `AGENTS.md`, `status.md`, `decisions.md` and `design.html`, which every thread can touch.

## Does the factory have the same problem

Checked in code, 2026-11-26 (`internal/dispatch`, `roles/`, ADR 0019):

- Lead starts a fresh session for every owner message. The prompt is the whole thread transcript plus the stored handoff (`lead.go`), limited to 20 minutes and 200k tokens (`roles/lead.yaml`). The transcript has no cap, so a very long thread grows the prompt.
- Builder and Verifier resume one harness session across their returns (`build.go`, `router.go`: `--resume`). That is bounded by `max_returns` (3) and by the per-job budget, but the supervisor's token rule leaves out cache reads and writes (ADR 0019), so nothing caps how much context a resumed session carries. A handoff happens only when the session cannot continue (design section 12).
- The conductor is Go code, not a session.

Risk: low today, and growing with the length of the tasks. Plan item L6 adds a context guard (fresh session from the handoff past a size; a summary instead of an uncapped transcript). The limit is not known yet and is set from the Mac live run.

## Rules

Applied in this PR: `docs/plan.md` ("How threads are run"), skill `finish-task` ("Thread size and handoff"), one sentence in `AGENTS.md`, plan item L6, decision 18.

1. One thread works one phase file or stream and ends when its batch is merged.
2. Hand off when any trigger fires: about 4 hours, about 10 merged PRs, context near half the window (about 250k tokens), a usage-limit stop, or a phase boundary. The numbers are chosen, not measured: two healthy threads sit at 252k and 260k after 4 hours. Tune them with the daily retro.
3. A handoff is a rewritten "Status" block plus a brief of at most 10 lines to the coordinator. It is not the transcript. The coordinator starts the next thread or schedules a wake in the same turn.
4. Phases run in order. A later phase may start early only when the earlier phase has only live checks left, and the plan page names the exception (today phase 6 beside 5, decision 16).
5. At most two threads change the repository at once, on files the briefs name as disjoint. Research, review and live-check threads do not push to another thread's branch.

## Not verified

- That the triggers (4 h, 10 PRs, 250k) are the right numbers.
- The live `main` ruleset (decision 18 depends on it).
- Whether a thread started from a brief loses facts a long thread kept. Watch this in the next daily retro: count re-asked questions per new thread.
- The `cost_usd` figures in dollars; they are not a bill.

## Chat skills and project instructions

The synced copies are read-only; the owner saves these (a skill card was posted in the thread).

- `task-kit`, step 5: add "A brief is at most 10 lines: goal, files owned, next item, done criterion, what not to touch. A new phase or stream gets a new thread; a thread past a trigger in `docs/plan.md` ("How threads are run") gets no new work."
- `nightshift-phase-status`, step 2: add "For each running thread give its age and context use (`get_session`: `created_at`, `context_usage.used_tokens`) and name any that is past a trigger."
- `autonomous-merge-gate`: add before "After merging": "Fetch `main` right before the merge. If it moved since CI started and the PR touches `AGENTS.md`, `docs/status.md`, `docs/decisions.md` or `docs/design.html`, run `scripts/repo-check.sh` on the merged result: two green PRs can turn `main` red."
- Project instructions, "Parallel work within a budget": replace "at most 3 build threads" with "at most 2 threads change the repo at once, on disjoint files; phases run in order" and add "a thread ends at a merged item and hands off by a brief".

Basis. Checked: `get_session` of five threads, the long thread's messages and last 23 events, `docs/plan.md`, `AGENTS.md`, `finish-task`, `roles/lead.yaml`, `internal/dispatch/{lead,build,router}.go`, ADR 0019, PR #321 and #327, `security/main-ruleset.json`. Inferred: why quality did not visibly fall; that the thresholds are reasonable. Confidence: high on the timeline and the numbers, medium on the conclusion about context.
