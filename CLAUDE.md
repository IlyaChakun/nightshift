@AGENTS.md

## Claude Code specifics

- `AGENTS.md` is the single source of instructions for every agent. Do not duplicate its rules here; propose changes to `AGENTS.md` instead.
- Use `gh` for PRs and issues. Open PRs as draft. Merge your own PR only under the bootstrap exception in `AGENTS.md`.
- Never pass `--dangerously-skip-permissions`, edit `.claude/settings.json`, or add MCP servers or hooks without the owner's approval.
- Never use the personal Claude subscription in CI, scripts or GitHub Actions. Automated runs use API keys only, through the `claude` harness adapter.
- For long work, keep a short plan in the PR description and update it as items are done.
