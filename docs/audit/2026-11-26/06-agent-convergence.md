# NS-A06 — Convergence, context engineering and corrective strategies

**Priority:** P2.  
**Status:** capability improvement; establish baseline and inspect existing supervisor, return counters, memory and budgets first.

## Contents

- [Problem](#problem)
- [Proposed invariants](#proposed-invariants)
- [Design options](#design-options)
- [Context and memory](#context-and-memory)
- [Candidate recovery strategy](#candidate-recovery-strategy)
- [Acceptance criteria](#acceptance-criteria)
- [Follow-up source check and retention](#follow-up-source-check-and-retention)
- [Decision brief — evidence-driven convergence, 2026-11-26](#decision-brief--evidence-driven-convergence-2026-11-26)

## Problem

Retries and bounded iteration prevent infinite execution but do not distinguish progress from stagnation. The earlier audit identified existing supervisor and escalation mechanisms; it did not establish that they detect repeated semantic failures, regressions or conflicting requirements. Additional model calls can increase cost without improving the candidate.

## Proposed invariants

- Each iteration has durable evidence of what changed, why it ran and how it performed against stable acceptance criteria.
- Repeated attempts are bounded by wall time, tokens, cost and relevant operation limits; the agent cannot expand its own budget or relax required checks.
- A change of strategy is based on observable evidence and recorded; failure to converge yields a diagnosable terminal or blocked state, not false success.
- Memory and tool output are untrusted inputs; tenant/role scope and provenance must be respected. Untrusted content must not become policy or trusted instructions.

## Design options

| Option | Value | Trade-offs |
|---|---|---|
| Normalize finding identities, record pass/fail transitions, patch digests and attempt cost | Cheap progress/stagnation signals | Finding wording changes and nondeterministic checks can produce false signals |
| Deterministic strategy policy (retry, refresh context, rollback, alternate harness, replan, escalate) | Bounded and explainable | Requires good classification and thresholds; avoid hardcoding speculative cases |
| LLM-assisted diagnosis of repeated failure | Useful for semantic causes and spec ambiguity | Additional cost, nondeterminism and susceptibility to misleading context; cannot override policy |

Start with instrumentation and a small deterministic escalation policy; tune on observed failures. Track unresolved required findings, regression severity and independent acceptance results rather than treating raw finding count as a monotonic score. Preserve best known verified checkpoints, but do not allow partial success to authorize release.

## Context and memory

Inspect retrieval scope, freshness and token limits. Store source provenance, project and role ownership, confidence and revision; distinguish factual artifacts, decisions, hypotheses and untrusted text. Prefer targeted retrieval over ever-growing transcripts. Validate policy-critical facts against authoritative state. Quarantine untrusted tool/repository content from instruction channels. Do not persist secrets or hidden scenario details into Builder-accessible memory. Turn recurring failures into deterministic tools/tests where feasible (rules into tools, words last), not automatically into global prompt instructions.

## Candidate recovery strategy

Classify infrastructure failures separately from code defects, spec ambiguity and architectural failure. Retry transient infrastructure failures with backoff. For repeated defects, give Builder a targeted, redacted failure summary; refresh context or switch harness if evidence justifies it. For persistent architectural or requirement conflicts, return to Lead or request an authorized clarification. Roll back regressions to a known checkpoint. Stop on budget exhaustion or policy violations.

## Acceptance criteria

Construct reproducible cases: same finding repeated with changed wording; genuine incremental progress; alternating patches; regression after a partial pass; conflicting requirements; transient provider failure; stale/poisoned retrieved text; exhausted budget. Verify appropriate strategy, bounded cost, durable reason codes and no relaxation of acceptance. Measure accepted completion rate and cost before/after policy changes on fixed representative tasks. Do not adopt more elaborate planning or routing unless it improves the baseline.

**Dependencies:** NS-A03 attempt identity/durability; NS-A05 parent replanning; NS-A07 benchmark and holdout strategy.

## Follow-up source check and retention

This dated external review is a candidate finding record, not the living design. Preserve the original findings and append a disposition or link to implementation evidence; do not delete this record merely because an issue is resolved. Superseding or relocating it requires an explicit owner decision.

**Checked:** pinned Nightshift base [9473fa4](https://github.com/IlyaChakun/nightshift/tree/9473fa4bc294825c110b6ec236da7110d0ff0e15), 2026-11-26; `docs/status.md` and `docs/research/README.md`. Current status lists ADR 0019 supervisor and model/effort per role; research README already identifies trajectory scoring and replay as gaps. Build on these, do not duplicate them.

**Inferred:** priorities and proposed solutions; no tests or live runs performed. **Confidence:** medium.

## Decision brief — evidence-driven convergence, 2026-11-26

**Disposition:** retain ADR 0019 and the existing bounded repair loop. Fix finding ingestion and status semantics before introducing semantic stagnation detection or learned routing. Elevate these concrete evidence-integrity issues to P1; the optional adaptive strategy remains P2.

### Pinned implementation findings

| Evidence | Disposition |
| --- | --- |
| [Supervisor](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/harness/supervise/supervise.go#L18-L100) checks consecutive identical tool/input pairs, stall and reported spend; [end check](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/harness/supervise/supervise.go#L163-L194) checks final tokens. | Intra-turn controls already exist. They do not detect alternating calls, equivalent calls with modified input, or cross-job semantic stagnation. Preserve limits and actual enforceability caveats (S1/T8, #345). |
| [Builder loop](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/build.go#L29-L168) bounds returns using events; [Verifier loop](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/verify.go#L106-L156) and [Decide](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/verdict.go#L29-L57) bound returns independently. | Existing termination mechanisms; count attempts/returns consistently and make mandatory counters durable (A03). A returned verdict count is not a measure of improvement. |
| [Jobs API completion](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/jobsapi/server.go#L399-L409) calls storeMemory; [memory.go](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/jobsapi/memory.go) validates and stores role findings with tagged JSON fields. [consumeVerdict](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/verify.go#L106-L132) separately inserts returned findings using an untagged Go struct for RuleID and LayerCaught. | Concrete decoder mismatch for schema keys rule_id and layer_caught; likely duplicate ingestion of the same logical finding through two paths. Existing SQL allows repeated fingerprints and empty strings in non-null text fields. Use one authoritative typed ingestion path; verify actual duplication with Postgres. |
| [enqueueBuilder/openFindings](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/build.go#L279-L306) sets open findings to fixed after enqueue, explicitly meaning handed over. | Fixed currently means delivered, not independently resolved. Do not compute closure or progress from this status; preserve unresolved obligation until comparable verification or authorized adjudication. |
| [SARIF fingerprints](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/check/sarif.go) use tool-supplied identity or rule/path/message; [Verifier fingerprint](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/verify.go#L118-L128) hashes rule/path/message. | Line shifts are accommodated but changed wording can split the same defect. Keep immutable occurrence IDs and conservative issue grouping separate. |
| [startFrom](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/build.go#L364-L390) continues from an open PR tip, otherwise base; [handoff](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/handoff.go) caps unverified summaries at 2 KB and falls back to recent event kinds. | Existing continuation and compact hints; not a best-known independently verified checkpoint protocol. A stale summary cannot authorize release. |
| [Research return](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/research.go#L18-L75) permits two needs_research returns for eligible tasks but deliberately refuses expanded specs and siblings without owner words. | Existing anti-loop mechanism; multi-task replan needs objective linkage and owner-approved revisions (A05), not unrestricted new work. |

### Reproduced decoder defect (isolated mechanism)

Executed with Go 1.23.2 linux/amd64, local toolchain, GOPROXY=off, no network. This reproduces the standard-library decoding behavior of the anonymous struct in consumeVerdict, not the full Nightshift integration.

```go
package main
import ("encoding/json"; "fmt")
func main() {
    var r struct {
        Findings []struct {
            RuleID, LayerCaught, Severity string
        } `json:"findings"`
    }
    raw := []byte(`{"findings":[{"rule_id":"test.invariant","layer_caught":"L5","severity":"block"}]}`)
    if err := json.Unmarshal(raw, &r); err != nil { panic(err) }
    fmt.Printf("%q %q %q\n", r.Findings[0].RuleID, r.Findings[0].LayerCaught, r.Findings[0].Severity)
}
```

Observed with equivalent struct including Category, Message and Location: untagged rule="" layer="" severity="block" path="a.go"; adding explicit json:"rule_id" and json:"layer_caught" yielded rule="test.invariant" layer="L5". Schema [finding.json](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/schemas/finding.json#L8-L26) requires those snake_case keys. [Migration 00005](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/book/migrations/00005_findings_notes.sql#L4-L26) has no nonempty rule/layer constraint. The actual DB duplicate/malformed-row behavior remains untested. Prefer consuming already validated persisted findings rather than simply tagging a second decoder. Test exact occurrence count, all field values, verdict validity and replay under A03.

### Research basis

Primary paper abstracts and source search excerpts checked 2026-11-26; not full-text reproductions.

- [Self-Refine, NeurIPS 2023](https://proceedings.neurips.cc/paper_files/paper/2023/hash/91edff07232fb1b55a505a9e9f6c0ff3-Abstract-Conference.html) studies iterative model feedback/refinement. Useful alternative to a single attempt, not proof that repeated same-agent reviews converge on software correctness.
- [Reflexion, NeurIPS 2023](https://papers.neurips.cc/paper_files/paper/2023/hash/1b44b878bb782e6954cd888628510e90-Abstract-Conference.html) reports gains from feedback and verbal memory on selected coding/reasoning/decision tasks. Transfer bounded failure summaries, not unverified global memory promotion.
- [Huang et al., ICLR 2024](https://proceedings.iclr.cc/paper_files/paper/2024/hash/8b4add8b0aa8749d80a34ca5d941c355-Abstract-Conference.html) reports that intrinsic reasoning self-correction without external feedback can fail or degrade performance. It does not establish all modern coding agents fail at self-repair. Prefer independently observed behavior over self-confidence.
- [Anthropic, Demystifying evals for AI agents](https://www.anthropic.com/research/demystifying-evals-for-ai-agents) distinguishes traces/graders from actual environment outcomes. Judge final behavior plus trajectory efficiency; replay is not a substitute for independent tests.
- Existing [harness-control research](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/docs/research/harness-control.md) covers Attractor loop patterns, Symphony, frameworks and supervisor alternatives; its earlier capability table predates the accepted ADR 0019 and is historical. Do not reopen the framework decision solely for convergence.

### Minimal progress model (proposal)

1. Repair typed ingestion first, keep immutable finding occurrences linked to job/attempt and revision. Separate open obligation, delivered, claimed addressed, verified resolved, disputed, adjudicated false positive and superseded. Decide whether to derive delivery from job inputs or add a small record; do not change canonical schema without an ADR. Only an independent comparable test/review or authorized adjudication closes the obligation. Missing finding in a failed/partial run is unknown.
2. Bind evaluation to task/objective, spec, base/head/tree, policy and suite identity. Preserve mandatory criterion outcomes individually, blocker severity, failing test identities, tool errors and check completeness. Stable matching uses tool ID and location/symbol/test/criterion with provenance; semantic clustering is advisory and must not merge distinct defects silently.
3. Compare consecutive comparable observations, not raw finding count. Progress includes a required failure independently resolved with no new required regression; regression includes newly failing previously satisfied criteria; unchanged/alternating behavior may indicate stagnation. A partial run or incomparable revisions cannot prove improvement. Retain known checkpoints as diagnostic recovery candidates, not release permissions.
4. Record strategy reason, allowed actions, attempt identity and remaining shared budget in durable events; reuse existing supervisor and dispatcher. Stop within existing iteration/time/spend bounds; do not let a restarted harness or role switch reset total logical limits. #345 handles physical usage; #348 handles owner attention. A01/A03 protect authority and atomicity.

### Bounded strategy and falsification

| Observation | Candidate response (only within approved budget and policy) |
| --- | --- |
| Transient provider/infrastructure failure | Existing retry/wait/fallback policy; bounded backoff and distinguish failure from code defect |
| First comparable required defect | Targeted independent evidence to Builder; continue from validated current candidate |
| Same unresolved obligation, different wording | Group provisionally; request a falsifiable test or different repair approach, not more generic critique |
| Repeated A/B candidate cycle or newly introduced required regression | Stop ordinary repeat; compare known checkpoints, refresh context, or escalate |
| Spec/architecture conflict | Return to Lead where existing policy permits; multi-task replanning requires A05 versioned objective and owner authorization |
| Stale/poisoned handoff, missing independent evidence, budget exhausted | Re-read authoritative state, restrict access, stop or escalate; never relax checks |

Start with shadow classification on frozen task histories (sanitized, tenant-scoped, including failed runs). Compare existing loop vs a deterministic evidence-driven policy on representative paired tasks. Hold model, acceptance suite and total allowed budget fixed; repeat stochastic runs as feasible. Use independently accepted outcomes, required regression escapes, cost/human minutes per accepted objective, false stagnation stops, extra attempts on genuine progress and unresolved obligations. Evaluate a model-assisted diagnosis only when deterministic classification cannot distinguish frequent meaningful failure classes; use independent review and log the additional cost.

Required tests: schema-valid verdict persists exactly once with correct rule/layer/location; retry after crash does not duplicate logical obligation; enqueue does not resolve it; changed message preserves candidate grouping but different defects remain distinct; failed/partial check cannot close; genuine progress is not falsely stopped; alternating candidates, transient provider outage, replan denial, stale checkpoint and budget exhaustion end in the intended bounded disposition. All mandatory acceptance tests still execute; hidden evidence never enters Builder-visible context.

**Go/no-go:** confirm the ingestion/status bugs with repository integration tests first, repair those in separate runtime PRs, then collect a baseline. Add one bounded strategy only if paired evidence improves acceptance or resource use without quality/safety regression. Keep negative results; no autonomous expansion of limits, global memory or policy.

**Checked:** pinned source paths, ADR 0019, finding schema/migration, local decoder fixture, primary abstracts/search excerpts. **Inferred:** full duplication effect, progress model and policy. **Confidence:** high for decoder mechanism and fixed-on-enqueue semantics, medium for actual DB effects and proposed gain. **Not verified:** full Nightshift runtime, live harness controls, integration race, real spend, full research papers, local make verify or CI.
