# A04 review — worker isolation by trust domain

Keep one Compose instance per trust domain. The audit correctly distinguishes controls on a worker from guarantees for every job inside it. Its proposed runtime change needs a declared workload and evidence from the target host.

Checked revision: `8171d5ea85723b73185fd65acb27d4ded7134b82`.

| Claim | Disposition | Evidence |
|---|---|---|
| One instance per trust domain | Confirmed | ADR 0015 explicitly makes the instance the trust boundary. See `docs/adr/0015-several-tenants-on-one-instance.md:20`. |
| Compose restricts the worker | Confirmed in configuration | `deploy/docker-compose.yml:94` sets CPU, memory and PID limits on the worker. A configured limit does not measure per-job consumption. |
| Jobs API and output handling provide some independent controls | Confirmed | `internal/jobsapi/api.go:153` filters role inputs. `internal/jobsapi/server.go:381` completes the lease and records result metadata in one transaction. |
| M3 work and live checks are still pending | Partly | The plan records partial delivery. The live ledger records a tenant HOME check and separately queues concurrent tenants and worker-death recovery. |
| Unresolved protection finding | Confirmed, details kept outside the repository | |
| Production exposure or runtime escape | Unverified | No observed incident or live result in the evidence examined. |

**Take — tenants stream M3 and phase 6 P5.** Classify workloads by owner and role. For each supported class, document the permitted inputs, outputs, credential lifetime, cache retention and cleanup owner. Complete the existing checks for independent verification, cancellation, reclaim, resource bounds and concurrency on the supported deployment. Keep separate instances for unrelated owners. Record host, workload and result for each promised boundary.

**Later — tenants stream.** If a required boundary cannot be demonstrated, evaluate disposable job containers with scoped mounts and a trusted external launcher. Include a dedicated verifier only if P5 needs one. Benchmark startup, memory, disk, cleanup, browser compatibility and throughput before selecting a runtime.

**Reject — current pilot.** Making gVisor or Firecracker the default adds another runtime before M3 establishes the requirement. Docker describes “namespaces and control groups for the container.” gVisor describes a userspace application kernel and Firecracker documents a Linux KVM boundary. Those descriptions establish the mechanisms, while compatibility with this deployment remains unmeasured. Read 2026-11-26. [Docker](https://docs.docker.com/engine/security/) · [gVisor](https://gvisor.dev/docs/architecture_guide/intro/) · [Firecracker](https://github.com/firecracker-microvm/firecracker/blob/main/docs/design.md)

**Checked** — local `TestEachTenantHasItsOwnHome` passed. Read ADRs 0004/0015, Compose, runner, jobs API, M3 and the live-check ledger. **Inferred** — runtime selection should follow workload measurements. **Unverified** — target-host behavior, cleanup after worker death, effective quotas, credential expiry and alternative runtime performance. **Confidence** — high for the trust-domain decision, medium for deployment behavior.
