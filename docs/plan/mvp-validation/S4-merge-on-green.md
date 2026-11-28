# Merge a fix the moment it is green

State: proposed

Source: `docs/research/mvp-validation-timeline.md`. #483 was green at 16:46 Zurich on 2026-11-28 and merged at 18:30: the owning thread ended its turn on a scheduled check, and `main` moved twice under it.

Scope:

1. `.agents/skills/finish-task`: merge in the same turn as the green check; never end a turn on a green PR (done in the PR that adds this file).
2. GitHub auto-merge, so a PR merges when its required checks pass even if the thread is idle. The repository has `allow_auto_merge: false` (`GET /repos/.../`, 2026-11-28); turning it on is a repository setting, so it is decision 23 in `docs/decisions.md`. The required checks and the review stay as they are.

Done when: decision 23 is answered, and if yes, the next validation fix merges through auto-merge.
