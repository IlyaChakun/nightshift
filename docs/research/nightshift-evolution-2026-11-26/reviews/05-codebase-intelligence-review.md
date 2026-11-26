# Code change impact analysis review (PR #344)

Test an optional Go package impact brief only after a brownfield task exposes a missed consumer. A local fixture confirms that build tags and external test packages change what the extractor sees. Keep mandatory verification independent of every edge the brief includes or misses.

Checked main `8171d5ea85723b73185fd65acb27d4ded7134b82` against research head `5ab0f47`.

| Claim | Disposition | Evidence at checked main |
| --- | --- | --- |
| Code navigation belongs to the harness. Book search indexes scoped knowledge. | Confirmed. Design excludes code embeddings. Search queries book documents by tenant, visibility and kind. | `docs/design.md:962`, `docs/design.md:1004`, `internal/search/search.go:90` |
| Lead and Verifier already have a place for revision-bound context | Confirmed. Lead supplies its base and project instructions. Verifier receives both revisions. Its role asks about missed callers. | `internal/dispatch/lead.go:215`, `internal/dispatch/verify.go:78`, `roles/verifier.md:23` |
| Task dependencies and memory have distinct meanings from source edges | Confirmed. The graph validates dependencies in a spec. Memory checks cited statements against their sources. | `internal/dispatch/deps.go:41`, `internal/dispatch/leadmemory.go:43` |
| The worker runs configured checks and Nightshift tests its full Go tree | Confirmed as configuration. A map has no role in selecting the mandatory suite in these paths. | `internal/runner/check.go:66`, `Makefile:59` |

## Feasibility and decisions

On 2026-11-26 a temporary five-package Go module on darwin/arm64 with Go 1.27.4 reproduced the research result. With `go list -deps -test -json ./...`, reverse imports included the API, service, core and two test identities. Adding `-tags=special` added the conditional consumer. The external test passed before a controlled constant change and then failed in `TestValue` with the message `changed`. This validates the fixture mechanism only.

- **Take — phase 5, bounded experiment.** Compare harness navigation to an optional stack skill on paired tasks with an independently labelled set of affected consumers. Include renames/deletions, external tests, tag-only edges, runtime references and a no-impact control. Tie the brief to both trees, build inputs and extractor version. Mark missing, inferred or truncated edges. Measure recall, false positives, missed tests, acceptance, cost and human effort. Keep ordinary navigation if results are comparable.
- **Later — phases 5 and 10.** Add typed or runtime hints only for repeated misses. Cache only after measuring repeated cost, keyed by tenant, repository, revisions and build configuration. Re-evaluate multi-repository contracts independently.
- **Reject — all phases.** Core-owned indexing, GraphRAG, a new service, using the task graph as an import graph, or reducing required CI would contradict design §13 or exceed the experiment.

## Primary evidence

Read 2026-11-26. Go documents “test binaries (for packages with tests)”. Its RTA call graph “does not include edges for calls made via reflection”. These limits matter when interpreting an empty graph. [Go list](https://pkg.go.dev/cmd/go#hdr-List_packages_or_modules), [Go RTA](https://pkg.go.dev/golang.org/x/tools/go/callgraph/rta).

**Checked** Design, runner, search, dispatch, role instructions and local fixture. **Inferred** An advisory skill is a reversible trial. **Unverified** Real-project recall, cross-service completeness, other languages and workflow gains. **Confidence** High for the fixture, medium for transfer.
