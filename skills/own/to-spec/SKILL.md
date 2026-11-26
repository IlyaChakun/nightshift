---
name: to-spec
description: Turn a challenged idea into the spec (schemas/spec.json) - title, goal, Given/When/Then criteria, non-goals, tags and tasks. Use when Definition of Ready is reached and the owner should see the spec card.
---

# To spec

Write the result Lead returns as `spec` in `schemas/lead-reply.json`, validated against `schemas/spec.json`.

## Rules

- **Title:** a short Conventional Commit (`feat: export the nightly report`). It names the commit and the pull request.
- **Goal:** one or two sentences, the outcome and who gets it. No solution details.
- **Acceptance criteria:** ids `AC-1`, `AC-2`...; each Given/When/Then, observable from outside, one behaviour each. Include one error or edge case. A criterion a test cannot check is not a criterion.
- **Non-goals:** what this work does not do. At least one when the scope could grow.
- **`blast_radius_tags`:** only the tags `challenge` found; never invent one to look careful.
- **`prd_path`:** where the PRD lives (`docs/prd/<slug>.md`, from `templates/prd.md`). Write the PRD file when the change is a new feature; a small fix needs none.
- **`adr_paths`:** the research record from `build-vs-buy` or `research-options`, if one was made.
- **Tasks:** use `to-tickets`.
- **Hidden scenarios:** use `hidden-scenarios` and return them as `hidden_scenarios` beside the spec; Builder never sees them.
- Add a mermaid diagram to the PRD when a flow or the architecture changes.

## Check before you send

Every criterion maps to a task; no task is without a criterion; the title matches the goal; the basis line says what you opened and how sure you are.
