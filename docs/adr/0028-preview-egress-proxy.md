---
status: accepted
date: 2026-11-28
---

# Previews reached through a preview-only egress proxy

## Context and Problem Statement

A Verifier acceptance job opens the task's preview in the worker (design section 17). The deployer published the preview on the engine host's `127.0.0.1`, which a container cannot reach, and the worker's only way out, `egress`, denies private ranges; the live check of 2026-11-28 on the owner's Mac confirmed both. Smokescreen v0.1.0 reads its ACL once at start (SIGHUP stops it), so an allowlist of one exact host per preview would mean a restart of the proxy for each preview. The owner approved a `preview` network with exact preview hosts in egress on 2026-11-25 (decision 13). How does the worker reach a live preview, and nothing else, without that restart?

## Considered Options

- A second Smokescreen, `egress-preview`, on `jobs` and `preview` only, with the ACL `*.preview.internal` and `--allow-range` for the fixed `preview` subnet; the deployer attaches each preview's entry service to `preview` with the alias `<project>.preview.internal`.
- The same rules added to `egress`, which also reaches the internet.
- Exact hosts in the ACL, rewritten and the proxy restarted on every preview.
- The worker joins `preview` directly, without a proxy.

## Decision Outcome

Chosen option: "a second Smokescreen", because a name passes only when it ends in `.preview.internal` (a suffix reserved for private use, so no public name has it), resolves into the preview subnet, and exists only while the deployer keeps that preview on the network; the proxy has no internet network, and `egress` is unchanged.

### Consequences

- Good: the reachable hosts are exactly the live previews; a name that resolves elsewhere is refused (checked on a real engine: a `.preview.internal` alias on `jobs` was denied as a private range).
- Good: no proxy restart, so running jobs keep their connections.
- Bad: one more container, and a fixed subnet that must not collide with the host's networks (`NIGHTSHIFT_PREVIEW_SUBNET`).
- Bad: entry services of different previews share the network and can reach each other; a per-preview network is the next step if a tenant needs that isolation.

### Confirmation

T1 tests in `internal/adapters/deploy/composelocal`; the live check `docs/live-checks/V2-worker-reaches-preview.md`.

## Pros and Cons of the Options

### Rules added to `egress`

- Good: no new container.
- Bad: one proxy then holds both the internet allowlist and the private preview range.

### Exact hosts and a restart per preview

- Good: the ACL names each host.
- Bad: every preview cuts the model connections of running jobs.

### The worker on `preview` directly

- Good: simplest.
- Bad: the agent reaches every container on the network on every port, with no log of what it opened.
