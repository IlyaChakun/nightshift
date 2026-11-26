# Factory techniques

Format: `docs/research/README.md`. Read 2026-11-26 unless stated.

## Scenarios held outside the code (StrongDM "scenarios")

- **Problem**: tests live in the codebase the agent edits, so an agent can rewrite them to pass.
- **How it works**: end-to-end user stories are stored outside the repository, like a holdout set, and judged by a model flexibly. A "satisfaction" figure is the share of observed runs that likely satisfy the user.
- **Trade-offs**: stops test tampering; needs a model judge, which is noisy and costs tokens.
- **Sources**: [factory.strongdm.ai](https://factory.strongdm.ai/) (Feb 2026, a company's own account: experimental); `docs/design.md` section 19.
- **Fit**: same as our hidden scenarios; the Verifier gets them and Builder never does.
- **Status**: partial. The visibility rule, jobs-API filter and the skill `hidden-scenarios` exist; storage and live acceptance are open (plan phases 5, 6). Satisfaction score: not applicable at our volume.
- **Proposal**: none beyond the plan.

## Digital Twin Universe

- **Problem**: third-party services are rate-limited, costly and cannot be made to fail on demand.
- **How it works**: build behavioural clones from API contracts and observed edge cases, then check each clone against the live service until differences stop appearing.
- **Trade-offs**: tests at any volume, with injected failures; a twin that drifts from the real service gives false confidence.
- **Sources**: [/techniques/dtu](https://factory.strongdm.ai/techniques/dtu) (experimental).
- **Fit**: our twins serve the T4 and T5 scenarios.
- **Status**: partial. Twins exist for GitHub, Linear and Slack; the "validate against the live service" step does not.
- **Proposal**: a contract test per twin call that runs against the real service on demand (T8, opt-in), after X2 uses the twins for faults.

## The filesystem as memory

- **Problem**: context is short-lived; state must outlive a session.
- **How it works**: agents write indexed Markdown and files with meaningful names, and reload them with search; they periodically reorganise the tree.
- **Sources**: [/techniques/filesystem](https://factory.strongdm.ai/techniques/filesystem) (experimental).
- **Fit**: the `artifacts` volume, handoff and memory page do this, with a size cap and a source for every entry.
- **Status**: implemented (ADR 0005). Proposal: none.

## Shift work (interactive, then non-interactive)

- **Problem**: clarification loops waste time when the intent is already complete.
- **How it works**: split work into interactive growth while intent forms and non-interactive runs once spec and tests are complete.
- **Sources**: [/techniques/shift-work](https://factory.strongdm.ai/techniques/shift-work) (experimental).
- **Fit**: this is Lead (interactive) then Builder and Verifier (non-interactive).
- **Status**: implemented. Proposal: none.

## Pyramid summaries

- **Problem**: many items do not fit in context.
- **How it works**: reversible summaries at several zoom levels; an agent scans the smallest and expands only the interesting ones; map, cluster, reduce.
- **Sources**: [/techniques/pyramid-summaries](https://factory.strongdm.ai/techniques/pyramid-summaries) (experimental). The design's clustering of findings (`internal/learn`) is a close relative.
- **Status**: missing; **Proposal**: wait until Lead reads dozens of documents at once (`ns search`, phase 5).

## Gene transfusion and semantic ports

- **Problem**: reuse a proven pattern from another codebase or language.
- **How it works**: extract the pattern's structure, invariants and edge cases from an exemplar, write an equivalent in the target, validate with behavioural tests.
- **Sources**: [/techniques/gene-transfusion](https://factory.strongdm.ai/techniques/gene-transfusion), [/techniques/semport](https://factory.strongdm.ai/techniques/semport) (experimental).
- **Status**: not applicable to the core. A Lead skill if a tenant wants it.

## Pipeline graph with goal gates (Attractor)

- **Problem**: a coding pipeline needs a declared flow, retries and a way to refuse to finish while a goal is unmet.
- **How it works**: a DOT graph; nodes are LLM tasks, tools, human waits and fan-outs; edges are chosen deterministically (condition, label, weight, then lexical order); a `goal_gate` node must succeed before the exit node; a JSON checkpoint after every node lets a run resume. The repository holds three specs and no code, under Apache-2.0 ([attractor](https://github.com/strongdm/attractor), spec read in part: first 40k characters of each file).
- **Trade-offs**: declarative and portable; a second language for flow next to our Go state machine.
- **Fit**: our gates are code (ADR 0003) and our flow is `internal/flow`; we do not take a graph file.
- **Status**: implemented in Go (gates, retries, checkpoint in the book). From its coding-agent loop spec we take **loop detection** (a repeated identical tool call ends the turn with a steering message; details of the spec's rule are unverified).
- **Proposal**: R3.

## Resume with at-least-once tools (ADK)

- **Problem**: after a crash, a workflow resumes, and a step may run twice.
- **How it works**: completed events are logged; on resume each agent's completion state is restored; "tools run at least once", so tools must check for duplicates ([ADK resume](https://adk.dev/runtime/resume/); Go support not verified).
- **Fit**: the same rule holds for the conductor and the train.
- **Status**: partial (outbox keys). **Proposal**: R2.

## Evaluating the path, not only the result (ADK eval)

- **Problem**: an agent can reach a right answer by a wrong or wasteful path.
- **How it works**: ADK scores the tool-use trajectory against an expected one, plus rubric-based quality, with `*.evalset.json` files ([adk.dev/evaluate](https://adk.dev/evaluate/), part model-summarised; Python examples only).
- **Status**: missing; the polygon records solve rate, cost and time only. **Proposal** (E2, phase 7): record the tool-call sequence in a polygon run and flag a run that took more than twice the reference path.

## Record and replay of live calls

- **Problem**: a hand-written fake drifts from the real model's output.
- **How it works**: ADK's `conformance record` saves live requests and tool calls as a baseline, and `conformance test` replays them (Python-first; same page).
- **Status**: missing. **Proposal** (E1, phase 7): a `fake` harness mode that replays a recorded transcript from a live polygon run.

## Agent graph frameworks (LangGraph, eino, ADK)

- **Problem**: structure for multi-step and multi-agent flows.
- **How it works**: LangGraph is Python, state graph with checkpointers (Postgres, SQLite, in-memory lost on restart); eino is Go with graphs, a ReAct agent and interrupt-resume; ADK has sequential, loop and parallel agents and graph workflows.
- **Fit**: roles run through CLI harnesses (ADR 0002); frameworks would duplicate `Harness`. **Status**: not applicable. Ideas taken: a snapshot per thread (our handoff), at-least-once rules (R2).

## Not read

StrongDM's `/principles`, `/products` and `/weather-report`; Attractor's lint rules, events and backoff presets; eino's checkpoint docs; LangGraph's interrupts and durable-execution pages; SWE-agent, OpenHands and Agentless papers, METR's reward-hacking report, Anthropic's long-running harness article, Spec Kit and Kiro for specification-driven work. They stay under "Mentioned, not researched" in `docs/prior-art.md` until read.
