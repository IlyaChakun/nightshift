---
status: accepted
date: 2026-11-26
---

# Choose the model and reasoning effort per role in config

## Context and Problem Statement

The owner asked on 2026-11-26 who picks Lead's model and thinking level. Nobody did. A provider could name a `model`, but the `claude` adapter never passed it, so every Claude run, Lead included, used the CLI's default model. That default changes between CLI versions ([model config](https://code.claude.com/docs/en/model-config), 2026-11-26: "Before v2.1.280, `default` resolved to Sonnet 5 on Pro…"). No role set an effort. So the strength of the factory's judgment roles depended on whichever CLI version was installed.

## Considered Options

- The role picks a model per harness and a neutral effort. The tenant overrides both per role. A provider's own `model` is a pin.
- The model only on the provider: one provider per model (for example `claude-sub-opus`, `claude-sub-sonnet`), with the tenant's provider chains choosing per role.
- Leave it to the agent (for example a CLI advisor or auto mode).

## Decision Outcome

Chosen option: role and tenant settings over provider pins. One subscription then stays one provider with one session at a time (design section 12). Lead and Verifier get a strong model without a new provider. A tenant changes it with one line.

- `roles/*.yaml`: `model: { <harness>: <full id> }` and `effort: low|medium|high|xhigh|max`. Lead and Verifier ship with `claude: claude-opus-5-5` and `high`. Builder sets neither until the polygon decides.
- `tenant.yaml`: `roles: { <role>: { model: { <harness>: <id> }, effort: <level> } }` overrides the role's values.
- A provider's `model` wins over both. It is all that provider serves, for example an OpenAI-compatible endpoint.
- The schemas reject a model id without a version digit, so an alias such as `opus` or `sonnet` cannot drift.
- The adapters map the values: `claude --model --effort`, and `codex -m` with `-c model_reasoning_effort`. The job carries both.

### Consequences

- Good: the model and effort of every run come from reviewed config, and a CLI update cannot change them.
- Good: there is no vendor name in the core. Ids live in role files and tenant config, keyed by harness.
- Bad: a pinned id must be moved by hand when a model is retired. Claude Code then falls back automatically, and the run still works.
- Bad: a provider without a model and a role without a model for that harness still get the CLI default. This is visible in the job's `model`, which stays empty.

### Confirmation

- `TestModelOfLayersProviderTenantAndRole` tests the layering.
- `TestArgsPinTheModelAndEffort` and the codex run tests check the flags.
- Schema tests reject aliases and unknown levels.
- A live run on the subscription confirms `--model` there. It is open until the Mac runs it.

## Pros and Cons of the Options

### Role and tenant settings, provider pins

- Good: one place per concern. It works for every harness and keeps the subscription a single provider.
- Bad: three layers to explain. The runbook and the `configure-tenant` skill list them.

### Model only on the provider

- Good: no new keys.
- Bad: two providers on one subscription break the one-session rule, or need a shared pool and concurrency logic.
- Bad: no neutral effort.

### Leave it to the agent

- Good: nothing to build.
- Bad: no control and no audit, which is the problem the owner raised.
