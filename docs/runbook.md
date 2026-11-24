# Runbook

Operational procedures for a Nightshift instance. Steps are filled in by the
phase that introduces each part; until then a section says which phase.

## Start and stop

1. Once: `sudo deploy/init-secrets.sh`. It writes random Postgres passwords, the worker token (and its hash) and the restic
   password to `deploy/secrets/`, each file 0400 and owned by the reading container's uid.
   Save `restic_password` in the password manager.
   A data directory created before `book_backup` existed needs the role once, by hand:
   `CREATE ROLE book_backup LOGIN CREATEDB PASSWORD '<pg_backup_password>' IN ROLE pg_read_all_data;`
   and `GRANT CONNECT ON DATABASE book TO book_backup;` as `postgres`.
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

3. Restore as the superuser inside `postgres`; the `backup` container only holds the
   read-only `book_backup` role. With the chosen file:

   ```sh
   docker compose cp backup:/dumps/<file> /tmp/book.dump
   docker compose cp /tmp/book.dump postgres:/tmp/book.dump
   docker compose exec postgres sh -c 'dropdb -U postgres --force book &&
     pg_restore -U postgres --create --exit-on-error -d postgres /tmp/book.dump && rm /tmp/book.dump'
   rm /tmp/book.dump
   ```

4. `docker compose start nightshift` and check that it is healthy and old threads still
   get "Noted on task" replies.

## Harness credentials

One file per credential in `deploy/secrets/harness/`; the file name is the variable the harness reads
and the provider's `key_env` names it.

- OpenAI (`codex-api`): `printf '%s' "$KEY" > deploy/secrets/harness/OPENAI_API_KEY`
- DeepSeek (`deepseek`): `printf '%s' "$KEY" > deploy/secrets/harness/DEEPSEEK_API_KEY`
- Claude on an API key (`claude-api`): `ANTHROPIC_API_KEY`.
- Claude on a subscription (`claude-sub`, personal instance only): run `claude setup-token` on a machine
  where you are signed in, then `printf '%s' "$TOKEN" > deploy/secrets/harness/CLAUDE_CODE_OAUTH_TOKEN`.
  The adapter drops every variable that would override it; one session at a time (design section 12).
  Never use it for company code or for other people.

Then `sudo chown 10001 deploy/secrets/harness/* && sudo chmod 0400 deploy/secrets/harness/*`, build with
`CODEX_VERSION=<version> docker compose build worker` (pin a version from `npm view @openai/codex version`) and add
the provider to the instance's `providers.yaml` and the role to the tenant's `providers`. Egress allows
`api.openai.com` and `api.deepseek.com` only; another endpoint needs its host in `deploy/egress-acl.yaml`.
Every worker holds every file in the directory, so keep only the keys you use there.

## Rotate secrets

Every 3 months and at once on suspicion. The general order: create the new secret,
update `deploy/secrets/`, restart the service that reads it, revoke the old one.
Keep each file 0400 and owned by the uid of its reader (see `deploy/init-secrets.sh`).

- **Chat tokens.** Regenerate them in the chat app settings, overwrite
  `chat_bot_token` and `chat_app_token`, then `docker compose restart nightshift`.
- **Postgres role passwords.** `deploy/postgres/10-roles.sh` runs only on an empty
  data directory, so a new password must be set in the database too. Each password is
  stored twice: in `pg_<role>_password` (read by `postgres`) and inside the URL in
  `pg_<role>` (read by `migrate` or `nightshift`). For the `nightshift` role:

  ```sh
  pw=$(od -An -N24 -tx1 /dev/urandom | tr -d ' \n')
  docker compose exec postgres psql -U postgres -d book \
    -c "ALTER ROLE nightshift PASSWORD '$pw'"
  sudo sh -c "printf '%s' '$pw' > deploy/secrets/pg_nightshift_password"
  sudo sh -c "printf '%s' 'postgres://nightshift:$pw@postgres:5432/book?sslmode=disable' \
    > deploy/secrets/pg_nightshift"
  docker compose restart nightshift
  ```

  For `book_owner` use `pg_owner_password` and `pg_owner`; nothing needs a restart,
  because only the next `migrate` run reads it.
- **Backup role.** `ALTER ROLE book_backup PASSWORD ...`, overwrite
  `pg_backup_password`, then `docker compose restart backup`.
- **Superuser password (`pg_admin`).** `ALTER ROLE postgres PASSWORD ...` and overwrite
  `pg_admin`; only `postgres` reads it.
- **Restic password.** Add a key, switch the file, then remove the old key; the
  repository is not re-encrypted. `restic key add` asks for the new password:

  ```sh
  docker compose run --rm -it --entrypoint sh offsite -c \
    '. /etc/backup/env.sh && restic key add && restic key list'
  ```

  Put the new password into `restic_password` and the password manager, then run
  `restic key remove <old id>` the same way.
- **Phase 2 and later:** harness credentials, worker token, tracker and GitHub App
  tokens follow the general order.

## Upgrade the factory

Phase 6. `deploy/upgrade.sh <tag>`: stop the dispatcher, wait for jobs, switch
the image tag, run the health check and the fast polygon; on failure it returns
to the previous tag.

## Emergency path

If a broken version cannot fix itself, use an interactive Claude Code session
outside the factory on a fresh branch and a normal PR.

## Observability (obs profile)

Start Grafana, Loki, Tempo and Prometheus next to the factory:

```sh
NIGHTSHIFT_OTLP=http://lgtm:4318 NIGHTSHIFT_TELEMETRY=1 docker compose --profile obs up -d
```

Grafana is on `http://127.0.0.1:3000` only. `nightshift` exports a trace per task and a span
per job. Each job carries the task's `traceparent`; the runner passes it to the agent as
`TRACEPARENT`, so Claude Code telemetry from the worker lands next to the task's spans.

## Repository settings

Only `main` is long-lived. Settings are applied by the owner in GitHub (Settings > General and
Settings > Code security); agents cannot change them.

- Merge: squash only; "Automatically delete head branches" on; auto-merge on.
- Branches: delete a branch when its PR merges. Sweep leftovers (branches with no commits ahead
  of `main` and no open PR) with:
  `gh api repos/OWNER/REPO/branches --paginate --jq '.[].name'`, compare each with `main`, delete
  the ones that are fully merged. Keep `archive/*`.
- Security: secret scanning and push protection on, Dependabot alerts on.
- Protect `main` (needs GitHub Pro, Team or a public repository; the free private plan has no
  branch protection): require a pull request with 0 approvals (agents self-merge, see
  AGENTS.md), require the `lint`, `test` and `checks` jobs of `verify` and an up-to-date
  branch, require linear history, block force pushes and deletion, and do not allow bypass.
