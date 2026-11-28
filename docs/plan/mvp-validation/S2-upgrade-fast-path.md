# A short upgrade path between validation fixes

State: proposed

Source: `docs/research/mvp-validation-timeline.md`. The freeze rule reruns the whole `deploy/upgrade.sh` (drill and sandbox task) and every V stage after the touched one for each fix; round 6 on 2026-11-28 took 22 min before V4 started.

Scope: a documented `upgrade.sh` mode, or runbook steps, that rebuilds and restarts only the services a fix changed and reruns only the failed V stage. The full upgrade and every V stage run once on the final tag, so the end evidence stays the same. The stream's freeze rule names when the short path may be used.

Done when: one fix round on the Mac uses the short path, its time is recorded from timestamps, and the final tag still passes the full run.
