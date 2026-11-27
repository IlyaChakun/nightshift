# Execution plane

> Part of the [Nightshift design](../design.md). Where agents run: the worker, the roles, the harnesses that reach models, and the stack and tools a run uses. It runs what control sends and returns results by schema.

## 5. Stack and tools

Tool versions are pinned in `mise.toml` and checksummed in `mise.lock`, the same in the worker image and in CI; `claude` is pinned too (section 12).

### The factory itself

| Layer | Choice | Why |
|---|---|---|
| Core language | Go 1.27 | One `nightshift` binary with the subcommands `serve`, `work`, `deps`, `toolchain`, `deployer`, `check`, `standards`, `skills`, `tenant`, `config`, `task`, `memory`, `search`, `digest`, `setup`, `eval`, `doctor`, `status`, `inbox`, `reply`, `approve`, `cancel`, `mcp` |
| Database and queue | Postgres 18 + pgvector; River (MPL-2.0) for the outbox flush; `book.jobs` for agent job leases | Book, queue and search in one database |
| DB access | pgx v5, sqlc, goose | Typed queries, expand-only migrations |
| Integrations | slack-go (Socket Mode), genqlient (Linear), go-github + GitHub App | No inbound ports |
| Config | YAML + JSON Schema (santhosh-tekuri/jsonschema) | Validated packs, roles, agent results |
| Telemetry | log/slog (JSON) + OpenTelemetry | One format for logs, traces, metrics |
| Tests | go test with `-fuzz`, rapid, httptest twins, the `fake` adapter; Postgres integration tests in CI only | No network, tokens or models (section 20) |
| Python | uv, ruff, mypy, pytest | Eval graders, transcription |
| CI | GitHub Actions pinned by SHA; Dependabot | Retagged actions |

### Project toolchains in the worker

Any language runs without a pack. The worker image (Debian slim, ADR 0009) has mise, git and the agent CLIs but no language toolchain. Before a job, as the agent user:

1. **Toolchain.** `mise install` from the project's `mise.toml` or `.tool-versions`, run by the `toolchain` service (the only container that reaches the tool hosts); tools persist in the `agent-home` volume (ADR 0010). Go modules come from `modcache` (ADR 0008).
2. **Dependencies.** `project.yaml: setup` (for example `npm ci`).
3. **Checks.** `verify` gates every language; stack packs add L1 tools with SARIF.

Without a toolchain file the detected stack packs' default versions apply and `tenant doctor` proposes a `mise.toml`. Stacks that need root packages need a tenant worker image ("Later").

### Quality tools (stack packs)

| Area | Tools |
|---|---|
| Go | golangci-lint v2 (staticcheck, gosec, errcheck, revive, gocognit, dupl, depguard), govulncheck (any advisory in a required module fails) |
| Python | ruff (with bandit rules), mypy, pytest + coverage |
| TypeScript | tsc, ESLint, Playwright Test |
| Own rules | Opengrep |
| Secrets, dependencies | gitleaks; osv-scanner, govulncheck |
| Migrations, APIs, accessibility | squawk; Spectral and oasdiff (`openapi` pack); axe-core (`web` pack) |
| Containers and CI | hadolint, actionlint, zizmor, shellcheck |
| Change metrics | diff size, new dependencies, coverage; later diff-cover, jscpd |
| Documents | doc-lint, markdownlint-cli2, mermaid-cli, Vale, lychee |
| Java (`java-maven`, with its first tenant) | Maven wrapper, Spotless or Checkstyle, Error Prone or SpotBugs, PMD, ArchUnit, JaCoCo, Flyway |
| Project specifics | `.nightshift/standards/` (for example kubeconform) |

### Observability and security

| Task | Choice |
|---|---|
| Telemetry | OTLP from `nightshift` and the runner: a trace per task, a span per job and agent turn; agent CLIs export their own OTel |
| Storage and UI | `grafana/otel-lgtm` in the `obs` profile; Grafana also reads the book as a Postgres source. Not Splunk: proprietary, and its free tier has no alerts or OTel traces; OTel lets the backend change without code |
| Egress | Smokescreen as the `egress` service: the worker's only way out, by allowlist |
| Supply chain | `mise.lock` checksums, actions by SHA, zizmor, updates only through PRs |

## 6. Roles

A separate role exists only where a **different context, a different model or a conflict of interest** is needed. Everything else is a role mode, a tool or code.

| Role | What it does | Rights |
|---|---|---|
| **Lead** | The only conversation partner in chat. **#planning:** research → grilling → PRD, ADR, criteria and hidden scenarios → slicing into tasks with dependencies. **#support:** looks for reasons *not* to open a task → reproduction → card with a priority. Answers other roles from the owner's past decisions, with quote, source and date | Reads the repository, `ns`. Writes to the worktree; only documents leave, as a patch through `nightshift`. Web research is a separate job without access to code: the input is only the formulated question, the output only the findings |
| **Builder** | TDD, unit and e2e tests, the "edit → `nightshift check`" loop until green. Answers findings of all layers (or disputes them) and every comment of the owner's review | Writes to the worktree. Does not see hidden scenarios; `nightshift` rejects a patch that touches protected paths (section 10, item 8) |
| **Verifier** | Fresh context, preferably a model of another family. **review:** spec first, then quality. **acceptance:** hidden scenarios on preview in Chromium through Playwright (own skill `browser-acceptance`; the browser is baked into the worker image). **audit** (scheduled): audit of the repository and of the factory itself (section 10). Settles disputes over findings. Escalates after 3 returns | Reads the repository only; writes only to its job directory. Receives hidden scenarios from the runner through the job tmpfs; after acceptance Builder sees only a description of the failed behavior |
| **learn** (nightly job) | Clusters findings, looks for the root cause, proposes the cheapest fix, tests it and opens a PR (section 10) | PRs to standards, skills, templates and `LEARNINGS.md`; what goes in without the owner is in section 9 |

**Why this is enough.** Author and checker are separated. Planning and triage need one context, so they are Lead modes. Audit is a Verifier mode, because it also needs fresh eyes without authorship. LLM rubric checks are not a role but a tool: one call to a cheap model without tools. The dispatcher, team-lead coordinator, release manager and watchdog from the Majento design are Go code.

**How Lead challenges a task.** The `challenge` skill on top of `grilling`:

1. Before every question it searches the book (`ns search`). If the answer exists, it quotes the decision instead of asking.
2. It checks: what problem we solve and how we will know it is solved; why now and what happens if we do nothing; the smallest version that tests the idea; contradictions with ADRs, standards and past decisions; money, data, access, architecture and irreversibility (then it sets a tag that raises the blast radius, section 11); what we deliberately do not do.
3. It asks one question per message, with 2–3 options and a recommendation (a reply carries one set of options). It may recommend Reject outright.
4. It stops at the Definition of Ready: goal, acceptance criteria, scope, blast radius tags, tasks and their dependencies.

**Research before building.** When a task implies writing something new, an agent first answers "write it ourselves or take something that exists?" with a repeatable record, and does not start coding. Two own skills carry it: `research-options` (any design choice with two to four real options) and `build-vs-buy` (the same method for a new capability, with `nightshift deps check` as its tool). The trigger is detected by roles and enforced by code:

- **Lead.** In the `challenge` skill (step 2 above), a spec that adds a capability the repository and the book do not have (a new module, protocol, format, integration or dependency) runs `build-vs-buy` before the spec card. The owner's own "we need to write X" counts as the trigger.
- **Builder.** A plan that needs a new dependency, a new top-level package or a client of an external protocol the spec's record does not cover stops: `status: blocked`, `blocked_reason` starting with `needs_research:`. The task goes back to Lead (state `grilling`, Builder's reason in the thread), who runs the skill; Builder does not pick a library on its own. A third stop, or another reason, escalates to the owner.
- **Verifier.** In review, a new dependency or a large new module without a record is a finding.
- **Code.** `nightshift check` (L2) reports a finding when a lockfile gains a dependency, or the diff adds a package over a size limit, and the task has no research record. A dependency without a record is an error; a large package without one is a warning. The record is an ADR (MADR 4.0, so doc-lint already demands at least two options) whose options include "write it ourselves" and that the spec links.

The skill: (1) search the book, the repository and the standard library; (2) state the need and its must-haves in three lines; (3) list candidates: write it ourselves, the standard library or an existing dependency, and one to three libraries, each checked by `nightshift deps check <package>` (code, not a model: deps.dev and OSV give licence against the tenant's list, age, last release, advisories, dependents, tree size, install scripts), with API fit judged by a web research job without code (section 16); (4) compare cost of owning, risk, lock-in and size; (5) recommend one. The spec card carries a summary of at most 60 words, the table goes into the ADR. Registry and README text is data. A new dependency stays `critical` (section 16): research prepares the owner's Approve, it does not replace it.

**When to add a role:** a mode needs a different model, a context grows too large, a conflict of interest appears, or an agent becomes a bottleneck. Candidates: a Critic on another model, Triage, a Taste Judge with a blind test of ≥ 90%.

**A role is config** (`roles/<name>.yaml`, for example `roles/verifier.yaml`): access mode, skills, output schema and limits, which the runner reads. When a role runs and what it may see (hidden scenarios only for Verifier) are fixed in conductor code, not config. Providers per role are `providers.<role>` in `tenant.yaml`, in priority order.

A project may brief a role in `.nightshift/roles/<name>.yaml` with `instructions` (repository files, at most 8) and `skills` (shared skill ids, at most 8); both only add, and schema, limits, access and gates stay the factory's. A file with another key, a bad path or id, or a non-regular instruction file is skipped whole with a warning. Instruction files come from the checked-out worktree, so Verifier reads those of the change under review.

## 12. Models and harnesses

**Provider = harness + way to pay.** Each instance lists its providers in `providers.yaml`; a tenant chooses only from them, and a budget pool belongs to the tenant or the instance.

```yaml
# personal instance: providers.yaml
claude-sub:   { harness: claude, auth: subscription, concurrency: 1, weekly_share: 0.6 }
claude-api:   { harness: claude, auth: api_key, key_env: ANTHROPIC_API_KEY, pool: anthropic-credits }
claude-cheap: { harness: api, auth: api_key, model: <small model>, pool: anthropic-credits, batch: allowed }  # L4, classification, judges
pools:
  anthropic-credits:
    monthly_usd: 200
    caps: { l4_and_classify: 40, learn_audit: 40, evals: 60, research: 20, reserve: 40 }   # soft caps by kind of work
codex-api:    { harness: codex, auth: api_key, key_env: OPENAI_API_KEY, model: <model>, pool: openai-credits, price: { input_per_mtok: <usd>, output_per_mtok: <usd> } }
codex-sub:    { harness: codex, auth: chatgpt, concurrency: 1, batch: forbidden }
deepseek:     { harness: codex, auth: api_key, key_env: DEEPSEEK_API_KEY, model: <model>, base_url: https://api.deepseek.com/, tier: fallback }
```

A work instance lists only what the employer allows (for example `claude-corp` on the company's organization key, `copilot` on its licence, pool `corp` with no factory cap).

**Adapters** (`internal/execution/harness/<name>`; the flags live there, with links to the CLI docs):

| Harness | Runs | Result and permissions |
|---|---|---|
| `claude` | `claude -p`, JSON or stream-JSON | `--json-schema` result in `structured_output`; `--permission-mode dontAsk`, `--allowedTools`, `--permission-prompts none` (needs `claude` 2.1.259+, pinned in `mise.toml`); subscription by a `claude setup-token` token, API providers with `--bare` |
| `codex` | `codex exec --json` | `--output-schema`, result in the `-o` file; `base_url` declares a custom Responses provider (OpenAI, DeepSeek); `auth: chatgpt` passes `CODEX_ACCESS_TOKEN` and forces ChatGPT login, needs `chatgpt.com` and `auth.openai.com` on the egress list; sessions never resume |
| `copilot` | `copilot -p --output-format=json` | explicit `--allow-tool` list; no schema check in the CLI, so the runner validates `result.json` and asks once for a fix |
| `api` | one Messages API call, no tools | L4 rubrics, classification, judges; nightly calls through the Batches API (section 20) |
| `fake` | a scenario file | factory tests (section 20) |

Later: `opencode run`, `acpx` for ACP agents. `codex-sub` and `deepseek` are experimental until a live run passes.

**Open and local models.** Lead and Verifier stay on a paid model; Builder may move to an open model behind an Anthropic- or OpenAI-compatible endpoint, because the gates trust no model. `base_url` works for `claude` as for `codex` (Ollama 0.14+ serves part of the Messages API, [Ollama](https://docs.ollama.com/api/anthropic-compatibility)); a provider may declare `context_tokens` so the router skips it for jobs that would not fit; a local provider has its own zero-price pool and one egress entry. The polygon decides which role moves (section 20).

**Model and effort per role** are configuration, never the agent's choice (ADR 0020): `model` per harness and a neutral `effort` in the role file, overridable per tenant under `roles`; a provider's own `model` pins it. Ids are full, never aliases, because aliases move with CLI versions. The job records both.

Role prompts, standards and skills are vendor-neutral text; everything CLI-specific (flags, instruction file names, permission syntax, environment) lives in the adapter.

**Instructions and skills are passed explicitly.** Bare mode and other harnesses do not discover `CLAUDE.md`, skills, hooks or MCP the same way, so the runner relies on none of it. It joins the project instructions (`project.yaml: instructions`) and the L0 checklist into one system-prompt addition (`--append-system-prompt-file` for `claude`), copies the task's skills into a job directory passed with `--add-dir`, and lists them in the prompt. The checklist points to pages under the worker's read-only `/standards` (`jobsapi.StandardsDir`), which the agent gets as one more directory. A subscription run is not bare, so the adapter sets `CLAUDE_CODE_DISABLE_CLAUDE_MDS=1` when the project names instruction files, or `CLAUDE.md` would load twice. Hooks and MCP from the worktree are protected paths (section 10); role settings come through `--settings`. Bare mode reads no subscription, only an API key, so `nightshift doctor` checks that `claude-sub` really runs on the subscription. Contract test T8 covers this.

**Claude subscription rules (personal instance).** Subscription OAuth is for Claude Code and Anthropic's apps, with limits for ordinary individual use. So:

- `claude-sub` runs only through an unmodified `claude`, one session at a time: Lead with the owner, Builder and Verifier on the owner's tasks, the role under test in the owner's evals (section 20);
- background and bulk work (learn, audit, L4, batch judges) uses `claude-api` and `claude-cheap`;
- never in GitHub Actions, never through other wrappers;
- the runner strips `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, `apiKeyHelper` and cloud provider variables from the `claude-sub` environment.

**Work instance.** Personal subscriptions and credits never touch company code; the company's commercial account is the default, and its policy, recorded in the work pack, says what is allowed. A tenant with `data_classification: pci` uses only a provider approved for that class.

**Continuity and handoff.** The book keeps the harness session id per (task, role), and Lead's next turn continues it. When a session cannot continue (restart, provider switch, context limit, `codex`), the runner starts a fresh one with the role's last `handoff` block (at most 2 KB: `done`, `pending`, `decisions`, validated against `schemas/`) as data under "Previous session, unverified", plus the task's `events`, the worktree diff and open findings. Without a handoff, `nightshift` builds one from the events, with no model. A handoff is never searched or shown to other roles and is deleted with the task's sessions (section 14).

**Router and scheduler** (ADR 0012). Usage comes from harness responses; `weekly_share` is a soft threshold because the subscription reports no exact number. A limit is stored per provider with its reset time; the job requeues without counting an attempt. Within `max_wait` (default 2 hours) the task waits, past it the next provider in the role's chain runs. A `tier: fallback` provider runs only opted-in tasks (`nightshift task add --allow-fallback` or `on_limit.fallback: always`), never answers the owner for Lead, and spends its own pool; the gates are the same, so a weaker model costs rounds, not safety. A fully exhausted chain parks the task; a "Blocker" goes out only when the reset is over 12 hours away. Tasks carry `not_before` and `priority`; the factory works while the queue is not empty, and quiet hours limit messages and releases, not work. Jobs go round-robin across tenants by `tenant.yaml` weights, with Lead's answers first, then Builder and Verifier, then learn, audit and polygon; the `nightshift` tenant has a guaranteed minimum.

### Skills

A skill is a folder with `SKILL.md` in the open Agent Skills format. A role says who works, skills say how. We prefer, in order: the technology vendor's official skill, a recognized community set ([skills.sh](https://skills.sh/official)), our own (`skills/own/`, only for what is specific to the factory).

| Area | Source, licence | Role |
|---|---|---|
| Planning, analysis | [mattpocock/skills](https://github.com/mattpocock/skills), MIT; own `challenge`, `research-options`, `build-vs-buy`, `hidden-scenarios`, `write-docs` | Lead |
| Process | [obra/superpowers](https://github.com/obra/superpowers), MIT | Builder, Verifier |
| Go | [samber/cc-skills-golang](https://github.com/samber/cc-skills-golang) or [cxuu/golang-skills](https://github.com/cxuu/golang-skills), chosen on the polygon | Builder, Verifier |
| Python, infrastructure, incidents | [wshobson/agents](https://github.com/wshobson/agents), MIT; [hashicorp/agent-skills](https://github.com/hashicorp/agent-skills), MPL 2.0 | Builder, Lead |
| Frontend, mobile | [anthropics/skills](https://github.com/anthropics/skills), Apache 2.0; [vercel-labs/agent-skills](https://github.com/vercel-labs/agent-skills), MIT; [expo/skills](https://github.com/expo/skills), MIT | Builder |
| UI acceptance | own `browser-acceptance` on the image's Chromium | Builder, Verifier |
| Review, security | [getsentry/skills](https://github.com/getsentry/skills), Apache 2.0; [trailofbits/skills](https://github.com/trailofbits/skills), CC BY-SA 4.0 (never modified, own overlay on top) | Verifier |
| The factory itself | getsentry `skill-scanner`, anthropics `skill-creator`; own `evidence-report`, `audit` | learn, Verifier |

`skills/vendor.yaml` is the single source of external skills: repository, path, 40-character `sha`, licence, `roles`, `applies_to`. `nightshift skills sync` fetches them by SHA into `skills/vendor/<id>/`, scans them with `skill-scanner` and gitleaks, and opens a PR. Roles name skills by `id` (`sentry/code-review`), so equal names from different sets do not clash.

**Taking an external skill:**

1. **Security.** Skills are supply chain with agent rights (Snyk's February 2026 audit found 13.4% of 3,984 skills with a critical issue, 76 malicious). Adding or updating one is blast radius `critical` with the owner's Approve; nothing that downloads at run time.
2. **Value.** It runs on the polygon with and without it; taken only if results improve at similar cost.
3. **Licence** recorded; no source-available skills.
4. **Connection.** The runner gives a job only the skills that apply (paths, tags, stack, dependencies such as `applies_to: { deps: [next] }`). Project skills live in `.nightshift/skills/`.
5. **Improvement** goes through the fix ladder and polygon (section 10); an external skill is never edited, an own overlay goes on top.

**Supervisor (ADR 0019).** Adapters turn CLI output into three events (output, tool call, usage), and `internal/execution/harness/supervise` stops a run on no event for `stall_after`, the same tool call `repeat_limit` times, or more than `tokens` or `cost_usd` in the turn: SIGTERM, then SIGKILL, exit `budget` with the reason. Claude's prompt-cache reads and writes are recorded per job (`cache_read_tokens`, `cache_write_tokens`, `nightshift status --task`) but not counted, Codex counts them inside input. A zero limit switches a rule off; limits come from the worker default, the role (`limits.stall_minutes`, `repeat_limit`, `cost_usd`, `minutes`, `tokens`) and the tenant (`budget.run`), each overriding the one before. New rules plug in as the `Rule` interface.
