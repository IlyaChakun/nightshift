# Safety nets around Lead

What other autonomous software factories and coding agents do so that one agent's mistake does not reach production, and what we take. Checked 2026-11-26. Systems and links: `docs/prior-art.md`. Lead intake and role count: `docs/research/task-intake.md`; harness limits: `docs/research/harness-control.md`.

## Verdict

- Nobody we read has a net that catches "the planner understood the task wrongly" by itself. All of them put a human approval of a plan or spec in front (Jules, Kiro, Factory spec mode and Missions planning) and a fresh validator behind (Factory). We have both; the gap is that every later stage reads Lead's reading, not the owner's words.
- Most of the other mechanisms (sandbox, egress allowlist, no self-merge, one pushable branch, spend limits, audit log) we already have or have planned. Nothing new is needed there.
- Take three small things: a coverage check of the owner's message against the spec (code, $0), a second independent reading by another model family measured on the polygon before it is switched on, and a run record that shows which model and effort Lead used.
- Evidence is thin: every external page below was read through a page summariser, not as raw text, and few numbers are measured on frontier models.

## How the sources were read

All pages were fetched on 2026-11-26 through a tool that returns a model summary of the page. "Read" below means that summary, so quotes may differ from the page by a word. Abstracts (MetaGPT, ChatDev, Agentless, Bias in the Loop) were read alone. Search snippets only: CodeT, Self-Preference Bias, "Don't Judge Code by Its Cover". Not found in any source: Sweep, a published canary or preview step of any agent, eval-driven prompt changes, a measured gain from a second-model critic on code, a measured gain from sampling several plans. Vendor numbers are vendor claims.

## What controls Lead today

| Net | Where | Limit |
|---|---|---|
| Output is JSON checked against `schemas/lead-reply.json`; Lead writes only documents | design section 6, `roles/lead.yaml` | content is not checked |
| Limits per run: 20 minutes, 200000 tokens | `roles/lead.yaml` | S1 supervisor (plan) |
| Spec card: Approve, Edit, Reject | design section 7 | the owner may skim |
| `challenge` skill, clarity check (N2), PRD rubrics (L4) | design sections 6, 8 | do not read the owner's message |
| Verifier on another model family, fresh context | design section 6 | reads Lead's spec first |
| Hidden scenarios | design section 19 | written from the same reading |
| Lead evals (3 frozen cases) | `evals/lead/cases.yaml` | N4 adds vague cases |
| Model and effort | `providers.yaml` per provider order (design section 12) | no effort setting per role found in design or `internal/harness/`; the job result's record of what Lead ran with was not checked |

## Mechanisms by system

Columns: **Misread** = does it address "Lead misunderstood"; **Ours** = exists, planned, missing, not needed.

| Mechanism | Who has it | Misread | Ours | Cost | Verdict |
|---|---|---|---|---|---|
| Plan or spec approval by a human | [Jules](https://jules.google/docs) ("review and approve it before any code changes"), [Kiro](https://kiro.dev/docs/specs/) (requirements, design, tasks; Quick Spec skips gates), [Factory planning](https://docs.factory.com/missions/planning), [Symphony](https://raw.githubusercontent.com/openai/symphony/main/SPEC.md) handoff state "Human Review" | partly: a human must notice | exists (spec card) | owner time | have |
| Contract of assertions before the features | [Factory Missions](https://factory.com/news/missions-architecture), 2026-04-10, vendor account | no: the orchestrator writes it from its own reading | exists (hidden scenarios) | tokens | have |
| Fresh validator, builder never self-certifies | Factory Missions: "worse at objectively evaluating its own work"; black-box user-testing validators; 81 issues found in one Slack-clone run, 2 to 4 rounds per milestone (vendor claim) | no: validates against the contract | exists (Verifier) | dollars | have |
| Orchestrator halts and hands back when blocked | Factory Missions; [Symphony](https://raw.githubusercontent.com/openai/symphony/main/SPEC.md) "MUST NOT stall indefinitely waiting for user input" | no | exists (`escalated`, decision card) | $0 | have |
| Independent second reading of the owner's message | none found | yes | missing | one cheap call per spec (estimate: cents) | take, pilot (N6) |
| Coverage of the owner's words by the spec | none found | yes | missing | code, $0 | take (N5) |
| Quote per criterion | none found | yes | planned (N3) | small | have in plan |
| Second-model critic of the spec | none found in use; the Open SWE README suggests review gates as middleware | partly | missing (task-intake proposal 5) | one call | maybe, after N6 data |
| Several samples, vote | code only: [majority voting](https://arxiv.org/pdf/2604.15618), 2026-04, Qwen3-4B on LiveCodeBench, 37.7% to 48.1% to 52.7% with 64 samples; frontier models not found | not for prose specs | missing | 2 to 64 times tokens | skip |
| Hidden tests | StrongDM holdout scenarios ([Willison](https://simonwillison.net/2026/Feb/7/software-factory/), vendor claim); [EvilGenie](https://arxiv.org/html/2511.21654v1): held-out tests are "not foolproof" | no | exists | tokens | have |
| Agents tamper with graders | [METR](https://metr.org/blog/2025-06-05-recent-reward-hacking), 2025-06-05: o3 hacked 39 of 128 RE-Bench runs; telling it not to had little effect | no | exists (protected paths) | $0 | have |
| LLM judge limits | [Bias in the Loop](https://arxiv.org/abs/2604.16790), abstract: verdicts shift with prompt wording; EvilGenie: GPT-5 judge 16.7% false negatives on 12 true cases (tiny sample) | warns | judge calibration at 80% (design section 20) | $0 | have |
| Sandbox and egress | [Copilot](https://docs.github.com/en/copilot/concepts/agents/coding-agent/risks-and-mitigations), [Codex](https://learn.chatgpt.com/docs/agent-approvals-security) (offline by default, per-environment allowlist, GET only), [Cursor](https://cursor.com/docs/cloud-agent/security) (microVM, allowlist), [OpenHands](https://arxiv.org/html/2511.03690v2) (opt-in) | no | exists (ADR 0004, T6) | $0 | have |
| One pushable branch, no self-approve or merge, workflows wait for review | Copilot; Cursor ("nothing merges until a person reviews") | no | exists (`nightshift` pushes and merges) | $0 | have |
| Scanning of the agent's own output | Copilot: CodeQL, secret scanning, advisory check on new dependencies | no | exists (L1, CI) | CI | have |
| Spend, turn and stall limits | [Devin](https://docs.devin.ai/admin/billing/acu-limits.md) budgets; Symphony stall and turn timeouts, max turns; [SWE-agent](https://arxiv.org/pdf/2405.15793) $4 per run with auto-submit; Open SWE step-limit notice | no | planned (S1, S2) | $0 | have in plan |
| Lint at edit time | SWE-agent: 18.0% vs 15.0% without, GPT-4 Turbo on SWE-bench Lite | no | exists (`nightshift check` loop) | $0 | have |
| Risk-tiered approval | [OpenHands](https://docs.openhands.dev/sdk/guides/security) confirm-risky; [Factory](https://docs.factory.com/autonomy-and-safety/auto-run) autonomy levels, block over ask over allow, an enterprise cap | no | exists (blast radius, autonomy ladder, section 11) | $0 | have |
| Rollback | [Aider](https://aider.chat/docs/git.html) auto-commit and undo; Replit restore after its 2025-07 incident ([Register](https://www.theregister.com/2025/07/22/replit_saastr_response/)); Anthropic harness reverts with git | no | exists (revert PR, train freeze) | $0 | have |
| Preview or canary | none found | n/a | preview in phase 6 | n/a | skip canary at personal scale |
| Audit trail | OpenHands event log; Cursor, Copilot session logs; Aider commits | no | exists (`events`, job results; S2 timeline) | $0 | have |
| Tests of the tests | [Antithesis](https://www.globenewswire.com/news-release/2026/04/23/3280089/0/en/Antithesis-Teaches-AIs-To-Correct-Their-Own-Output.html) agent-run mutation testing; rqlite: 11 of 13 properties falsified (vendor press release) | no | missing | compute | maybe, phase 7 |
| Drift: reconcile state every tick | Symphony section 8.5 | no | exists (conductor tick) | $0 | have |
| Drift: model or prompt change | none found | yes | Lead evals exist; no run on a model change | evals cost | maybe |
| Do not feed new requirements mid-task | [Devin review 2025](https://cognition.com/blog/devin-annual-performance-review-2025): worse when told more after the start | partly | exists (spec freeze, new thread) | $0 | have |

## What we take

1. **N5, coverage check.** A code check that every number, name and "not" in the owner's message appears in the spec or in its out-of-scope list; a miss becomes a question, not a spec. Why: catches dropped constraints without a model. Counter-argument: word matching misses a changed meaning, so it is a floor, not a net. Inferred; no source does it.
2. **N6, independent reading.** A model of another family reads only the owner's message and lists what "done" means; a diff against Lead's criteria goes onto the spec card as "readings differ here". First measure it on seeded misreadings in the Lead evals (N4); switch it on only if it catches some Lead misses and the false alarms stay under what the owner tolerates. Counter-argument: two models disagree on wording, so the owner may drown in noise; the polygon decides. Inferred from Factory's fresh-validator argument and task-intake gap 1.
3. **L5, run record.** The job result and the task timeline (S2) show provider, model and effort for each Lead run, and `providers.yaml` can pin an effort per role. Why: the owner asked who picks model and effort; today that answer should be one line in the timeline. Counter-argument: an effort setting is a new config key per harness, and a harness without one must say so.

## Needs the owner

Raising Lead's model or effort uses more of the subscription or API budget (`AGENTS.md`: "anything that costs money" goes to the owner). L5 only records and allows a pin; the choice of a higher setting is the owner's.

## Not verified

- Every external claim above rests on a page summary; quotes may differ slightly from the pages.
- Not found, so not stated: a stuck detector in OpenHands, a published confidence rating or plan approval in Devin, Symphony's "proof of work", Codex review, Sweep, Kiro approval gates in the normal flow, Factory spec mode (not fetched).
- Our side was read from `docs/design.md`, `docs/research/*.md`, `roles/lead.yaml` and `docs/plan.md`, not from running code.
