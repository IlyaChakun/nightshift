# Spend fewer tokens per finished task

State: proposed

Source: `docs/research/token-economy.md` (the owner, 2026-11-26: tokens run out too fast for the factory to make sense). Spend is context size times model calls; cache reads are most of it and the budgets did not see them.

Done when: the book shows cache reads per job on the stable instance, and each item below is merged or dropped with the numbers that decided it.

- [x] L7a. `feat(harness): record cache tokens; no CLAUDE.md twice on the subscription` — `cache_read_tokens`, `cache_write_tokens` per job, OTel and `nightshift status --task`; `CLAUDE_CODE_DISABLE_CLAUDE_MDS=1` for `claude-sub`; T1. Live check: `docs/live-checks/L7-cache-tokens.md`. The Grafana task timeline (`book.dash_task_timeline`) does not show cache reads yet
- [ ] L7b. `feat(dispatch): fix a broken Lead reply in the same session` — `sendReplyBack` resumes the job's session with the reason instead of a fresh run; T1, T4
- [ ] L7c. `feat(supervise): a cache-read rule per run` — next to the token rule, off by default until L7a's numbers set it; pairs with L6; T1
- [ ] L7d. `chore(roles): Builder model and Lead effort from the polygon` — Lead `medium` while grilling; needs decision 20. Builder stays on the CLI default (owner, decision 19, 2026-11-28)
- [x] L7g. `docs(roles): small tool output` — one line in `roles/builder.md` and `roles/verifier.md`; the role evals it triggers run on the Mac (`docs/live-checks/L7-cache-tokens.md`)
- [ ] L7f. `feat(harness): one cache prefix across worktrees` — pass `--exclude-dynamic-system-prompt-sections` after a live run shows the pinned CLI accepts it and cache writes per job drop; T1, T8
- [ ] L7e. `chore(roles): max_returns from round data` — lower to 2 only if the third round rarely passes
