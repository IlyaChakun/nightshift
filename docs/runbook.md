# Runbook

Operational procedures for a Nightshift instance. Steps are filled in by the
phase that introduces each part; until then a section says which phase.

## Start and stop

1. Once: `sudo deploy/init-secrets.sh`. It writes random Postgres passwords and the restic
   password to `deploy/secrets/`, each file 0400 and owned by the reading container's uid.
   Save `restic_password` in the password manager.
2. Paste the chat tokens into `deploy/secrets/chat_bot_token` and `chat_app_token`, then
   `sudo chown 10001 deploy/secrets/chat_* && sudo chmod 0400 deploy/secrets/chat_*`.
3. Put `instance.yaml`, `providers.yaml` and `tenants/` into `deploy/config/` (or set
   `NIGHTSHIFT_CONFIG`).
4. `cd deploy && docker compose up -d --build`. The one-shot `migrate` runs first;
   `docker compose ps` shows `nightshift` as healthy.

Stop with `docker compose stop` (state stays in the `pgdata` volume) or `down`. Never
`down -v`: it deletes the book and the dumps.

## Back up and restore the book

- `backup` dumps the book every hour to the `dumps` volume and keeps 48 dumps.
- `offsite` copies the dumps every night with restic to `restic_repository`
  (default `deploy/offsite/`, mount a second disk there or point it at cloud storage).
- Every Sunday `backup` restores the newest dump into a scratch database and writes the
  time to `/dumps/restore-test.ok`. Check it: `docker compose logs backup | grep restore`.

Restore:

1. `docker compose stop nightshift`.
2. Pick a dump: `docker compose exec backup ls -t /dumps`. If the machine was lost, run
   `sudo deploy/init-secrets.sh`, put the saved password into `restic_password`, start
   `postgres` and `backup`, then fetch the dumps from the off-site copy:

   ```sh
   docker compose run --rm --entrypoint sh offsite -c \
     '. /etc/backup/env.sh && restic restore latest --target /offsite/restore'
   docker compose cp offsite/restore/dumps/<file> backup:/dumps/
   ```
3. Run, with the chosen file:

   ```sh
   docker compose exec backup sh -c '. /etc/backup/env.sh &&
     dropdb --force book && pg_restore --create --exit-on-error -d postgres /dumps/<file>'
   ```

4. `docker compose start nightshift` and check that it is healthy and old threads still
   get "Noted on task" replies.

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
