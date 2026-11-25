---
status: accepted
date: 2026-11-25
---

# Deploy targets are adapters behind one small interface

## Context and Problem Statement

Phase 6 gives the factory preview environments, releases, smoke checks and rollbacks. Tenants deploy in very different ways: the owner's first project starts on `docker compose` on a laptop, a later one will run on a rented server or Kubernetes, and a team tenant has its own pipeline. The design so far knows two preview kinds (`script`, `external`) and three bare commands (`release`, `rollback`, `smoke`), and says that preview through `docker compose` comes later because the worker has no Docker socket. How does the core stay free of any platform while a tenant can start on a laptop and move to a cluster without changing the factory?

## Considered Options

- Keep only `script`: every tenant brings its own commands for every operation.
- A small `Deployer` interface in the core with built-in adapters (`compose-local`, `script`, `external`; `compose-ssh` and `kubernetes` later), chosen per capability in `project.yaml`.
- Hand everything to GitOps (Argo CD or similar): release is a merge and a tag, preview is a pull request generator.
- An external plugin protocol: any executable that speaks a JSON protocol is an adapter.

## Decision Outcome

Chosen option: "A small `Deployer` interface with built-in adapters", because the factory has to reason about environments (clean them up on cancel, enforce the preview limit, record the previous revision for rollback, resume after a restart), and bare scripts give it no way to do that. `script` stays as the escape hatch, so no tenant is blocked by a missing adapter.

- The interface has six operations: `up`, `down`, `release`, `smoke`, `rollback`, `status`. Each is idempotent and keyed by tenant, task and step, so the step journal that the merge train already uses can replay it after a restart.
- `project.yaml: capabilities.preview|release|rollback|smoke` each name an adapter by `kind`. A bare string means `kind: script`. One tenant can mix adapters, for example preview on `compose-local` and release on `kubernetes`.
- Phase 6 builds `compose-local`, `script`, `external` and a twin. `compose-ssh` and `kubernetes` join with the first tenant that needs them. Creating servers or clusters is not an adapter: it is code in the project (for example Terraform), changed through a task and a human button.
- Deploy operations never run in the worker or in the core. A separate `deployer` job runs them with the credentials of one tenant, as `toolchain` does for installs (ADR 0010). The worker still has no Docker socket, kubeconfig or SSH key.
- `compose-local` needs the host's Docker engine. That access is root-equivalent on the engine's host, so the deployer renders the recipe from `main`, rejects privileged mode, host networking, host mounts and the Docker socket, and limits CPU and memory. Giving the deployer the engine, and the preview network that the egress proxy allows host by host, are owner decisions (`docs/decisions.md`, 13) and are not built before the answer.

### Consequences

- Good: a tenant moves from a laptop to a cluster by changing `kind`, not by changing the factory; the core and the state machine see only the six operations.
- Good: previews have one lifecycle for every platform: limit, time to live, teardown on cancel.
- Bad: one more service with access to a deploy target, and one more interface to keep stable.
- Bad: three adapters to maintain in phase 6 instead of a script convention. Mitigation: `script` is the only adapter every tenant needs; the others are small and tested on the twin.

## Pros and Cons of the Options

### Keep only `script`

- Good: nothing new to build; the schema already has it.
- Bad: every tenant rewrites preview naming, teardown, limits and rollback bookkeeping; the factory cannot tell a failed preview from a slow one.

### GitOps for everything

- Good: no cluster rights for the factory; a merge is the release.
- Bad: assumes a cluster; nothing for a first project on a laptop; preview per branch needs the GitOps stack first.

### External plugin protocol

- Good: any platform without a core change.
- Bad: a second contract to version and secure, for a gap that `script` already covers.
