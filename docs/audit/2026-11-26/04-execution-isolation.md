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

## Decision brief — execution boundary by threat model, 2026-11-26

**Recommendation:** complete M3 and explicitly preserve ADR 0015's instance-level trust boundary. Use separate instances for unrelated trust domains now. For independent hidden acceptance and concurrent jobs, establish actual per-job/attempt isolation before increasing concurrency. Defer gVisor or microVM adoption until a representative security and compatibility experiment warrants it.

### Verified static evidence

| Evidence at pinned base | Disposition |
| --- | --- |
| [Compose worker](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/deploy/docker-compose.yml#L60-L96) uses read-only rootfs, drops capabilities except UID/GID switching, restricts networks, caps CPU/memory/PIDs and mounts work, artifacts, agent-home and repositories. [Dockerfile](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/Dockerfile#L75-L81) creates root-only worker secrets and separate artifact permissions. | Real worker/core boundary; shared mounts are not independent per-job boundaries. Do not claim all artifacts are agent-readable: permissions differ. |
| [HomeFor/prepareHome](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/runner/toolchain.go#L209-L227) separates HOME by tenant name and creates it under the common agent identity. [prepare](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/runner/runner.go#L279-L305) keys checkout by tenant/task, not job attempt. [cleanup](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/runner/runner.go#L308-L348) removes old checkouts and rejects non-directory tenant roots. | Valuable hygiene; same UID and common mounts do not prove isolation against arbitrary code. Concurrent attempts of a reclaimed task need distinct paths and ownership. |
| [writeArtifact](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/runner/runner.go#L366-L379) uses job-ID directories and restricted modes. [heartbeat](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/runner/runner.go#L388-L413) logs failures without immediately stopping execution. | Artifact identity is not attempt-specific in the inspected path. A lost lease may leave old work executing while a new attempt starts; reproduce overlapping attempts and check that stale work cannot mutate accepted artifacts. |
| [VisibleInputs](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/jobsapi/api.go#L146-L171) filters hidden scenario inputs by role; [hidden storage](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/dispatch/hidden.go#L16-L45) keeps scenarios out of the spec card and repository. | Existing logical access controls. They do not prove filesystem/session separation after a verifier receives scenarios. |
| [complete](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/jobsapi/server.go#L340-L421) checks lease validity and persists result metadata and artifacts transactionally. [events](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/internal/jobsapi/server.go#L297-L338) checks the lease before separate event writes. | Completion already has strong DB fencing; stale telemetry race and filesystem writes remain distinct questions. Coordinate with A03. |
| [ADR 0015](https://github.com/IlyaChakun/nightshift/blob/9473fa4bc294825c110b6ec236da7110d0ff0e15/docs/adr/0015-several-tenants-on-one-instance.md#L18-L32) explicitly states that the instance is the trust domain and hostile tenants are not contained. | Correct the earlier assumption: tenant-level hostile isolation is not an existing architecture promise. Treat stronger separation as an extension, not a current defect by itself. |

### Primary research and trade-offs

- [Docker Engine security](https://docs.docker.com/engine/security/), read 2026-11-26: namespaces, cgroups and restricted daemon access underpin container isolation. Scope mounts, identities and network policy; do not give a job the runtime socket.
- [gVisor security architecture](https://gvisor.dev/docs/architecture_guide/intro/), read 2026-11-26: an application kernel reduces direct exposure to the host kernel; this is a stronger runtime option, not a substitute for tenant-scoped filesystem and credential policy.
- [Firecracker design](https://github.com/firecracker-microvm/firecracker/blob/main/docs/design.md), read 2026-11-26: a lightweight VM boundary is another option for untrusted workloads. Linux host/KVM prerequisites and compatibility with the actual harnesses on the owner's Mac are not verified.

| Option | Fit |
| --- | --- |
| Harden current reusable worker | Best initial step for same-trust-domain workloads; cheap and preserves warm harness state. Cannot promise hostile isolation with shared UID and mounts. |
| Separate Compose instances | Existing architecture supports separate trust domains; operational duplication, but no new core service. Recommended for confidential or unrelated owners. |
| Disposable container per job with scoped mounts and external launcher | Stronger job and role boundary; startup, cache and launcher complexity. Evaluate for hidden acceptance and reclaimed jobs first. |
| gVisor or microVM execution | Additional kernel boundary for higher-risk workloads; benchmark browser, toolchain, resource and host compatibility. Not needed by default for the personal pilot. |

### Incremental implementation

1. Define the workload classes and their promised isolation: trusted solo, same-domain projects, independent verifier, unrelated untrusted tenant. State explicitly which combinations are forbidden until a stronger boundary exists.
2. Finish M3: inventory accessible mounts, HOME, tool caches, mirrors, temporary data and process lifetimes. Use attempt-specific working paths and immutable result identities, with cleanup scoped to the owning attempt. Keep cross-attempt writes rejected even after cancellation or lease expiry.
3. Isolate verifier state and hidden inputs from Builder; use fresh role-scoped context and avoid putting hidden material in shared session state, search output, logs or exported artifacts. Retain only safe evidence identifiers outside the verifier boundary.
4. If filesystem and process controls are insufficient for the promised isolation, evaluate a disposable job container launched by a trusted component outside the workload. Mount only the authorized repository and scratch/artifact scope. Keep any runtime-control credentials outside the sandbox. Shared caches should be read-only or explicitly classified as safe.
5. Measure cold/warm start, per-job disk and memory, cleanup success, browser/tool compatibility and throughput on the actual deployment platform before changing the runtime.

### Acceptance and decision gate

Use synthetic canaries and disposable test infrastructure, never production credentials. T6 should verify denied cross-job and cross-role access, stale-attempt write rejection, process cleanup, denied access to control-plane credentials and unauthorized destinations, and correct resource limits. Run sequential and concurrent jobs, cancel and reclaim an expired lease, and verify independently that the newer attempt's artifact remains intact. Existing T3 lease tests and T6 compose checks are useful but not proof of live isolation.

Do not permit mutually untrusted tenants inside the current instance. Do not expand autonomous production release until hidden-scenario confidentiality and immutable attempt evidence are demonstrated. Retain the current runtime if scoped containers and existing protections satisfy the agreed threat model at acceptable cost.

**Checked:** pinned runner, jobs API, compose, Dockerfile, ADRs 0004/0015, design and status; Docker, gVisor and Firecracker documentation. **Inferred:** concurrent stale-attempt risk, recommended sequencing and runtime trade-offs. **Confidence:** high for inspected structure, medium for actual isolation on the host. **Not verified:** live file permissions, sandbox escapes, network policy behavior, process cleanup, performance or alternative runtime compatibility.
