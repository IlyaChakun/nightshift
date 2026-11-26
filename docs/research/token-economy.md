# Token economy: where the tokens go and what to cut

## Summary

Spend grows with **context size × number of model calls**. Every tool call is one more model call that re-reads the whole context from the prompt cache. Measured here: the project's 80 Claude sessions of 2026-11-24 to 2026-11-26 read 2,419M tokens from the cache for 7.7M output tokens (315 to 1), and the one recorded factory run read 110,281 of its 111,358 tokens from the cache. The factory's budgets counted only the rest, so nobody saw it (fixed in #393).

What cuts spend, in order of evidence: keep each context small (short sessions, old tool output dropped, small tool output), make fewer calls (no polling, batched calls), put routine work on a cheaper model behind the same gates, and keep the static prefix small and stable so the cache hits. Multi-agent fan-out and LLM-written summaries cost more, not less.

Ranked actions: [factory](#ranked-actions-for-the-factory) and [this project](#ranked-actions-for-how-we-work). **Landed** means merged, **plan** means `docs/plan/limits/L7-token-economy.md`, **owner** means `docs/decisions.md`.

## What was measured

| Source | Fact |
|---|---|
| 80 session records of this project (`get_session`, read 2026-11-26 22:55 UTC; 4 of 84 not found; table in the project files, `research/token-economy/sessions.csv`) | $655.7 notional `cost_usd`; cache reads 2,419M, cache writes 40M, input 1.9M, output 7.7M. Seven sessions are 54% of the spend; 47 sessions under $3 are 10%. By model: Sonnet 5.5 48 sessions $297, Opus 5.5 25 sessions $241, Fable 5.2 7 sessions $117. By start day: 23, 25 and 32 sessions. |
| Same records, two night-watch sessions | A fresh session is at ~93k tokens of context after two or three calls ($0.46 each): that is the fixed price of starting a thread here. |
| This session at 22:52 UTC | 23.1M cache reads, 78k output, 289k tokens of context; the account's seven-day limit was at `allowed_warning`. |
| Recorded factory run, `internal/harness/claude/claude_test.go` (Mac, 2026-11-26) | 7 calls: input 10, cache writes 8,159, cache reads 102,122, output 1,067. The first call read ~21k (CLI system prompt, tools, role text). |
| Files | `AGENTS.md` 10,969 bytes, `docs/design.md` 233,183 bytes: ~2.7k and ~58k tokens at ~4 bytes per token (estimate). |

At Opus 5.5 list prices the project's tokens cost about $484 for cache reads, $202 for cache writes and $154 for output: context is about 80% of the bill (estimate: the sessions ran on three models, so the records' own total differs). How cache reads weigh against the subscription's limits is not documented beyond "cached … counts less" ([usage limit best practices](https://support.claude.com/en/articles/9797557-usage-limit-best-practices)); the weight is **unverified**.

Not measured: the factory's spend per role on the stable instance. The query is in `docs/live-checks/L7-cache-tokens.md`.

## What the sources say

Read by a research agent on 2026-11-26; the cache prices were checked twice, the other numbers were not re-checked.

- **Prices.** Opus 5.5: $4 input, $20 output, $0.20 cache read per MTok; Sonnet 5.5: $2, $10, $0.10. A 5-minute cache write costs 1.25× input, a 1-hour write 2× ([prompt caching](https://platform.claude.com/docs/en/build-with-claude/prompt-caching)).
- **The cache is an exact prefix** (tools, system, messages). A model switch, new tools or MCP servers, compaction or a CLI upgrade costs one full re-read. On a subscription `claude -p` gets a 1-hour TTL, subagents 5 minutes. The cache is per directory, so separate worktrees miss each other unless `--exclude-dynamic-system-prompt-sections` is set ([Claude Code prompt caching](https://code.claude.com/docs/en/prompt-caching)).
- **Claude Code's own advice**: costs scale with context; clear between tasks; Sonnet by default, Haiku for simple subagents; filter test and log output before it enters context; keep `CLAUDE.md` under 200 lines; agent teams use ~7× the tokens ([costs](https://code.claude.com/docs/en/costs)).
- **Dropping old tool output works.** On SWE-bench Verified, masking observations older than 10 turns halved cost at an equal or better solve rate; LLM summaries made runs 13–15% longer and cached badly ([JetBrains, The Complexity Trap](https://arxiv.org/html/2508.21433v3)). Anthropic measured −84% tokens with context editing on a 100-turn task ([context management](https://claude.com/resources/articles/context-management)). OpenHands' condenser: up to 2× cheaper per turn, cost linear instead of quadratic, solve rate 54% vs 53% ([OpenHands](https://openhands.dev/blog/openhands-context-condensensation-for-more-efficient-ai-agents)).
- **Smaller contexts are also better answers**: reliability falls as input grows, on all 18 models tested ([Chroma, context rot](https://trychroma.com/research/context-rot)).
- **Load on demand.** Cursor moved long tool output to files and loaded MCP tools and skills on demand: −46.9% tokens in an A/B ([dynamic context](https://cursor.com/blog/dynamic-context-discovery)); Aider sends a 1k-token repository map instead of files ([repo map](https://aider.chat/docs/repomap.html)).
- **Fan-out is expensive**: agents use ~4× and multi-agent systems ~15× the tokens of chat ([multi-agent research](https://www.anthropic.com/engineering/multi-agent-research-system)). Subagents pay when they keep big reads out of the main context and return 1–2k tokens ([context engineering](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents)).
- **Cheaper model, same gates.** Sonnet with an Opus advisor scored +2.7 points at −11.9% cost per task against Sonnet alone ([advisor strategy](https://claude.com/blog/the-advisor-strategy)); on HAL's SWE-bench Verified Mini, Sonnet 4.5 beat Opus 4.1 at a third of the cost ([HAL](https://hal.cs.princeton.edu/swebench_verified_mini), costs may be overstated). No measurement for our roles exists yet.
- **Handoff instead of compaction**: Amp starts a focused thread from a goal and a file list ([handoff](https://ampcode.com/news/handoff)), as our `finish-task` handoff does.

## Ranked actions for the factory

Savings are estimates until L7's live check has numbers.

1. **Count cache tokens per job (landed, #393).** Needed for every item below.
2. **No `CLAUDE.md` twice on the subscription (landed, #393)**, for projects that name their instruction files: ~2.7k tokens per call on this tenant (estimate).
3. **Small tool output in Builder and Verifier (landed).** One line in `roles/builder.md` and `roles/verifier.md`: search before reading, read parts, show only failures. Claude Code and Cursor report the largest savings here; it triggers the role evals (`evals/selection.yaml`).
4. **Fresh session from the handoff instead of a long resume (plan, L6).** A return round resumes Builder and Verifier with every old tool result. Starting fresh from the handoff, the worktree diff and the findings is the coarse form of observation masking, the best-measured technique above.
5. **Builder on Sonnet 5.5, Opus as reviewer (owner, decision 19).** Half the price per cache read; the gates decide what merges. A/B on the polygon first.
6. **Lead effort by step (owner, decision 20).** `medium` while grilling, `high` for the spec.
7. **Fix a broken Lead reply in the same session (plan, L7b)** instead of a fresh run that explores the repository again.
8. **A stable prefix across worktrees (plan, L7f).** Every job runs in its own worktree, so the CLI's system prompt is written to the cache again for each job. `--exclude-dynamic-system-prompt-sections` is documented for this; it needs a live check on the pinned CLI before the adapter passes it.
9. **A cache-read rule per run and data-driven `max_returns` (plan, L7c, L7e).**

## Ranked actions for how we work

1. **Keep thread contexts small.** Cost per call is proportional to context; a thread at 300k pays about three times what a fresh one pays per call (~93k). Hand off near ~200k (owner, decision 21), read sections not whole files (landed in `finish-task`), and filter command output (landed).
2. **Make fewer calls.** One blocking wait for CI, not polls; independent reads in one message (landed in `finish-task`).
3. **No Fable by default.** Seven Fable sessions were 18% of the spend. The default is Opus 5.5 since 2026-11-26 (`docs/decisions.md`, "Taken").
4. **Sonnet 5.5 for routine threads (owner, decision 21).** Relays, status, docs-only PRs, Mac briefs; design, ADRs and independent review stay on Opus. Sonnet already ran 48 of the 80 sessions.
5. **Delegate big reads to a subagent that returns a summary**, and give it a cheap model: the session table above was collected by a Sonnet subagent, so its 84 tool results never entered this thread's context.
6. **Fewer threads per phase, not one per question.** 32 sessions started on 2026-11-26; each start costs ~93k tokens of context before any work.

## Counter-argument

A cheaper model or lower effort can cost more if it needs more rounds: cost per finished task matters, not per call. That is why items 5 and 6 for the factory wait for the polygon and L7's numbers, and why the independent review stays on Opus.
