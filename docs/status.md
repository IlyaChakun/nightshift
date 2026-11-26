# Status

> Phases 0–4 are built; phase 5 (Lead) is in progress. Live checks are open for several items. Next: the memory prune rule, then hidden scenarios, tracker projection, `ns search`, #support, voice.

Rewrite at the end of every session. Limit: 40 lines and 600 words. No history. Updated: 2026-11-26.

## Done

- **Phases 0–1** (#1–#43): built. Restore and restart pass on the Mac (`make drill`, 22 of 22, 2026-11-25).
- **Phase 2**: items 1–10 and the codex adapter. No token in the worker and no way past the egress allowlist are checked live.
- **Phase 3**: items 1–18 and doors I1–I5. A chat idea reached `released` on the `nightshift` tenant (#148).
- **Phase 4**: any-language path (`toolchain` service, ADR 0010; packs js, java-maven, java-gradle; skills web-dev, mobile-dev; `deps check`). L4 rubrics exist, but the worker image lacks `standards`, so the runner does not call them yet (the Dockerfile is owner-gated). `nightshift skills sync` works; `skills/vendor.yaml` holds four starter skills.
- **Phase 5 so far**: morning `nightshift digest` and the 09:00 Slack post; `/ns status|memory|remember`; `needs_research` returns to Lead; a PR merged or closed outside the factory ends its task; `nightshift cancel <task>`; role overrides in `.nightshift/roles/<name>.yaml`; memory page (`nightshift memory`, 40 entries, 4 KB, daily prune).
- **Polygon** (ADR 0011): `nightshift eval run` grades one synthetic task offline. **Deploy targets** (ADR 0014): designed, no phase 6 code yet.
- **Several tenants** (ADR 0015): `config validate`, skill `configure-tenant`, `capacity.tasks`, fair claims, a conductor loop and a GitHub token per tenant (M2, M4), runbook "Several projects on one instance". M3 is half built: HOME per tenant waits for the phase 5 thread.
- **Promise checks** (plan Q0): `cmd/nightshift/promises_test.go`, skill `self-audit`, AGENTS.md "Done means checked". 21 schema keys are still unread (plan Q1, Q2).
- **Audit** (A1–A5) and **setup** (S1–S5) done. ADR 0018 says no to Temporal; plan R1–R3. **Docs style**: skill `write-docs`; `repo-check` limits words as well as lines.

## Next

1. Prune rule for memory entries whose fact moved into the repo (design section 13).
2. Hidden scenarios storage, tracker projection, `ns search`, #support, voice; M3 part 2 (HOME per tenant).
3. Re-check live the defects fixed in code: tool lockfiles after `mise install`, Builder gets the whole task text, a return round starts from the open PR's branch tip and reads failing checks (the Mac thread).

## Not verified

- Two tenants at once and the tenants bind mount.
- One-click Slack app creation (S1–S5).
- The weekly audit is described in a PR, not run.
- Full CI logs for Builder need `actions: read` and a log host in the egress allowlist: the owner's call.

## Blockers

- Revoke the Slack config token that was shown in chat.
- `make verify` on arm64 Mac: the pinned `hadolint` 2.12.0 (x86_64) segfaults; `gitleaks dir` also scans the ignored `.env`.
- Flaky: `TestCheckoutOfAMirrorOwnedByAnotherUser` failed once with ETXTBSY; audit it.

## Questions for the owner

All are in `docs/decisions.md`.
