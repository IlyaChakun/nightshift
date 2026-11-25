# Images are built locally with `docker compose build` (design section 3).
FROM golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY schemas ./schemas
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/nightshift ./cmd/nightshift

# nightshift keeps the repository mirrors and applies patches with git, so it needs a base with
# git. It owns /repos; /artifacts belongs to the worker's runner (root), and its group lets
# nightshift read what is written there. A new volume takes this ownership from the image.
FROM alpine:3.23@sha256:469c9ea410b9215eaee7976ba6c0639039b444be8e5d638f7abbada189971eb2 AS nightshift
RUN apk add --no-cache git ca-certificates \
    && adduser -D -H -u 10001 nightshift \
    && mkdir -p /repos /artifacts \
    && chown 10001 /repos \
    && chown 0:10001 /artifacts \
    && chmod 2751 /artifacts
COPY --from=build /out/nightshift /usr/local/bin/nightshift
# Read-only content of the conductor: role definitions and the standards for checklists.
COPY roles /roles
COPY standards /standards
USER 10001
ENTRYPOINT ["/usr/local/bin/nightshift"]
CMD ["serve"]

# The egress proxy is built from a pinned module version, so no registry digest is needed.
FROM golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS smokescreen-build
ARG SMOKESCREEN_VERSION=v0.1.0
RUN CGO_ENABLED=0 GOBIN=/out go install -trimpath github.com/stripe/smokescreen@${SMOKESCREEN_VERSION}

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab AS egress
COPY --from=smokescreen-build /out/smokescreen /usr/local/bin/smokescreen
USER 10002
ENTRYPOINT ["/usr/local/bin/smokescreen"]

# The worker: the runner runs as root without capabilities except SETUID and SETGID (compose)
# and starts the agent and every project command as uid 20001, which has no access to
# /run/secrets. The Go toolchain is here because projects run their tests in the worktree.
# bash is the shell of the agent CLIs' command tool. CLAUDE_CODE_VERSION and CODEX_VERSION pin the CLIs; leave one empty to build without it, then
# `nightshift doctor` reports the harness as missing.
FROM golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS worker
ARG CLAUDE_CODE_VERSION=
ARG CODEX_VERSION=
RUN apk add --no-cache bash git ca-certificates nodejs npm \
    && if [ -n "$CLAUDE_CODE_VERSION" ]; then npm install -g "@anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}"; fi \
    && if [ -n "$CODEX_VERSION" ]; then npm install -g "@openai/codex@${CODEX_VERSION}"; fi \
    && adduser -D -u 20001 -h /home/agent agent \
    && mkdir -p /work /artifacts /repos \
    && chown agent /work \
    && chown 10001 /repos \
    && chown 0:10001 /artifacts \
    && chmod 2751 /artifacts \
    && mkdir -m 0700 /run/worker-secrets
COPY --from=build /out/nightshift /usr/local/bin/nightshift
# Role instructions and skills are read-only content that the runner gives to the agent.
COPY roles /roles
COPY skills /skills
# Stack packs: the static checks that run on a patch after the project's own verify command.
COPY stacks /stacks
ENTRYPOINT ["/usr/local/bin/nightshift"]
CMD ["work"]
