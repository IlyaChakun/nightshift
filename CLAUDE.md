@AGENTS.md

## Claude Code specifics

- `AGENTS.md` is the single source of instructions for every agent. Do not duplicate its rules here; propose changes to `AGENTS.md` instead.
- Every directory with an `AGENTS.md` (not under `testdata/` or `evals/`) also holds a `CLAUDE.md` with the single line `@AGENTS.md`. Claude Code reads a subdirectory's `AGENTS.md` by itself only when no `CLAUDE.md` exists in the working directory or above, and this repo has one, so without the import a nested file is never loaded ([Claude Code memory docs, 2026-11-26](https://code.claude.com/docs/en/memory#agentsmd)). `scripts/repo-check.sh` checks it.
- Use `gh` or the GitHub MCP tools for PRs and issues.
- Never pass `--dangerously-skip-permissions`, edit `.claude/settings.json`, or add MCP servers or hooks without the owner's approval.
- Never use the personal Claude subscription in CI or GitHub Actions, or through anything other than an unmodified `claude`. In the personal instance the factory's `claude-sub` provider may run roles and evals on it under design section 12 (one session at a time); everything else uses API keys.
- For long work, keep a short plan in the PR description and update it as items are done.
