# Observability plane

> Part of the [Nightshift design](../design.md). What the factory measures about itself and the products it builds: signals, metric definitions, read models, health, alerts and diagnostics. It decides nothing: Control decides from the book and policy, and Experience presents the result (section 15).

## 27. Signals, metrics and diagnostics

**Who owns what.**

- Observability owns the signals, the definition of each metric, the read models built from the book, health and alerting.
- Experience owns where a person sees them: the Home tab, the morning summary, `/ns status`.
- Control owns every decision. A read model or a dashboard never authorizes a merge, a release or a gate (ADR 0026). This is a rule, not an import check. The archtest policy holds only the other direction: Observability may import nothing from Control but `flow`, so it cannot drive the task flow.

**Signal categories.** The "Today" column names what exists; the rest is direction, linked to its research proposal. A stack that can show a metric does not mean the metric exists.

| Category | What is observed | Today | Direction |
|---|---|---|---|
| Factory health | queue depth, worker health, API latency, resources | traces per task; the watchdog's alerts | health dashboards |
| Agent performance | tokens, duration, retries, tool errors, stalls | job records with model, provider, session and transcript | per-role trends |
| Delivery and quality | lead time, acceptance, escapes, regressions, rollbacks | the morning summary from the book (`summary`); the task timeline dashboard | |
| Economics and attention | cost per accepted task, the owner's minutes, decision latency | tokens by role on the Home tab (estimate) | [PR #345](https://github.com/IlyaChakun/nightshift/pull/345), [PR #348](https://github.com/IlyaChakun/nightshift/pull/348) |
| Learning effectiveness | paired baseline and candidate outcomes, recurrence, safety floors | rules in trial (section 10) | [PR #350](https://github.com/IlyaChakun/nightshift/pull/350) |
| Tenant products | SLO, errors, latency, business outcomes, from sources the tenant scopes | smoke and health checks after a release | production intelligence, [PR #346](https://github.com/IlyaChakun/nightshift/pull/346) |

**Evidence is not telemetry.** Traces may be sampled or lost. Anything a gate or a review needs (check reports, verdicts, the patch and its SHA) is stored in the book and in `artifacts`, never only in a trace. Authoritative counters come from SQL views over the book; OpenTelemetry explains how a number came about.

### Telemetry and diagnostics

- **A trace per task.** `nightshift` and the runner write OTLP (task, jobs, agent turns, check layers, merges); Claude Code and Codex send their telemetry to the same collector. The `trace_id` is in the card, the PR body and a commit trailer.
- **"Why did the agent do X".** Each job records the role and its config hash, skill and standard SHAs, model, provider, `session_id` and transcript path, so any decision is reproducible. The owner's actions are book events too.
- **Where to look.** Grafana on `127.0.0.1:3000` (`obs` profile): Loki, Tempo, Prometheus, and factory dashboards from the book through `grafana_ro`: task flow, quality, standards, cost (by role, provider, tenant; weekly limit share; credits), health (River queue, job errors, API latency).
- **Alerts.** In the daytime the watchdog messages the owner only about what needs action: production or smoke down, the factory stuck, limits exhausted, a loop (over N jobs per task per hour), budget burning too fast. In quiet hours only a production incident the automatic rollback did not fix wakes the owner. The rest goes to the 09:00 morning summary. An external free dead man's switch (for example healthchecks.io) emails the owner if the factory stops checking in.
- **LangSmith** trace export for evals is optional and off by default: it sends transcripts with code to a SaaS.
