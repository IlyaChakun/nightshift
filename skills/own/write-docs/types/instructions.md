# Agent instructions or skill

Instructions (`AGENTS.md`, `CLAUDE.md`) load into every session; a skill's `SKILL.md` loads when it is used. Every word is paid on every load.

Budget: instructions 800 words typical, 1,800 max and 200 lines ("target under 200 lines per CLAUDE.md file", [Claude Code memory](https://code.claude.com/docs/en/memory), read 2026-11-26); `SKILL.md` 600 words typical, 1,000 max and 100 lines, hard limit 500 lines ([Skill authoring best practices](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/best-practices.md), read 2026-11-26).

## Shape

- A rule is one line: the condition, the action, the reason only when it is not obvious.
- Put a rule where it is read at the moment of action: a skill step beats a line in a file loaded at start.
- Detail goes to a file next to `SKILL.md`, loaded only when needed; reference files over 100 lines get a table of contents.
- The `description` says what the skill does and when to use it, at most 1,024 characters.

## Keep it short

- Prefer a linter or a test to a prose rule.
- Do not repeat a rule another file holds; link it.
