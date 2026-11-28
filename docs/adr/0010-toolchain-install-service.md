---
status: accepted
date: 2026-11-25
---

# Install project toolchains in a separate service with its own way out

## Context and Problem Statement

The worker runs `mise install` for the toolchain a project pins (ADR 0009). The tools come from hosts such as `go.dev`, `nodejs.org`, `dl.google.com` and `github.com`, which the worker's egress allowlist does not open. Opening them for the worker would also open them for the agent. Access to `github.com` is a real leak path, because a push to a foreign repository works with a foreign token. How does the worker get its tools without widening what the agent can reach? The owner delegated the choice on 2026-11-25.

## Considered Options

- A `toolchain` service in compose: the only container that reaches the toolchain hosts; the runner asks it to run `mise install` in the job's workspace before the agent starts. The worker's egress does not change.
- Add the toolchain hosts (Go, Node, Google, GitHub) to the worker's allowlist.
- Add those hosts and also the Java and Rust hosts at once.
- Bake the tools into the worker image.

## Decision Outcome

Chosen option: "A `toolchain` service", because the agent keeps no network path to `github.com`, and the install step is the only code that needs the hosts. It is the same pattern as `modcache` (ADR 0008).

How it works:

- The service runs the worker image with the command `toolchain`. It shares the `work` and `agent-home` volumes with the worker and has its own egress proxy with a separate allowlist (`deploy/egress-install-acl.yaml`). The list holds the hosts of Go, Node, GitHub releases, sqlc, the npm registry and the Sigstore trust root that provenance checks need; Go tools built from source come from the module cache (`GOPROXY`, ADR 0008).
- The runner calls `POST /install` on the `jobs` network with the job id and the workspace path, then waits. The service runs `mise install` there as the agent user and returns the output tail.
- Each attempt of a job is accepted once, in its own checkout, and never while another attempt of the job installs; an attempt seen before is refused. The agent starts only after the call returns, so an agent that edits `mise.toml` and calls the service again is refused, while a job retried after a worker died installs in its fresh checkout.
- The runner then runs `project.yaml: setup` as before, with the worker's own egress and `GOPROXY`.
- Hosts for Java and Rust are added to the install allowlist per instance, when the first project needs them.

### Consequences

- Good: the worker's allowlist stays as it is; tools persist in the `agent-home` volume; the install step has one place for registry mirrors.
- Bad: one more service with a way out and a small HTTP API; the `agent-home` volume is shared by two containers.

## Pros and Cons of the Options

### Add the hosts to the worker's allowlist

- Good: no new service, about a day less work.
- Bad: the agent reaches `github.com` and can leak code through it.

### Add all hosts at once

- Good: simpler to configure.
- Bad: wider than the first option, with the same leak path.

### Bake the tools into the image

- Good: no runtime download.
- Bad: every project needs its own image build; contradicts "any language without a pack".
