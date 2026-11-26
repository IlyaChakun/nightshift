# NS-A04 — Execution isolation and agent trust boundaries

**Priority:** P1; P0 if cross-job secret or hidden-scenario access is confirmed.  
**Status:** threat-model concern; no sandbox escape or data exposure was demonstrated.

## Observation

The earlier review of [deploy/docker-compose.yml](https://github.com/IlyaChakun/nightshift/blob/main/deploy/docker-compose.yml#L54-L89) noted read-only root filesystem, capability restrictions, network segmentation and egress proxy, but also shared volumes for work, artifacts, agent home and read-only repositories. Agents reportedly run under the same UID. Verify actual mount scopes, worker topology, file permissions, namespace isolation, cleanup and harness-specific behavior at current HEAD.

A worker-level container is not necessarily a job-level security boundary. A malicious dependency, compromised tool or erroneous agent might access another task's files, process state, credentials, hidden verification inputs or cached context. Shared HOME may also persist state between unrelated jobs. The threat model differs for a single trusted project versus arbitrary untrusted customer code.

## Invariants

- A Builder cannot read or modify hidden acceptance inputs, trusted verifier state or release credentials.
- Jobs cannot read or overwrite unrelated jobs' workspace, HOME, artifacts or secrets, except through explicitly authorized APIs.
- Cancellation, timeout and worker failure do not leave uncontrolled child processes or reusable credentials.
- Network and filesystem permissions reflect role, project and job; resource budgets are enforced outside the agent.

## Options and trade-offs

| Option | Benefits | Costs / limitations |
|---|---|---|
| Per-job directories, distinct HOME, scoped artifact access and cleanup | Low-cost improvement for trusted workloads | Directory separation alone is not a security boundary for arbitrary same-UID code |
| Ephemeral container per job with scoped mounts, identities, quotas and egress policy | Clearer job boundary; practical default for untrusted execution | Startup overhead, image/cache management, container runtime exposure must be addressed |
| Dedicated verifier runner and separate secret/evaluation store | Protects acceptance inputs and evidence authority | Additional orchestration and data transfer; needs careful result redaction |
| Hardened runtime or microVM | Stronger isolation for higher-risk multi-tenant workloads | More operational cost and latency; threat-model dependent |

## Suggested approach

Start by mapping assets, actors, permissions and cross-job data paths. Enforce the verifier/Builder boundary before expanding autonomous release. For untrusted code use a real isolation boundary, not merely separate directories. Use short-lived scoped credentials, deny-by-default egress with explicit allowances, bounded CPU/memory/disk/time, no privileged runtime socket in the sandbox, and cleanup/reaping controlled externally. Cache only explicitly approved non-sensitive data. Evaluate container-per-job before microVMs unless threat model requires stronger isolation.

## Acceptance criteria

In a disposable test environment, use two concurrent jobs with synthetic secrets and distinct workspaces; assert neither can access the other's protected data. Exercise Builder attempts to access hidden scenarios, verifier artifacts, production credentials and disallowed network destinations. Verify process-tree cleanup after cancellation, timeout and worker death; check disk/CPU/memory quota enforcement and credential expiry. Test stale files and agent home leakage across sequential jobs. Record actual platform isolation assumptions and limitations. Do not use real credentials or production evaluation content in these tests.

**Dependencies:** NS-A01 evidence authority, NS-A02 permission matrix, NS-A07 live verification.

## Follow-up source check and retention

This dated external review is a candidate finding record, not the living design or a verified defect register. Preserve the original findings and append a disposition or link to implementation evidence; do not delete this record merely because an issue is resolved. Superseding or relocating it requires an explicit owner decision.

**Checked:** pinned Nightshift base [9473fa4](https://github.com/IlyaChakun/nightshift/tree/9473fa4bc294825c110b6ec236da7110d0ff0e15), 2026-11-26. At the pinned base, `deploy/docker-compose.yml` line 81 still mounts shared work, artifacts and agent-home volumes. `docs/status.md` says two tenants concurrently and live worker-death recovery remain unverified. No escape was reproduced.

**Inferred:** proposed priorities, failure modes and remedies in this document; no tests or live runs performed. **Confidence:** medium for the risk assessment, not a confirmed incident.
