@AGENTS.md

## Claude Code specifics

- `AGENTS.md` is the single source of instructions for every agent. Do not duplicate its rules here; propose changes to `AGENTS.md` instead.
- Use `gh` or the GitHub MCP tools for PRs and issues.
- Never pass `--dangerously-skip-permissions`, edit `.claude/settings.json`, or add MCP servers or hooks without the owner's approval.
- Never use the personal Claude subscription in CI or GitHub Actions, or through anything other than an unmodified `claude`. In the personal instance the factory's `claude-sub` provider may run roles and evals on it under design section 12 (one session at a time); everything else uses API keys.
- For long work, keep a short plan in the PR description and update it as items are done.
- A `Stop` hook (`.claude/settings.json`, `.claude/hooks/self-audit-stop.sh`, owner-approved 2026-11-25) stops a session that changed code until it has put a filled `self-audit` block in its report and the promise tests are not red because of its change. It blocks once per stop and passes on failures that origin/main has too. Do not weaken or bypass it; fix its message or logic in a PR.
