# Builder and Verifier can read the standards their checklist names

Plan item G8 of `docs/plan/go-quality/`; source: finding F3 of `docs/research/go-quality-audit.md`. The owner chose on 2026-11-27 to give the agent `/standards` read-only.

Run on a real stack with the `claude` harness: queue a Builder job on a synthetic Go task (`example-solo`). Expected: the job's checklist line reads `Go style (go.style), details: /standards/industry/go/style.md`, the agent's transcript shows it read that file without a permission error, and a write to `/standards` fails (the files belong to root). Repeat with `codex` when its live check runs.

The unit tests `TestAgentCanReadTheStandards` (runner, fake harness) and `TestChecklistPathsAreInTheWorker` (repository files and the Dockerfile) cover the logic only.

Status: pending
