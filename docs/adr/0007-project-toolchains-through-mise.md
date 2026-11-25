---
status: accepted
date: 2026-11-25
---

# Run any project language through its own toolchain file in one worker image

## Context and Problem Statement

The factory must work on projects in any language: Go, Python, TypeScript and JavaScript today, Java, Rust, C++ and others later. The worker image carries only Go and Node, so a Java or Rust project cannot even build, and a stack pack per language would never cover every stack a tenant brings. How does a project get its toolchain and checks without a change to the core for each language?

## Considered Options

- One Debian slim worker image with mise; the project's `mise.toml` or `.tool-versions` pins its toolchain, installed into a tenant cache volume; `project.yaml: setup` installs dependencies; `verify` is the gate; stack packs only add structured checks
- A worker image per tenant, built from the stack packs it uses
- A devcontainer per project (`devcontainer.json`)
- More toolchains baked into the current Alpine image, one stack pack per language

## Decision Outcome

Chosen option: "One Debian slim worker image with mise", because every language mise supports runs without a pack or an image build, the factory already pins its own tools with mise, and the same pattern is what Devin (blueprint), OpenHands (`.openhands/setup.sh`) and the Copilot coding agent (setup steps) use: the project describes its environment, the agent platform runs it.

### Consequences

- Good: a new language needs no change to the core; a pack is added only when a tenant needs structured findings.
- Good: one image to build, scan and pin.
- Bad: a first job on new toolchain versions downloads them; the cache volume amortizes it.
- Bad: stacks that need system packages and root (C++ system libraries, an Android SDK) are not covered; they wait for a tenant worker image ("Later").
- Bad: package registries of new languages are new egress entries, approved by the owner.

## Pros and Cons of the Options

### A worker image per tenant

- Good: system packages and exact toolchains are possible.
- Bad: an image build per tenant and per pack change; the factory has to build images, which compose without a Docker socket in the worker does not do today.

### A devcontainer per project

- Good: an existing standard that many repositories already have.
- Bad: it needs Docker to build and run, and the worker has no Docker socket (ADR 0004). Discovery may still read `devcontainer.json` as a hint for the toolchain.

### More toolchains in the Alpine image, one pack per language

- Good: no new runtime step.
- Bad: the image grows with every language, versions are the image's instead of the project's, musl breaks some prebuilt binaries, and the core changes for each new stack.
