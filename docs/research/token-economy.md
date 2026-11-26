# Token economy: where the factory's tokens go and what to cut

## Summary

Spend grows with **context size × number of model calls**, not with the size of the prompt we write. Every tool call is one more model call that re-reads the whole context from the prompt cache. In the measured project threads that re-reading was 280 to 400 cache tokens per output token; in the one recorded factory run it was 110,281 of 111,358 tokens. The factory's budgets count only the remaining 1%, so nobody could see it.

So the order of work is: measure what each job really reads (landed), then make calls fewer, contexts smaller and models cheaper where the gates allow it. Ranked list below; items marked **landed** are merged with this document, **plan** are in `docs/plan/limits/L7-token-economy.md`, **owner** wait in `docs/decisions.md`.

## What was measured

| Source | Fact |
|---|---|
| Recorded Claude stream, `internal/harness/claude/claude_test.go` (Mac run, 2026-11-26) | 7 model calls: input 10, cache writes 8,159, cache reads 102,122, output 1,067. The budget counted 1,077. The first call alone read about 21k tokens (CLI system prompt, tools, role text) before any work. |
| `docs/retro/2026-11-26-long-threads.md` (session records, 2026-11-26) | Five project threads: 103M to 558M cache reads each, 0.34M to 1.40M output tokens; 280 to 400 cache reads per output token. |
| `/mnt/project-files/retro/retro-2026-11-26.md` | 25 threads were started on 2026-11-26; `list_threads` showed 83 in the project on 2026-11-26 22:35 UTC. |
| Files | `AGENTS.md` 10,969 bytes, `docs/design.md` 233,183 bytes, Lead's role text 9,578 bytes; about 2.7k, 58k and 2.4k tokens at ~4 bytes per token (estimate). |
| Prices (model table of the Claude API reference, cached 2026-11-03; cache multipliers from [prompt caching](https://platform.claude.com/docs/en/build-with-claude/prompt-caching), read 2026-11-26) | Opus 5.5: $4 input, $20 output, $0.20 cache read per MTok. Sonnet 5.5: $2, $10, $0.10. Cache writes cost 1.25× input (5 minutes). |

With those prices, the 558M cache reads and 1.4M output tokens of the longest thread come to about $112 and $28: re-reading context is about 80% of the bill (estimate from measured counts and list prices; the session record shows another figure because it ran on other models). How cache reads weigh against the subscription's usage limits is not documented ([Pro and Max plans](https://support.claude.com/en/articles/11145838-use-claude-code-with-your-pro-or-max-plan), read 2026-11-26): **unverified**, so the ranking assumes they count in proportion to price.

Not measured: the factory's real spend per role. The book has `usage.cost_usd` per job, but this cloud session cannot reach the Mac's book. The query is in the live check `docs/live-checks/L7-cache-tokens.md`.

## Ranked savings in the factory

Savings are estimates until L7's numbers exist.

1. **Count cache tokens per job (landed).** `cache_read_tokens` and `cache_write_tokens` in the job's usage, the OTel span and `nightshift status --task`. Saves nothing by itself; every other item needs it.
2. **No `CLAUDE.md` twice on the subscription (landed).** Runs without `--bare` loaded the worktree's `CLAUDE.md` (for this repository: all of `AGENTS.md`, plus the nested ones as files are read) on top of the instructions the runner appends. `CLAUDE_CODE_DISABLE_CLAUDE_MDS=1` stops it for a project that names its instruction files in `project.yaml`; a project that names none keeps the CLI's discovery ([environment variables](https://code.claude.com/docs/en/env-vars), 2026-11-26). About 2.7k tokens per model call on this tenant (estimate). It also makes a subscription run behave like an API run, as design section 12 intends.
3. **Builder on a cheaper model (owner, then plan).** Builder has no `model`, so it runs the CLI default. Sonnet 5.5 is half the price of Opus 5.5 per token, and the gates, not Builder's judgment, decide what merges. Run the A/B on the polygon (design section 20) before switching.
4. **Lead effort by step (owner).** Lead runs `high` on every chat message, including "thanks" and short clarifications. Proposal: `medium` while grilling, `high` for the spec turn. The owner chose high effort for Lead on 2026-11-26, so this needs the owner.
5. **Fix a broken Lead reply in the same session (plan).** A reply that breaks the chat rules starts a fresh Lead run with the whole transcript and repository exploration again (`sendReplyBack`, `internal/dispatch/lead.go`). Resuming the session to rewrite only the text is one call instead of a whole run.
6. **A context rule in the supervisor (plan, after L7).** A per-run limit on cache reads, next to the token rule. This strengthens a protection; the number comes from L7, not a guess. It pairs with L6 (context guard for resumed sessions and Lead's transcript).
7. **Fewer return rounds (plan, after L7).** Each return is a resumed Builder plus a resumed Verifier on a growing context. If L7 shows the third round rarely passes, lower `max_returns` from 3 to 2 and escalate sooner.
8. **A smaller tool list for read-only roles (later).** `--tools` sends fewer tool definitions ([CLI reference](https://code.claude.com/docs/en/cli-reference), 2026-11-26); `--allowedTools` only pre-approves. Small per call; only after L7 shows the base context matters.

## Ranked savings in how we work on this project

The same rule applies to the Claude sessions that build the factory, and there the volume is far larger (hundreds of millions of cache reads per long thread).

1. **One blocking wait, not polls (landed in `finish-task`).** Every poll of CI is a model call that re-reads the whole thread. `scripts/pr-gate.sh wait` or a PR subscription waits in one call.
2. **Read sections, not whole files (landed).** Reading all of `docs/design.md` puts ~58k tokens (estimate) into every later call of the thread. Find the heading with `grep -n '^## '` and read that section.
3. **Batch independent tool calls in one message (landed).** Three reads in one call cost one context re-read, not three.
4. **Hand off sooner (owner).** `docs/plan.md` hands off near ~250k tokens of context, a chosen number. Cost per call grows linearly with context, while a fresh thread starts at a few tens of thousands (estimate). Proposal: ~150k.
5. **Fewer threads and posts (owner).** 25 threads in one day, and hourly night-watch sessions that each start a new context. Proposal: one thread per phase file as the plan already says, the watchdog every 3 hours, status answers only when asked.
6. **Cheaper model for routine threads (owner).** Threads default to Opus with a 1M window. Relays, status, docs-only PRs and Mac live-check briefs could run on Sonnet 5.5 at half the price; design, ADRs and independent review stay on Opus.

## Counter-argument

A cheaper model or lower effort can cost more if it needs more rounds: cost per finished task matters, not per call. That is why items 3, 4 and 7 wait for L7 and the polygon instead of switching now.
