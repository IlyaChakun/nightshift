---
status: accepted
date: 2026-11-25
---

# Serve Go modules to the worker from a cache service

## Context and Problem Statement

Large Go modules redirect from `proxy.golang.org` to `storage.googleapis.com`. The worker's egress allowlist does not open that host, so a Builder task on a project with such modules cannot download them; on the first live run the module cache was filled by hand. How does the worker get Go modules without widening what an agent can reach?

## Considered Options

- A module cache service (`modcache`, Athens) in compose: the only container that reaches `proxy.golang.org` and `storage.googleapis.com`; the worker's `GOPROXY` points at it.
- Allow `storage.googleapis.com` in the worker's egress, limited to the Go proxy path.
- Fill the module cache while building the worker image.

## Decision Outcome

Chosen option: "A module cache service", because the worker's egress does not grow, the cache is shared by all tasks and survives restarts, and the same pattern covers npm, pip and Maven later. The owner chose it on 2026-11-25.

### Consequences

- Good: no new host in the worker's allowlist; faster repeated downloads; one place to apply a registry mirror for work projects.
- Bad: one more container with a way out; its disk volume must be backed up or rebuilt; a cache miss still needs the public proxy to be up.

## Pros and Cons of the Options

### Allow storage.googleapis.com for the worker

- Good: no new service.
- Bad: a wider allowlist (any bucket path, a possible leak channel) and tied to how Google hosts modules today.

### Fill the cache at image build

- Good: no runtime dependency, no new service.
- Bad: a new dependency of a project is not available until the next image build.
