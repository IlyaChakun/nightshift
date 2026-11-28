# Prior art

The one digest of outside systems we looked at. One entry per system: what it is, what we take, what we do not, a link, the date checked and what it influenced. Limit: 150 lines.

This list is incomplete: it was built from the repository (the table below, ADRs, the research reports) and not from the chat history, which the first session could not re-read. A system that was only discussed in chat may be missing; add it under "Mentioned, not researched" when you meet it.

Rules for every research session:

- Add or update your entry in the same PR that reports the research. Do not write a second copy in an ADR: the ADR keeps its own "prior art" lines only for the decision it records and links here.
- A system that was only named, not read, goes under "Mentioned, not researched" with no "we take" line.
- A fact about the outside world carries a link and the date you read it, or is marked unverified (`AGENTS.md`, "Evidence").
- Tools we only use as dependencies go to "Tools we use", one line each.

## Researched

### Operator attention and actionable decisions (source excerpts checked 2026-11-26)

Report: [human attention](research/nightshift-evolution-2026-11-26/09-human-attention.md). [Developer perceptions, FSE 2014](https://www.microsoft.com/en-us/research/publication/software-developers-perceptions-of-productivity/) and [daily life, TSE 2019](https://www.microsoft.com/en-us/research/publication/today-was-a-good-day-the-daily-life-of-software-developers/) describe interruptions and perceived productivity; [SRE toil](https://sre.google/workbook/eliminating-toil/) and [on-call](https://sre.google/workbook/on-call/) motivate actionable alerts. Candidate: one durable decision projection and version-bound approvals before UI/ranking experiments. No approval automation or new service adopted; no full-paper reading or measured operator benefit.

### Antithesis (checked 2026-11-25)

A hosted platform for deterministic simulation testing: your system runs as Linux containers (Docker Compose or Kubernetes) inside one virtual machine under a deterministic hypervisor, "a deterministic fork of bhyve"; faults are injected and any found bug is reproducible from a run id, input hash and virtual time. [Docs](https://antithesis.com/docs/introduction/how_antithesis_works/), [hypervisor post, 2024-03-20](https://antithesis.com/blog/deterministic_hypervisor/), [fault types](https://antithesis.com/docs/product/writing_tests/controlling_faults/fault_types/), [assertions](https://antithesis.com/docs/product/writing_tests/assertions/), [pricing](https://antithesis.com/pricing/).

- We take: the ideas. A seed that makes every run replayable; faults injected in the twins (a code host that fails mid-step, a worker that dies, a limit hit); `always` and `sometimes` assertions, where `sometimes` proves that a scenario really reaches the state it claims to test.
- We do not take: the platform. It is SaaS with no self-hosting statement found, Teams is a beta at $2,249.25 per month for the first month and $2,999 regular, with 2,400 compute hours (pricing page, 2026-11-25), and a new account and spending need the owner (`AGENTS.md`, "Ask before you do it"). Our non-determinism lives in models, and the `fake` harness already removes it.
- Influenced: decision 14 in `docs/decisions.md` and plan items X1, X2.
- Not opened: Docker Compose and Kubernetes setup pages, the Go instrumentation pages, the licence of the hypervisor and of the Go SDK repository. The platform has no stated self-hosted option; that is a reading of the pricing page, not a statement on it.

### Temporal and durable-execution options (checked 2026-11-26)

Verdict and cost: ADR 0018; comparison: `docs/research/durable-execution.md`. [Temporal](https://docs.temporal.io/temporal-service/persistence): four services plus a database, Cloud from $50 per million actions ([pricing](https://docs.temporal.io/cloud/pricing)); we do not take it. [DBOS Transact for Go](https://docs.dbos.dev/golang/programming-guide): a Postgres-only library, pre-v1; the first option to test if ADR 0018's triggers fire. [Restate](https://restate.dev/): single binary, BSL server; no. [River Pro](https://riverqueue.com/): paid workflows; no. Not read: Temporal and Go SDK licences, Temporal versioning pages, River's free resumable jobs.

### StrongDM Software Factory and Attractor (checked 2026-11-26)

[Techniques](https://factory.strongdm.ai/techniques) (DTU, gene transfusion, filesystem, shift work, semport, pyramid summaries) and [attractor](https://github.com/strongdm/attractor) (three Apache-2.0 specs, no code): entries in `docs/research/factory-techniques.md`. We take loop detection and the at-least-once discipline; we do not take a DOT pipeline file or a satisfaction score. Not opened: `/principles`, `/products`, the rest of the specs past their first 40k characters.

### eino, LangGraph and ADK (checked 2026-11-26)

[eino](https://github.com/cloudwego/eino) (Go, Apache-2.0), [LangGraph](https://github.com/langchain-ai/langgraph) (Python, MIT, Postgres checkpointer), [ADK](https://adk.dev/) (Go, Python and others; resume and eval pages read). We take: at-least-once rules for resumed work, trajectory scoring and record-replay as proposals E1 and E2. We do not take a framework as the role runtime (ADR 0002). Not read: eino checkpoint docs, LangGraph interrupts, ADK Go durability, release versions of all three.

### Task intake and role count (checked 2026-11-26)

Report: `docs/research/task-intake.md`. [MetaGPT](https://arxiv.org/html/2308.00352v6) (Product Manager, Architect, Project Manager, Engineer, QA; ablation on GPT-era models), [ChatDev](https://arxiv.org/html/2307.07924v4) (CEO, CPO, CTO chat chain; no role ablation found), [Ambig-SWE](https://arxiv.org/html/2502.13069v3) and [ClarEval](https://arxiv.org/html/2603.00187) (models rarely ask unprompted), [Devin](https://docs.devin.ai/essential-guidelines/when-to-use-devin), [Factory spec mode and Missions](https://docs.factory.com/autonomy-and-safety/specification-mode). We take: a written spec before code, required clarifying questions, a measured ask rate. We do not take person-shaped roles (CTO, PM, architect): Lead holds them in one context. Not read: the dev.to planning-floor article (fetch failed), Open SWE and Symphony intake, StrongDM intake beyond `factory-techniques.md`; numbers from papers came through page summaries and are unverified.

### Open ways to the same goal (checked 2026-11-25)

- [FoundationDB simulation](https://apple.github.io/foundationdb/testing.html): the whole cluster in one single-threaded process, "perfect repeatability", faults such as clogged connections, reboots, full disks. We take the single-process, seeded shape for the conductor; we do not take a custom simulator for a Go service that talks to real Postgres. `BUGGIFY` was not on the page: unverified.
- [Jepsen](https://jepsen.io): an open library and published analyses of distributed databases. Its fault driver and checker terms were not read (the library repository was not opened). We take nothing yet.
- [`testing/synctest`](https://pkg.go.dev/testing/synctest): a fake clock inside a "bubble", time moves only when every goroutine is durably blocked; `Test` is marked added in go1.25.0. We take it for timer-heavy conductor tests (limit resets, `not_before`); I/O and mutex waits are not "durably blocking", so a test with a real Postgres cannot use it.
- [rapid](https://pkg.go.dev/pgregory.net/rapid) v1.3.0 (MPL-2.0): state machine tests with `T.Repeat` and automatic shrinking of a failing run. Already our T2; the gap is that the model has no outside events.

### Entire and review tooling (checked 2026-11-25)

Report: `/mnt/project-files/research/entire-and-review-tooling.md`, read from the vendors' own pages. Entire (CLI, Trails, a git network): the local multi-agent review with a judge is not a merge gate in its docs; [docs](https://docs.entire.com/learn/review-and-recap-agent-work). CodeRabbit (pre-merge checks off, warning or error), Greptile, Copilot review (reads `AGENTS.md`), Trunk (hold the line, merge queue), Danger (policy as code), reviewdog (diff-scoped), Qodo (rules mined from reviews). We take the shape of a per-path rule with block, warn or off and a logged exception; we do not take a hosted reviewer. The gap list (block or warn per check, `nightshift check --plan`, logged exceptions) is in that report and is not yet in the plan. Not confirmed: blocking by Graphite and Sourcery, Qodo's configuration layers.

### What the design takes from each source (reading date not recorded)

| Source | We take | We do not take now |
|---|---|---|
| [Majento "Dark Factory"](https://df.majento.ai/talk/) | A book written by code; dispatcher and watchdog as code; one conversation partner; research → grilling → PRD and ADR → tickets; AGENTS.md of ~100 lines; 4 message types of up to 60 words; trust layers; evidence as artifacts; acceptance by a fresh model; a hard gate without AI; a train to production; a nightly lessons loop with an exam and rollback; "words last" | TLA+ and mutation testing as mandatory; a separate team lead, critic and arbiter; a −10% performance gate |
| [LangChain GTM agent](https://www.langchain.com/blog/how-we-built-langchains-gtm-agent) | send / edit / cancel cards → Approve / Edit / Reject; SLA auto-send → SLA auto-merge; learning from diffs with structured observations in Postgres; periodic memory compression; evals (rules + LLM judge) in CI with mocks; behavior drift = a bug | LangSmith Deployment |
| [Deep Agents](https://docs.langchain.com/oss/python/deepagents/deploy) and [LangSmith](https://www.langchain.com/pricing) | Offloading large results to files; LangSmith Developer as an optional trace export | Deep Agents as the base of Lead: roles run through harnesses |
| [Open SWE](https://github.com/langchain-ai/open-swe) | A sandbox per thread; a reviewer that cannot push; rerunning only confirmed-flaky CI jobs | The framework itself: under active development, does not accept outside contributions |
| [Claude Code `/init`](https://www.claude.com/blog/using-claude-md-files), Codex `/init`, [Open SWE rules](https://docs.langchain.com/labs/swe/usage/custom-rules) | Read the repository before asking; `AGENTS.md` first, other agent files as fallbacks; a generated file is a draft to review and trim | Regenerating or rewriting a project's existing instructions |
| [OpenAI Symphony](https://openai.com/index/open-source-codex-orchestration-symphony/) | A workspace per task; setup and cleanup hooks; restarting stuck agents; policy in a file in the repository; proof of work with video; the tracker token is never given to the agent; work along the dependency graph | Linear as the state machine |
| [StrongDM Software Factory](https://simonwillison.net/2026/feb/7/software-factory/) | Hidden scenarios (holdout); twins of external services for the polygon; "humans do not write code", reading is allowed by policy | The satisfaction metric, a $1,000 per day budget |
| [obra/superpowers](https://github.com/obra/superpowers), [mattpocock/skills](https://github.com/mattpocock/skills) | Process and planning skills (section 12) | — |
| [anthropics/skills](https://github.com/anthropics/skills), [getsentry/skills](https://github.com/getsentry/skills), [trailofbits/skills](https://github.com/trailofbits/skills), [expo/skills](https://github.com/expo/skills), [vercel-labs/agent-skills](https://github.com/vercel-labs/agent-skills), [hashicorp/agent-skills](https://github.com/hashicorp/agent-skills), [wshobson/agents](https://github.com/wshobson/agents), [skills.sh](https://skills.sh/official), [agentskills.io](https://agentskills.io) | Ready skills by area: documents, review, security, mobile, frontend, infrastructure, incidents; the catalog and the format | Skills that download code at runtime and source-available skills |
| [acpx / ACP](https://github.com/janitrai/acpx), [Codex non-interactive](https://learn.chatgpt.com/docs/non-interactive-mode) | One adapter for all ACP agents; `codex exec --json --output-schema` | acpx as the main adapter: pre-1.0, and the Claude subscription works only in the native `claude` |
| [Playwright Test Agents](https://playwright.dev/docs/test-agents), [playwright-cli](https://playwright.dev/docs/getting-started-cli) | Playwright library for acceptance (screenshots, trace); planner, generator, healer for e2e | Playwright MCP |
| [LangMem](https://langchain-ai.github.io/langmem/concepts/conceptual_guide), [mem0](https://arxiv.org/abs/2504.19413), [Zep / Graphiti](https://arxiv.org/abs/2501.13956), [Letta](https://docs.letta.com/) | Memory types (working, episodic, semantic, procedural); writing memory in the background (learn) rather than in the hot path; add / update / no-op on write (deduplication); validity in time (`superseded_by`) | A memory service or SaaS, a graph database, agents editing their own memory, top-k memory injected into every prompt |
| [Multica](https://github.com/multica-ai/multica) | "Go server + Postgres + a daemon that runs agent CLIs"; workspaces as tenants | Its own UI and a license with restrictions |
| [Paperclip](https://github.com/paperclipai/paperclip) | Budgets at company, project and agent level; approval gates; several companies in one instance | The agent org chart |
| [Sortie](https://github.com/sortie-ai/sortie), [Cyrus](https://github.com/cyrusagents/cyrus) | One Go binary with tracker and agent adapters; feeding CI and review back to the agent; a worktree per task | SQLite, the cloud part of Cyrus |
| [Anthropic: Demystifying evals for AI agents](https://www.anthropic.com/engineering/demystifying-evals-for-ai-agents) | Start with a small set of tasks from real failures; regression and capability evals; code graders before LLM judges; one judge, one dimension, and an "I don't know" answer; calibrating judges against a human; reading transcripts; pass^k for reliability; a clean environment for every run | Large sets at the start |
| [Stripe Minions](https://stripe.dev/blog/minions-stripes-one-shot-end-to-end-coding-agents-part-2) | Deterministic steps around agent steps; at most two CI rounds, then a human; a small tool set per agent; rules scoped by directory | Hundreds of tools per agent; cloud devboxes |
| [OpenAI: harness engineering](https://openai.com/index/harness-engineering/) | `AGENTS.md` as a map of about 100 lines; architecture enforced by linters whose messages tell the agent how to fix; background PRs that fix stale docs | Merging without human review as the default |
| [Devin blueprint](https://docs.devin.ai/onboard-devin/environment-yaml), [OpenHands repository setup](https://docs.openhands.dev/openhands/usage/customization/repository.md), [Copilot setup steps](https://docs.github.com/en/copilot/how-tos/use-copilot-agents/coding-agent/customize-the-agent-environment) | The project describes its own environment: toolchain, a setup step for dependencies, commands for the agent; skills in `.agents/skills/` selected by paths | Snapshots in a vendor cloud; GitHub Actions as the agent runtime; a quality gate as an agent hook |
| [Factory: agent readiness](https://factory.com/news/agent-readiness) | A readiness report for a repository before agents work in it (`tenant doctor`) | Five maturity levels as a gate |
| [Harbor](https://github.com/laude-institute/harbor) | The polygon task format: `instruction.md`, `task.toml`, hidden `tests/`, `solution/` | Running in Harbor's Docker environments: later, to compare harnesses and models on the host |
| [rapid](https://github.com/flyingmutant/rapid), [Message Batches API](https://docs.anthropic.com/en/docs/build-with-claude/batch-processing) | Property tests of the state machine; −50% on nightly judge calls | — |
| [MADR](https://adr.github.io/madr/), [Vale](https://vale.sh/), [Opengrep](https://github.com/opengrep/opengrep), [squawk](https://squawkhq.com), [zizmor](https://github.com/zizmorcore/zizmor), [River](https://riverqueue.com), [grafana/otel-lgtm](https://github.com/grafana/docker-otel-lgtm), [Smokescreen](https://github.com/stripe/smokescreen) | ADR template; a prose linter with ready styles; our own SAST rules; a migration linter; workflow audit; a queue on Postgres; observability in one container; an egress proxy | — |

### Agent systems (reading date not recorded)

- [Open SWE](https://github.com/langchain-ai/open-swe): a sandbox per thread, a reviewer that cannot push, rerunning only confirmed-flaky CI jobs. We do not take the framework. Its retry behaviour (ADR 0012) was not checked.
- [OpenAI Symphony](https://openai.com/index/open-source-codex-orchestration-symphony/): a workspace per task, setup and cleanup hooks, restarting stuck agents, policy in a repository file, the tracker token never given to the agent. We do not take Linear as the state machine.
- LangChain: [GTM agent](https://www.langchain.com/blog/how-we-built-langchains-gtm-agent) (approval cards, SLA auto-send, learning from diffs), [Deep Agents](https://docs.langchain.com/oss/python/deepagents/deploy) and [LangSmith](https://www.langchain.com/pricing) (LangSmith only as an optional trace export), LangGraph (ADR 0002: not the role runtime; its retry and fallback behaviour is unverified, ADR 0012). [LangSmith Insights Agent](https://www.langchain.com/blog/from-traces-to-insights-understanding-agent-behavior-at-scale), read 2026-11-25 for ADR 0016: we take the clustering, not a hosted service.
- [Majento Dark Factory](https://df.majento.ai/talk/) (the book, dispatcher as code, evidence, a nightly lessons loop) and [StrongDM Software Factory](https://simonwillison.net/2026/feb/7/software-factory/) (hidden scenarios, twins).
- [Multica](https://github.com/multica-ai/multica): Go server, Postgres and a daemon that runs agent CLIs. We do not take its UI or its licence.
- [acpx / ACP](https://github.com/janitrai/acpx) and [Codex non-interactive](https://learn.chatgpt.com/docs/non-interactive-mode): one adapter for all ACP agents was rejected (pre-1.0, the subscription works only in the native `claude`); `codex exec --json` is taken (the `codex` harness, PR #66).
- Memory: [LangMem](https://langchain-ai.github.io/langmem/concepts/conceptual_guide), [mem0](https://arxiv.org/abs/2504.19413), [Zep / Graphiti](https://arxiv.org/abs/2501.13956), [Letta](https://docs.letta.com/). We take memory types, background writes, deduplication and validity in time; no memory service (ADR 0005).
- Skills: [obra/superpowers](https://github.com/obra/superpowers), [mattpocock/skills](https://github.com/mattpocock/skills), [anthropics/skills](https://github.com/anthropics/skills) and the other catalogues in the table above. The vendored list stays empty until decision 6.

### Process and metrics (read 2026-11-25 for design section 10 and ADR 0016)

[SRE error budget policy](https://sre.google/workbook/error-budget-policy/), [Argo Rollouts analysis](https://argo-rollouts.readthedocs.io/en/stable/features/analysis/), [Levels of Autonomy for AI Agents](https://arxiv.org/abs/2506.12469), [DORA metrics](https://dora.dev/guides/dora-metrics/), [Go telemetry](https://go.dev/doc/telemetry). For the autonomy ladder (design section 11) we take automatic tightening on a spent budget and a written policy, not SLO percentages (volume too small); metric-driven automatic rollback, not automatic promotion; autonomy as the human's role granted by evidence, not certificates; change failure and rework rate as the metric shape. The rest is in design section 10 and ADR 0016.

### Safety nets of other factories and agents (checked 2026-11-26)

Report: `docs/research/safety-nets.md`. Read through page summaries: [Copilot coding agent](https://docs.github.com/en/copilot/concepts/agents/coding-agent/risks-and-mitigations) (one branch, no self-merge, workflows wait for review, CodeQL on its output), [Codex](https://learn.chatgpt.com/docs/agent-approvals-security) (offline by default, allowlist), [Cursor cloud agents](https://cursor.com/docs/cloud-agent/security) (microVM, allowlist), [Factory Missions](https://factory.com/news/missions-architecture) (contract before features, fresh validators, halt and hand back) and [autonomy levels](https://docs.factory.com/autonomy-and-safety/auto-run), [Devin](https://docs.devin.ai/admin/billing/acu-limits.md) (budgets) and its [2025 review](https://cognition.com/blog/devin-annual-performance-review-2025), [Jules](https://jules.google/docs) and [Kiro](https://kiro.dev/docs/specs/) (plan or spec first), [OpenHands](https://docs.openhands.dev/sdk/guides/security) (confirm by risk), [SWE-agent](https://arxiv.org/pdf/2405.15793) (lint at edit, $4 cap), [Aider](https://aider.chat/docs/git.html) (commit and undo), [METR](https://metr.org/blog/2025-06-05-recent-reward-hacking) and [EvilGenie](https://arxiv.org/html/2511.21654v1) (graders get tampered with), [Antithesis](https://www.globenewswire.com/news-release/2026/04/23/3280089/0/en/Antithesis-Teaches-AIs-To-Correct-Their-Own-Output.html) (mutation testing, vendor claim). We take a coverage check of the owner's message (N5), an independent reading (N6, pilot) and a Lead run record (L5). Not found: Sweep, any canary step, a measured gain of a second-model critic. Open SWE, Symphony, StrongDM, MetaGPT and ChatDev keep their entries above.

### Token economy of agent harnesses (checked 2026-11-26)

Report: `docs/research/token-economy.md`. [Anthropic prompt caching](https://platform.claude.com/docs/en/build-with-claude/prompt-caching), [Claude Code costs](https://code.claude.com/docs/en/costs) and [prompt caching](https://code.claude.com/docs/en/prompt-caching), [JetBrains Complexity Trap](https://arxiv.org/html/2508.21433v3) (masking old observations halves cost), [OpenHands condenser](https://openhands.dev/blog/openhands-context-condensensation-for-more-efficient-ai-agents), [Cursor dynamic context](https://cursor.com/blog/dynamic-context-discovery), [Aider repo map](https://aider.chat/docs/repomap.html), [Amp handoff](https://ampcode.com/news/handoff), [Chroma context rot](https://trychroma.com/research/context-rot), [advisor strategy](https://claude.com/blog/the-advisor-strategy). We take: cache tokens per job, small tool output, fresh sessions from a handoff, a cheaper Builder behind the same gates, one stable prefix. We do not take: LLM-summary compaction or multi-agent fan-out as defaults.

### Document length guidance (checked 2026-11-26)

Google design docs, Nygard ADRs, MADR, the SRE postmortem chapter, Diátaxis, and Anthropic's `CLAUDE.md` and skill guides. Quotes and links: `docs/research/doc-length.md`.

- We take: design doc 1–3 pages for a feature, 10–20 for a system; an ADR of one or two pages; `CLAUDE.md` under 200 lines, `SKILL.md` under 500; Diátaxis's split of how-to and reference.
- We do not take: a fixed template for every kind; sections are suggestions in all sources.
- Influenced: the budgets in `skills/own/write-docs`, `scripts/doc-budget.sh`, stream `docs/plan/docs-budget/`.

## Mentioned, not researched

- [Clio](https://arxiv.org/abs/2412.13678): cited in ADR 0016; only its title was opened.
- [Harbor](https://github.com/laude-institute/harbor) and Terminal-Bench: we use the Harbor task format (design section 20); the framework itself was not evaluated.
- Hegel and Bombadil: named on Antithesis's pricing page as its open-source tools; not opened.
- TLA+ and mutation testing: named in the table above as not taken now; not evaluated.

## Tools we use

- Codex and DeepSeek (through Codex's `base_url`) as worker providers: the `codex` harness (PR #66); ADR 0012 covers their limits. The live run is not done.
- [Playwright Test Agents](https://playwright.dev/docs/test-agents) and [playwright-cli](https://playwright.dev/docs/getting-started-cli): acceptance runs; not the Playwright MCP.
- mise (pinned toolchains, ADR 0009) with Sigstore's trust root `tuf-repo-cdn.sigstore.dev` in the install egress list, River, sqlc, goose, testcontainers, gitleaks, CodeQL.
