# Task intake and role count

How other systems take in a task, what happens here when Lead misreads one, and whether more roles help. Checked 2026-11-26; systems are listed in `docs/prior-art.md`. Status is read from `roles/`, `skills/own/`, `schemas/spec.json`, `evals/lead/cases.yaml` and `docs/design.md` sections 6 to 8, not from a code audit.

## Verdict

- No prompt-rewriting layer. Lead already is the intake step; strengthen it (restate, one question, a clarity check, vague cases in the Lead evals).
- No CTO, PM or architect role. The one gap that more roles would not close is a wrong reading shared by every later stage; the fix is a cheap check against the owner's own words.

## Task intake elsewhere

| System | Intake | Source |
|---|---|---|
| Devin | The user supplies success criteria and context; Ask Devin scopes a task and hands a prompt to the agent. No question or approval gate is described | [guidelines](https://docs.devin.ai/essential-guidelines/when-to-use-devin), [Ask Devin](https://docs.devin.ai/work-with-devin/ask-devin.md) |
| Factory.ai | Spec mode is read-only: Droid may ask clarifying questions, proposes a plan, the user approves. Missions add an orchestrator, workers and QA validation | [spec mode](https://docs.factory.com/autonomy-and-safety/specification-mode), [Missions](https://docs.factory.com/missions/overview.md) |
| MetaGPT | A Product Manager turns the requirement into a PRD with an "Anything UNCLEAR" section | [paper](https://arxiv.org/html/2308.00352v6) |
| ChatDev | CEO, CPO, CTO design in a chat chain; a human client is optional | [paper](https://arxiv.org/html/2307.07924v4) |

All four put a plan or spec step before code. None had a published check of the spec against the original wording in what was read.

## Do models ask by themselves?

- Ambig-SWE: models "default to non-interactive behavior without explicit encouragement"; with interaction, up to 74% gain over not asking; only Claude models detected missing information well ([paper](https://arxiv.org/html/2502.13069v3)).
- ClarEval: "most agents lack systematic clarification skills" ([paper](https://arxiv.org/html/2603.00187)). Its numbers (for example Pass@1 falling from 89.02% to 8.94% under ambiguity on one model) came from a summary of the page and are unverified.

So asking has to be required by the role and measured, which is what the Lead evals do. The trade-off is over-asking: both papers measure missing questions, not annoyance, so the cost of too many questions is not measured anywhere we read.

## Do more roles help? (honest test)

- MetaGPT ablation (summary-sourced, GPT-era models): executability 1.0 with Engineer alone, 2.0 with a Product Manager, 2.5 with Product and Architect, 4.0 with four roles. The Project Manager row added nothing alone. No ablation isolates a strong model, and none separates "extra role" from "extra structured document".
- ChatDev: no role ablation found; 13.33% of runs failed on token limits and dependencies.
- Reading: the gain comes from a written spec and a design step, not from a person-shaped role. Here Lead writes both in one context, so PM, architect and CTO would split one context into several that must re-agree. Design section 6 already says when to add a role (different model, conflict of interest, a context too big). The one candidate is a Critic on another model for the spec. Whether it pays is open: test it on the polygon with seeded misreadings before adding it.
- Verdict: the owner's suspicion holds for CTO, PM and architect. It does not hold for the Verifier, which has a conflict-of-interest reason.

## Safety nets when Lead misreads

| Net | Catches | Misses |
|---|---|---|
| Spec card (Approve, Edit, Reject) | A reading the owner notices | The owner skims a Given/When/Then list; no "how I understood it" line. Only human gate for a `low` task on SLA auto-merge |
| PRD rubrics (L4, `standards/industry/docs/rubrics/prd.*`) | A missing metric, alternatives, diagram, non-functional notes, ADR conflicts | Does not read the owner's message; cannot see a wrong goal |
| Verifier review | Code against the spec | Reads the spec first; a wrong spec passes. Whether it sees the owner's message: not found in `roles/verifier.md` |
| Hidden scenarios | Code that fits the letter | Written by Lead from the same reading, so they share its error |
| Checks, CI, blast-radius review | Broken or risky code | Intent |
| Learn job (phase 7) | Patterns in owner rejections | Not built |

Gaps: (1) one reading feeds spec, scenarios and review, so errors are correlated; (2) nothing ties each criterion to the owner's words; (3) Lead evals have 3 frozen cases, none for a vague task or for over-asking; (4) no clarity gate before the spec.

## Proposals (plan items I-series, after the phase 5 thread's open PRs)

1. `lead-reply.json`: an `understood` line (at most 25 words) on every question and spec card; the owner corrects it before work starts.
2. A "task clarity" rubric (goal, done criterion, scope) in the `challenge` skill; an empty field means a question, not a spec. Questions only where the answer changes the result.
3. Spec gets `source_quote`: the owner words each criterion rests on; the Verifier gets the original message beside the spec.
4. Lead evals: vague, over-specified and already-clear cases; pass means asks on the first and does not ask on the last.
5. Later: a Critic on another model for specs, only if the polygon shows misreadings it catches.

**Not verified:** every external number above beyond the cited pages' own text; no Lead run, live or fake, was made for this note; Devin's interactive-planning page was not found.
