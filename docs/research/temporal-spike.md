# Temporal spike

Format: `docs/research/README.md`. Decision: ADR 0018 ("Experiment result"). Run 2026-11-26 between 08:11 and 08:20 UTC (about 9 minutes of wall-clock time, from the first command to the opened PR; the tokens and the shallow scope follow from that), no paid service, no new egress host for the factory. The spike code stays on the branch `spike/temporal-experiment` (commit 35bb0bf); it is never merged.

## What was built

One task flow as a Temporal workflow: Lead, Builder, Verifier, `OpenPR`, then a wait for a human signal with a 24-hour timer. The activities call the real `harness.Harness` interface (the `fake` harness replaying `testdata/scenarios/happy-path.yaml`) and `integration.PullRequests.OpenPR` (a test double that appends to a file, so a repeat is countable). Server: `temporal server start-dev` (CLI 1.10.1, server 1.32.2, UI 2.56.4) with a SQLite file. Go SDK 1.52.0. Code: `cmd/spike-temporal/main.go`, 195 lines including the worker, a start/signal/result client and fault injection by files.

## Results

| Question | Result | Evidence |
|---|---|---|
| Does the happy path run through the existing interfaces? | Yes, no change to `Harness` or `PullRequests` | workflow A completed; `OpenPR` called once |
| Retry config | Works as configured: backoff 1 s, 2 s, maximum 4 attempts | workflow B2: Builder failed twice (injected), attempt 3 passed |
| Kill the worker (`kill -9`) in the middle of a Builder run | The workflow resumed on the new worker; Lead did not run again; Builder ran again from the start as attempt 2, 9 s after the kill | workflow C2 |
| Kill the worker while the workflow waits for a human | Nothing re-ran; the signal sent after the restart was delivered; `OpenPR` was not repeated | workflow E |
| Restart the server | History and state survived (SQLite file) | `workflow list` after restart |
| Stall detection | A heartbeat timeout of 15 s cut a Builder that sent no heartbeat; four attempts, then the workflow failed with the cause | workflow D, UI screenshot showed "4 retries" |
| Audit and debugging | The event history is complete for scheduled, started, completed and signaled events, and the UI has a timeline. **The errors of failed attempts are not in the history**: for B2 the history has one `ActivityTaskScheduled`, `Started`, `Completed` for Builder; the two injected failures are only in the worker log. The UI shows a retry count and the last failure | `workflow show --workflow-id B2` |

## What the spike showed that reading did not

- **Recovery works as advertised, and costs a whole turn.** A killed worker is noticed by a heartbeat or start-to-close timeout, then the activity starts again from its first line. For a real agent that re-spends the tokens of the turn. Resuming needs the session id in the heartbeat details and an adapter that can continue a session (ADR 0005 handoff already does this for returns).
- **The stall detector is the useful part, and it needs S1.** Heartbeat timeout is a ready stall control, but something must heartbeat when the agent produces output. That is exactly the normalized event stream of plan S1; with it, our runner gets the same control without Temporal.
- **The fake harness is not repeatable.** The stock fake keeps its cursor in memory, so after a restart it served the wrong step. The spike had to build a per-role copy. Activities must be repeatable by construction; our twins and the book already assume that (R2).
- **Retried attempts leave no trace in the history.** Our own timeline (plan S2) can record every attempt and its error as an event, which the Temporal history does not. For "why did this task take three tries" the book is better.
- **Two records.** Every task fact the owner reads (`nightshift status`, the digest, Grafana) lives in the book. The workflow history would be a second record to keep in step; the spike did not write to the book at all, so it did not test that cost.

## Cost

- **Code.** 195 lines for four steps and a wait, with no book writes, no budget, no outbox, no handoff, no tenant, no cancel. The factory's conductor flows would be a rewrite (ADR 0018 estimates phases 2 and 3 again; this spike does not change that estimate, it only shows the first 5% is easy).
- **Services.** Development server: one process, 173 MB resident memory, a 125 MB binary, a 0.7 MB SQLite file after seven workflows. Not measured: a production-shaped cluster (four services and a database per the upgrade guide, ADR 0018).
- **Determinism and versioning.** Not tested. A workflow change while tasks are in flight is the cost ADR 0018 names, and the factory edits its own code. Reading only.

## Against S2 (a task timeline on our events table)

| Need | Temporal history and UI | S2 on the book |
|---|---|---|
| One view per task | yes, ready | to build (plan S2), small |
| Error of every attempt | no (last failure only) | yes, if each attempt writes an event |
| Same data as the digest, status, Grafana | no, a second record | yes, one record |
| Replay for debugging | yes (replay needs the workflow code and the same version) | no; scenarios replay on twins |
| Extra services | a server and a store | none |

## Not verified

- A real harness (`claude`, `codex`): the activity ran the fake only.
- Postgres persistence, several workers, Temporal Cloud.
- Replay of an old history against changed workflow code.
- The licence of the Go SDK (still unstated on the pages read).
- Nothing was written to the book from a workflow.

## Recommendation

No-go: keep the Go conductor on the book and River. Take S1 (stall, token and repeat controls from the harness event stream) and S2 (timeline with every attempt). The spike confirms recovery and retries work, but it does not remove a defect class the factory has, adds a second record of task state, and the part that is most valuable (stall detection by heartbeat) is built by S1 anyway. Revisit on the triggers in ADR 0018.
