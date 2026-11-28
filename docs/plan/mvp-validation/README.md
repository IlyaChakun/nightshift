# Stream — MVP validation on the owner's Mac

Owned by the session that runs the validation; another session proposes an item as a new file here. Rules and limits: `docs/plan.md`. Source: the owner, 2026-11-28: before any new development, write down what must be true at the end of phase 6 and prove it live on the Mac; new work starts only after that, as a separate step.

Done when: every item marked **MVP** below has `passed` with evidence in its entry in `docs/plan/live-checks.md` or `docs/live-checks/`, on one tagged commit of `main`, and the phase 6 `README.md` records its Done criterion as met.

## Status

- Last done (2026-11-28, c3ce144, `factory-stable-2026-11-28.3`): V1, V2 and V3 passed. V1: `deploy/upgrade.sh` green, drill ok, sandbox #60 through the preview to `release_done` (`V1-upgrade-c3ce144.txt`, `V1-task60.txt`); the A7 results are from the 912a627 run, not rerun. V2: the worker and the agent's Chromium reach the preview and nothing else (`V2-worker.txt`); escapes refused, an unhealthy release rolled back (`V2-escapes.txt`, `V2-release.txt`). V3: a killed worker's job ran again once and the task was released (`V3-worker-death-2.txt`); a stopped worker kept the job's reason (`V3-G5-stop.txt`). The evidence card is checked in V4 (a CLI task has no chat thread).
- Next: the fix that records a stopped job as `job_failed` and quiets Chromium changes the tag: rerun V1 and V3 on its commit, then V4 after P5b.
- Decided (owner, 2026-11-28): a rollback a person starts is not part of the MVP; phase 6 proves only the automatic rollback (E6), and the manual one is in design section 23, "Later".

## Freeze rule

From the merge of this file until every **MVP** item has passed:

- Only these merge: phase 6 P5b; a fix for a failure that a V item found, with its regression test, linked from that item; doc updates that record results.
- Nothing else starts or merges: no new item in any phase or stream, no parked research PR, no refactor. Open streams (architecture A7, the go-quality live checks aside) wait.
- A fix changes the tagged commit: rerun `deploy/upgrade.sh` on the new commit (it reruns the drill and a sandbox task) and then every V item at or after the first one the fix touches.
- An exception needs the owner's words in the thread.

After the last **MVP** item passes, the phase 6 thread closes the phase, and new work starts in fresh threads in plan order.

## End result: what is true at the end of phase 6

Each line is the claim, then its repeatable check. Evidence files go to `~/nightshift-evals/<date>/` on the Mac, never into the repository.

| # | Claim | Check |
|---|---|---|
| E1 | The stack upgrades, restores and isolates on the Mac | `deploy/upgrade.sh` ends green on the tag (it runs `make drill` and one sandbox task to `released`); V1 |
| E2 | The worker cannot reach the book, trusted Git or Grafana without a password | the commands of `docs/live-checks/A7-worker-book-isolation.md` and `A7-grafana-login.md`; V1 |
| E3 | The deployer brings a preview up on a real engine, refuses escapes, rolls an unhealthy release back, and the worker reaches the preview | entry "Deployer on a real Docker engine" in `docs/plan/live-checks.md`, plus a fetch of the preview URL from the worker; V2 |
| E4 | A job whose worker stops or dies is recorded and runs again once | entry "Worker death sweeps leases", `docs/live-checks/G5-shutdown-reason.md`; V3 |
| E5 | A Slack idea ends in a released change: Russian cards, Approve, Builder, checks, Verifier, preview, acceptance, an evidence card with the preview URL and screenshots, the owner's approval, release, preview torn down | `nightshift status --task <id>`, Slack screenshots, `docker compose ls` before and after; V4 |
| E6 | A failed production smoke rolls back on its own, a revert pull request takes the bad commit out, and the next release does not ship it | `docs/live-checks/P6c-revert-after-rollback.md`; V5 |
| E7 | The owner sees the night in one readable Slack digest | entry "Morning digest in Slack"; V6 |

## Validation run

Run by the Mac Remote Control thread, in this order: each stage needs the one before it. **MVP** items block the end of phase 6; *should* items run in the same pass because they cost little there, and a failure in one becomes a plan item without stopping the run.

- [ ] V0. Prepare (no model cost). Copy `deploy/secrets/` out of the checkout first (a re-clone deletes it). Confirm the sandbox repository has a `compose-local` recipe with a healthcheck, a spec with hidden scenarios, and one earlier healthy release; add them in a sandbox pull request if missing. A test tenant sets `people.owner.chat` and no quiet hours. Export `NIGHTSHIFT_DEPLOY_DIR` to a folder under the home folder. Evidence: `v0.txt` with the file list and tenant config. **MVP**
- [x] V1. Base stack on the tag. `deploy/upgrade.sh` on the current `main` (E1). Then the A7 checks (E2). *Should*: `make docker-lint` and `make verify` on the Mac (entry "`make docker-lint` on an arm64 Mac"); the upgrade's one-shot services. **MVP**
- [x] V2. Deployer. Turn the `deploy` profile on (`docs/runbook.md`, "Previews and releases on this machine"), then the entry "Deployer on a real Docker engine" (E3). First check the known risk from `docs/live-checks/P5a-acceptance-leg.md`: the worker may not reach a preview on the host's 127.0.0.1 through its egress proxy. If it cannot, stop: that is a code fix before V4. Failed 2026-11-28 on `factory-stable-2026-11-26.5` (no route, egress denied); fix: `egress-preview` (ADR 0028), rerun V1, then `docs/live-checks/V2-worker-reaches-preview.md`. The second run failed too (mirror ownership, unbounded preview retry, no preview proxy for agents; fixed in #477). Passed 2026-11-28 on c3ce144; the checks of the #477 fixes in `V2-worker-reaches-preview.md` (steps 5–7) stay open and run with V4. **MVP**
- [x] V3. Recovery (E4): the entries "Worker death sweeps leases" and `G5-shutdown-reason.md`. *Should*: `A04-attempt-isolation.md` and "Side effects repeat safely on real GitHub". **MVP**
- [ ] V4. The golden path, phase 6 P5c, after P5b is merged and V1 reran on its tag (E5). One idea in Russian in Slack. It covers the entries "Lead card labels in the chat language", "Approve hands the spec to Builder", `G8-standards-readable.md`, `A06a-single-verifier-finding-writer.md`, `P5a-acceptance-leg.md` and `P6a-preview-teardown.md`, `V4-human-merge.md` (both the Approve and the Merge path), and P5b's card. *Should*: `A06b-finding-handoff-status.md` and `A06b-check-evidence.md` (local checks) if the Verifier returns a finding. **MVP**
- [ ] V5. The failure path (E6): `P6c-revert-after-rollback.md`. *Should*, on the same tenant: "Quiet hours hold a release" (a `compose-local` release is the release target it lacked) and `P6d-preview-hours.md` with `preview_hours: 1`. **MVP**
- [ ] V6. The owner's view (E7): `nightshift digest <tenant> --send` the next morning covers V4 and V5. *Should*: "Alert counts in the digest" and "Plan folders in the digest" from the same send. **MVP**

## Not needed for the MVP

They stay pending in `docs/plan/live-checks.md` and run after the MVP, in plan order. Each needs something the MVP does not: an account or token from the owner, an unbuilt item, or a staging stack with fault injection.

- Owner's account or token: "Codex on the owner's ChatGPT plan", "Supervisor on `codex`", "Tracker projection" (`5-tracker-approval.md`).
- Waits for unbuilt work: "Remote MCP door" (doors I6).
- Isolated staging with injected faults: `A01a-required-checks.md`, `A01b-approval-follows-patch.md`, `A01b-review-freshness.md`, `A01r-resume-evidence.md`, `A02a-upgrade-recovery.md`, `A03a-durable-dispatch-decisions.md`, `A03b-atomic-verdict.md`.
- Features off by default or switched on per tenant: "SLA auto-merge", "Review reminder", "Review reminder without a chat thread", "Independent reader chain", "Problem report reproduction", "Several tenants", `L3a-lead-task-schedule.md`, "Lead uses the book search and the project memory".
- Eval runs that cost model tokens: `L7-cache-tokens.md`.

## What the owner does by hand

Everything else is the Mac thread's work. Five things need the owner:

1. Once, before V0: revoke the Slack configuration token that was shown in chat (`docs/status.md`, "Blockers"), in the Slack app's settings.
2. During V1 to V6: keep the Mac awake with Docker Desktop running.
3. In V4: write the idea in Slack in Russian and press Approve on the spec card. A person's click is what the check tests.
4. In V4 and V5: approve the pull request on GitHub as the reviewer (the factory's App cannot approve its own pull request; this is the "Merge" of the Done criterion). In the V4 rerun, approve one task and press Merge on another (`docs/live-checks/V4-human-merge.md`).
5. In V6: read the digest and say whether it reads well.

If Docker Desktop does not share the folder in `NIGHTSHIFT_DEPLOY_DIR`, the thread asks for one setting in Docker Desktop (unverified whether a folder under the home folder is shared by default).

## Rules for this stream

- One stage at a time; record `passed` or `failed` with evidence in each covered entry as it runs, in place.
- A failure gets a fix item in the plan folder that owns the code, then the rerun rule above. A stage that fails the same way twice stops the run and goes to the owner.
- No durations or end times are promised for any stage; the record holds the clock times that happened.
