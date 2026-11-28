# Run the real compose stack in CI before the Mac

State: proposed

Source: `docs/research/mvp-validation-timeline.md`. Seven of the ten defects the Mac found on 2026-11-28 came from the real worker container (its uid, capabilities, networks and proxies); CI starts only the deployer on a real engine (`make engine-test`), and `make drill` and the sandbox task run only inside `deploy/upgrade.sh` on the Mac. Each such defect cost a fix and a full Mac round.

Scope: a CI job that starts `deploy/docker-compose.yml` with the `fake` harness and the `deploy` profile, runs `make drill`, takes one task from intake through a `compose-local` preview to `released`, and probes from the worker as the agent uid: the preview URL through `egress-preview`, a write to the acceptance evidence folder, a read of a mirror owned by another uid. One step kills the worker mid-job and expects the retry to finish. It runs when `deploy/`, `internal/execution/runner/`, the worker image or `scripts/drill.sh` change, and nightly.

Done when: the job fails on a branch that reverts #474, #477 or #483 and passes on `main`; its run time is measured and recorded here.

Priority: first of the code items in this folder. It would have caught defects 2, 4, 5, 6, 7 and 10 of the timeline before the Mac (estimate).
