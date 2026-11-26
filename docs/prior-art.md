# Prior art

The one digest of outside systems we looked at. One entry per system: what it is, what we take, what we do not, a link, the date checked and what it influenced. Limit: 150 lines.

This list is incomplete: it was built from the repository (design section 19, ADRs, the research reports) and not from the chat history, which the first session could not re-read. A system that was only discussed in chat may be missing; add it under "Mentioned, not researched" when you meet it.

Rules for every research session:

- Add or update your entry in the same PR that reports the research. Do not write a second copy in an ADR: the ADR keeps its own "prior art" lines only for the decision it records and links here.
- A system that was only named, not read, goes under "Mentioned, not researched" with no "we take" line.
- A fact about the outside world carries a link and the date you read it, or is marked unverified (`AGENTS.md`, "Evidence").
- Tools we only use as dependencies go to "Tools we use", one line each.

## Researched

### W3C PROV and OpenLineage (Deep Research excerpts, 2026-11-26)

[PROV-DM](https://www.w3.org/TR/prov-dm/) models entities, activities and agents; [OpenLineage](https://openlineage.io/docs/spec/object-model/) separates job, run and dataset observations from design metadata. Candidate: typed source-linked relations and explicit observation status over the existing book. No graph database or universal lineage service. See [metadata research](research/metadata-governance-lineage.md).

### SLSA provenance and in-toto (Deep Research excerpts, 2026-11-26)

[SLSA v1.2](https://slsa.dev/spec/v1.2/build-provenance) distinguishes build and source provenance. Candidate: exact artifact identity and trusted producer evidence, not a substitute for independent behavioral acceptance. [in-toto specifications](https://in-toto.io/docs/specs/) are a potential interchange format. No signing implementation adopted; full attestation specification not reviewed.

### DataHub and OpenMetadata (Deep Research excerpts, 2026-11-26)

[DataHub lineage](https://docs.datahub.com/docs/features/feature-guides/lineage), [OpenMetadata](https://docs.open-metadata.org/latest) document catalog and lineage capabilities. Candidate: scoped read-only integration if a real tenant already needs it; no catalog deployment in core. [DataHub UI lineage](https://docs.datahub.com/docs/features/feature-guides/ui-lineage) describes latest-state lineage, not historical reconstruction. Licensing, integration overhead and coverage unverified.

### Policy, traces and data artifacts (Deep Research excerpts, 2026-11-26)

[OPA](https://www.openpolicyagent.org/docs) separates policy decisions from enforcement; retain current deterministic gates. [OpenTelemetry traces](https://opentelemetry.io/docs/concepts/signals/traces/) provide correlated execution evidence, not authorization; [dbt manifest](https://docs.getdbt.com/reference/artifacts/manifest-json) provides versioned design metadata, not proof of execution. See [metadata research](research/metadata-governance-lineage.md). [OTel sampling](https://opentelemetry.io/docs/concepts/sampling/) reinforces the non-authoritative telemetry boundary. [dbt artifacts](https://docs.getdbt.com/reference/artifacts/dbt-artifacts) distinguish parsed metadata and execution results. No new policy engine or data stack adopted.

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

### Agent systems (from design section 19; reading date not recorded)

- [Open SWE](https://github.com/langchain-ai/open-swe): a sandbox per thread, a reviewer that cannot push, rerunning only confirmed-flaky CI jobs. We do not take the framework. Influenced: design section 19. Its retry behaviour (ADR 0012) was not checked.
- [OpenAI Symphony](https://openai.com/index/open-source-codex-orchestration-symphony/): a workspace per task, setup and cleanup hooks, restarting stuck agents, policy in a repository file, the tracker token never given to the agent. We do not take Linear as the state machine.
- LangChain: [GTM agent](https://www.langchain.com/blog/how-we-built-langchains-gtm-agent) (approval cards, SLA auto-send, learning from diffs), [Deep Agents](https://docs.langchain.com/oss/python/deepagents/deploy) and [LangSmith](https://www.langchain.com/pricing) (LangSmith only as an optional trace export), LangGraph (ADR 0002: not the role runtime; its retry and fallback behaviour is unverified, ADR 0012). [LangSmith Insights Agent](https://www.langchain.com/blog/from-traces-to-insights-understanding-agent-behavior-at-scale), read 2026-11-25 for ADR 0016: we take the clustering, not a hosted service.
- [Majento Dark Factory](https://df.majento.ai/talk/) (the book, dispatcher as code, evidence, a nightly lessons loop) and [StrongDM Software Factory](https://simonwillison.net/2026/feb/7/software-factory/) (hidden scenarios, twins). Details in design section 19.
- [Multica](https://github.com/multica-ai/multica): Go server, Postgres and a daemon that runs agent CLIs. We do not take its UI or its licence.
- [acpx / ACP](https://github.com/janitrai/acpx) and [Codex non-interactive](https://learn.chatgpt.com/docs/non-interactive-mode): one adapter for all ACP agents was rejected (pre-1.0, the subscription works only in the native `claude`); `codex exec --json` is taken (the `codex` harness, PR #66).
- Memory: [LangMem](https://langchain-ai.github.io/langmem/concepts/conceptual_guide), [mem0](https://arxiv.org/abs/2504.19413), [Zep / Graphiti](https://arxiv.org/abs/2501.13956), [Letta](https://docs.letta.com/). We take memory types, background writes, deduplication and validity in time; no memory service (ADR 0005).
- Skills: [obra/superpowers](https://github.com/obra/superpowers), [mattpocock/skills](https://github.com/mattpocock/skills), [anthropics/skills](https://github.com/anthropics/skills) and the other catalogues in design section 19. The vendored list stays empty until decision 6.

### Process and metrics (read 2026-11-25 for design section 10 and ADR 0016)

[SRE error budget policy](https://sre.google/workbook/error-budget-policy/), [Argo Rollouts analysis](https://argo-rollouts.readthedocs.io/en/stable/features/analysis/), [Levels of Autonomy for AI Agents](https://arxiv.org/abs/2506.12469), [DORA metrics](https://dora.dev/guides/dora-metrics/), [Go telemetry](https://go.dev/doc/telemetry). What we take from each is in design section 10 and ADR 0016.

### Safety nets of other factories and agents (checked 2026-11-26)

Report: `docs/research/safety-nets.md`. Read through page summaries: [Copilot coding agent](https://docs.github.com/en/copilot/concepts/agents/coding-agent/risks-and-mitigations) (one branch, no self-merge, workflows wait for review, CodeQL on its output), [Codex](https://learn.chatgpt.com/docs/agent-approvals-security) (offline by default, allowlist), [Cursor cloud agents](https://cursor.com/docs/cloud-agent/security) (microVM, allowlist), [Factory Missions](https://factory.com/news/missions-architecture) (contract before features, fresh validators, halt and hand back) and [autonomy levels](https://docs.factory.com/autonomy-and-safety/auto-run), [Devin](https://docs.devin.ai/admin/billing/acu-limits.md) (budgets) and its [2025 review](https://cognition.com/blog/devin-annual-performance-review-2025), [Jules](https://jules.google/docs) and [Kiro](https://kiro.dev/docs/specs/) (plan or spec first), [OpenHands](https://docs.openhands.dev/sdk/guides/security) (confirm by risk), [SWE-agent](https://arxiv.org/pdf/2405.15793) (lint at edit, $4 cap), [Aider](https://aider.chat/docs/git.html) (commit and undo), [METR](https://metr.org/blog/2025-06-05-recent-reward-hacking) and [EvilGenie](https://arxiv.org/html/2511.21654v1) (graders get tampered with), [Antithesis](https://www.globenewswire.com/news-release/2026/04/23/3280089/0/en/Antithesis-Teaches-AIs-To-Correct-Their-Own-Output.html) (mutation testing, vendor claim). We take a coverage check of the owner's message (N5), an independent reading (N6, pilot) and a Lead run record (L5). Not found: Sweep, any canary step, a measured gain of a second-model critic. Open SWE, Symphony, StrongDM, MetaGPT and ChatDev keep their entries above.

### Apache Atlas (Deep Research excerpts, 2026-11-26)

[Atlas](https://atlas.apache.org/) documents metadata classification and lineage. Optional existing-tenant source only; no deployment adopted. See [metadata research](research/metadata-governance-lineage.md).

### Open Data Contract Standard (Deep Research excerpts, 2026-11-26)

[ODCS](https://bitol.io/odcs/) documents machine-readable contracts and schema validation; [Data Contract CLI](https://cli.datacontract.com/) uses ODCS. Candidate: one repository-owned tenant contract, separate declaration from evidence of runtime compliance. No dependency adopted.

## Mentioned, not researched

- OpenLineage column-level coverage, full in-toto attestation specification, schema registry compatibility and catalog deployment/licensing: follow-up leads in `docs/research/metadata-governance-lineage.md`; not fully evaluated.
- [Clio](https://arxiv.org/abs/2412.13678): cited in ADR 0016; only its title was opened.
- [Harbor](https://github.com/laude-institute/harbor) and Terminal-Bench: we use the Harbor task format (design section 20); the framework itself was not evaluated.
- Hegel and Bombadil: named on Antithesis's pricing page as its open-source tools; not opened.
- TLA+ and mutation testing: named in design section 19 as not taken now; not evaluated.

## Tools we use

- Codex and DeepSeek (through Codex's `base_url`) as worker providers: the `codex` harness (PR #66); ADR 0012 covers their limits. The live run is not done.
- [Playwright Test Agents](https://playwright.dev/docs/test-agents) and [playwright-cli](https://playwright.dev/docs/getting-started-cli): acceptance runs; not the Playwright MCP.
- mise (pinned toolchains, ADR 0009) with Sigstore's trust root `tuf-repo-cdn.sigstore.dev` in the install egress list, River, sqlc, goose, testcontainers, gitleaks, CodeQL.
