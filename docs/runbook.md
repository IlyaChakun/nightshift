# Runbook

Operational procedures for a Nightshift instance. Steps are filled in by the
phase that introduces each part; until then a section says which phase.

## Start and stop

Phase 1. `docker compose -f deploy/docker-compose.yml up -d`; stop with `down`.

## Back up and restore the book

Phase 1. Hourly `pg_dump`, daily restic off the machine, weekly restore test.

## Rotate secrets

Phase 2. Every 3 months and at once on suspicion: harness credentials, worker
token, chat, tracker and GitHub App tokens. Order: create the new secret, update
`deploy/secrets/`, restart the service that reads it, revoke the old one.

## Upgrade the factory

Phase 6. `deploy/upgrade.sh <tag>`: stop the dispatcher, wait for jobs, switch
the image tag, run the health check and the fast polygon; on failure it returns
to the previous tag.

## Emergency path

If a broken version cannot fix itself, use an interactive Claude Code session
outside the factory on a fresh branch and a normal PR.
