# skills/

Skills for the factory's roles. Skills for agents that work on this repo live elsewhere (below).

## Three places

| Path | What | Rule |
|---|---|---|
| `skills/own/<name>/SKILL.md` | our skills for Lead, Builder and Verifier (for example `to-spec`), and for the owner's own agent (`onboard-tenant`, `configure-tenant`, `plan-night-queue`) | a role loads the ones its `roles/<role>.yaml` lists; `cmd/nightshift/promises_test.go` fails on a missing one |
| `skills/vendor/` | vendored third-party skills | pinned by commit SHA in `skills/vendor.yaml`; never edit by hand; `nightshift skills sync` refreshes them |
| `.agents/skills/<name>/SKILL.md` | skills for agents changing this repo (`finish-task`, `self-audit`, `write-docs`, which builds on `skills/own/write-docs`) | `.claude/skills` is a symlink to it |

## Rules

- A skill is a folder with `SKILL.md`. Front matter holds `name` (equals the folder name) and a `description` that says when to use it. `scripts/repo-check.sh` checks `.agents/skills`.
- Keep `SKILL.md` under 100 lines; put detail in a file next to it. Write it with `.agents/skills/write-docs`.
- Adding or updating a `skills/vendor.yaml` entry is blast radius critical and needs the owner's Approve; read the licence and pin a SHA.
- A skill that a role needs but is missing: add it here, then name it in the role file.
