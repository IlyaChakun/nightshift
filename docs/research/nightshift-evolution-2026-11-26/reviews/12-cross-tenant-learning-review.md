# Cross-tenant learning and knowledge promotion review

Keep learning scoped to its source tenant. The repository provides tenant-scoped search and memory. Sharing a useful rule with another tenant introduces a separate consent, review and revocation decision.

Reviewed PR #351 against `8171d5ea85723b73185fd65acb27d4ded7134b82`.

| Claim | Disposition on main |
| --- | --- |
| One instance can host several tenants inside a single trust domain | Confirmed design boundary. The several-tenant stream records an outstanding live two-tenant exercise. `docs/adr/0015-several-tenants-on-one-instance.md:31`, `docs/plan/tenants.md:11`. |
| Existing learning is a basis for deliberate reuse | Partly. Search filters tenant and visibility. Lead checks the source of a proposed memory and writes it under the task's tenant. The clusterer groups recurring findings supplied by its caller. None of these operations authorizes export. `internal/search/search.go:92`, `internal/dispatch/leadmemory.go:43`, `internal/dispatch/leadmemory.go:125`, `internal/learn/cluster.go:60`. |
| A maintainer report permits cross-tenant learning | Rejected. ADR 0016 specifies opt-in, previewed counts. Phase 7 still lists the report as open. Recipient consent and rights to a reusable artifact are separate questions. `docs/adr/0016-improvement-reports-per-tenant-and-per-operator.md:24`, `docs/plan/phase-7.md:13`. |
| A synthetic artifact is safe and beneficial to share | Unverified. This PR records neither a reuse experiment nor measured disclosure risk. |

**Take, phase 7.** Keep source findings and lineage local. A tenant can adopt its own check or fixture through the existing fix ladder. Ask for a concrete repeated problem across consenting recipients before funding a transfer mechanism.

**Later, phase 7 and the several-tenant stream.** Once two-tenant isolation is checked live, try one manually reviewed, deliberately synthetic check between the synthetic tenants. Record source authorization, recipient opt-in, version, rights, negative and unrelated cases, false positives and review time. Verify that revoking a version prevents new adoption and identifies existing recipients. Keep private lineage outside the portable artifact.

**Reject now.** Shared raw search, embeddings, transcripts, automatic publication and a central service expand the trust boundary without measured value.

NIST recommends evaluating “the potential risks that releasing de-identified data might create”. The publication concerns datasets. Applying that caution to a software fixture is an inference. Read 2026-11-26. [NIST SP 800-188](https://csrc.nist.gov/pubs/sp/800/188/final)

**Checked** ADRs, current tenant and phase 7 plans, search filter, memory writes and clustering. **Inferred** Phase and pilot design. **Confidence** High for the existing boundaries. Live isolation, rights for actual material, transfer efficacy and recall of exported copies remain unverified.
