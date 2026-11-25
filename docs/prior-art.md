# Prior art

The one digest of outside systems we looked at. One entry per system: what it is, what we take, what we do not, a link, the date checked and what it influenced. Limit: 150 lines.

This list is incomplete: it was built from the repository (design section 19, ADRs, the research reports) and not from the chat history, which the first session could not re-read. A system that was only discussed in chat may be missing; add it under "Mentioned, not researched" when you meet it.

Rules for every research session:

- Add or update your entry in the same PR that reports the research. Do not write a second copy in an ADR: the ADR keeps its own "prior art" lines only for the decision it records and links here.
- A system that was only named, not read, goes under "Mentioned, not researched" with no "we take" line.
- A fact about the outside world carries a link and the date you read it, or is marked unverified (`AGENTS.md`, "Evidence").
- Tools we only use as dependencies go to "Tools we use", one line each.

## Researched

### Antithesis (checked 2026-11-25)

A hosted platform for deterministic simulation testing: your system runs as Linux containers (Docker Compose or Kubernetes) inside one virtual machine under a deterministic hypervisor, "a deterministic fork of bhyve"; faults are injected and any found bug is reproducible from a run id, input hash and virtual time. [Docs](https://antithesis.com/docs/introduction/how_antithesis_works/), [hypervisor post, 2024-03-20](https://antithesis.com/blog/deterministic_hypervisor/), [fault types](https://antithesis.com/docs/product/writing_tests/controlling_faults/fault_types/), [assertions](https://antithesis.com/docs/product/writing_tests/assertions/), [pricing](https://antithesis.com/pricing/).

- We take: the ideas. A seed that makes every run replayable; faults injected in the twins (a code host that fails mid-step, a worker that dies, a limit hit); `always` and `sometimes` assertions, where `sometimes` proves that a scenario really reaches the state it claims to test.
- We do not take: the platform. It is SaaS with no self-hosting statement found, Teams is a beta at $2,249.25 per month for the first month and $2,999 regular, with 2,400 compute hours (pricing page, 2026-11-25), and a new account and spending need the owner (`AGENTS.md`, "Ask before you do it"). Our non-determinism lives in models, and the `fake` harness already removes it.
- Influenced: decision 14 in `docs/decisions.md` and plan items X1, X2.
- Not opened: Docker Compose and Kubernetes setup pages, the Go instrumentation pages, the licence of the hypervisor and of the Go SDK repository. The platform has no stated self-hosted option; that is a reading of the pricing page, not a statement on it.

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

## Mentioned, not researched

- [Clio](https://arxiv.org/abs/2412.13678): cited in ADR 0016; only its title was opened.
- [Harbor](https://github.com/laude-institute/harbor) and Terminal-Bench: we use the Harbor task format (design section 20); the framework itself was not evaluated.
- Hegel and Bombadil: named on Antithesis's pricing page as its open-source tools; not opened.
- TLA+ and mutation testing: named in design section 19 as not taken now; not evaluated.

## Tools we use

- Codex and DeepSeek (through Codex's `base_url`) as worker providers: the `codex` harness (PR #66); ADR 0012 covers their limits. The live run is not done.
- [Playwright Test Agents](https://playwright.dev/docs/test-agents) and [playwright-cli](https://playwright.dev/docs/getting-started-cli): acceptance runs; not the Playwright MCP.
- mise (pinned toolchains, ADR 0009) with Sigstore's trust root `tuf-repo-cdn.sigstore.dev` in the install egress list, River, sqlc, goose, testcontainers, gitleaks, CodeQL.
