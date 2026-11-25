---
status: accepted
date: 2026-11-25
---

# The factory reports what to improve per tenant, and tells its maintainer only what an operator allows

## Context and Problem Statement

The owner asks whether the factory needs what the people building it use: memory at several levels, analysis of sessions and logs, proposals of improvements to a tenant, and a way for an operator to send logs or improvement ideas to the maintainer of Nightshift. Design section 10 already turns findings into fixes (nightly learn, the fix ladder, trial rules, "Lessons of the week"), section 13 has the memory levels, section 11 the autonomy ladder, section 15 the morning summary. What is missing is (1) a signal source from what agents did, not only from checks and reviews, (2) a periodic "what to improve" report per tenant that covers process, cost and configuration, not only standards, and (3) any path from an instance to the maintainer. In a work instance the operator is not the tenant's owner, and the maintainer is a third party, so tenant data must not travel without consent.

## Considered Options

- Do nothing: findings and the weekly "Lessons of the week" card stay the only loop; an operator reports problems by hand
- Counters first: friction counters from the book feed the existing learn loop; one weekly per-tenant report; a local, opt-in operator report that is shown before anything leaves and is sent by hand (chosen)
- Full pipeline now: a model reads all transcripts, and an uploader sends them to a collector run by the maintainer

## Decision Outcome

Chosen option: "Counters first", because the book already holds the events, so counters cost SQL and no model, and nothing leaves the machine until a person has read it.

- Friction signals (retries, loops, limit hits, questions the memory page already answers, tokens per merged task) become `process` findings. A model reads a small sample of transcripts only for a cluster that crossed a threshold, and only proposes (design section 10, item 10).
- One weekly per-tenant report, part of the "Lessons of the week" card, with at most three proposals, each on the fix ladder or as a PR to the tenant's `.nightshift/` files (item 11).
- The operator report has two tiers: per-tenant aggregates inside the instance, and a counts-only report for the maintainer that is off by default, previewed with `nightshift report --preview` and sent by hand until an egress host is approved (item 12).
- Phase 7 carries the work; nothing here starts before phase 5 closes.

### Consequences

- Good: the same loop that fixes standards now sees process and cost problems, without a new store or service.
- Good: a tenant's text, code and transcripts never reach an operator or the maintainer without the tenant's own setting.
- Bad: counts alone miss problems that only the text shows. Mitigation: the maintainer asks, and the operator shares a redacted excerpt by hand.
- Bad: another message per week. Mitigation: it is one section of an existing card.
- Risk: transcript text is foreign text. Mitigation: sampled output only becomes a finding or a proposal and goes through the constitution (section 10, item 8).

## Pros and Cons of the Options

### Do nothing

- Good: no work.
- Bad: process and cost problems (a retry loop, a provider that keeps hitting its limit) appear only when the owner notices them, which is the escape the design wants to shift left.

### Full pipeline now

- Good: the richest signal.
- Bad: transcripts carry code and customer text; an uploader needs a new egress host, a secret and a collector to run and secure; with a handful of tasks a day the volume does not justify it.

## More Information

Prior art, read on 2026-11-25:

- [Go telemetry](https://go.dev/doc/telemetry): data stays local by default, upload needs `go telemetry on`, and only named counters leave. We take the local-first, opt-in, counters-only shape and the local view; we do not take a central collector yet.
- [LangSmith Insights Agent](https://www.langchain.com/blog/from-traces-to-insights-understanding-agent-behavior-at-scale): clusters traces into usage patterns and failure modes. We take the clustering of agent behaviour; we do not take a hosted service that receives transcripts (section 15, LangSmith stays off). Whether the service is hosted is not stated on the page: unverified.
- [Clio](https://arxiv.org/abs/2412.13678): privacy-preserving aggregate insight into real use. We take aggregation with a minimum group size; we do not take model summaries of private text for an outside reader. The paper's details were not opened: unverified beyond its title.
