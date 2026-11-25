# Runbook

Operational procedures for a Nightshift instance. Steps are filled in by the
phase that introduces each part; until then a section says which phase.

## Start and stop

1. Once: `sudo deploy/init-secrets.sh` (macOS with Docker Desktop: without `sudo`, here and in
   the `chown` steps, which you skip). It writes random Postgres passwords, the worker token (and its hash) and the restic
   password to `deploy/secrets/`, each file 0400 and owned by the reading container's uid.
   Save `restic_password` in the password manager. Every file, who creates it and who
   reads it: [`docs/secrets.md`](secrets.md).
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

## The conductor

`nightshift serve` runs the conductor (queued task to Builder to pull request to merge). Compose
passes `--repos /repos`; the conductor stays off until it has a GitHub credential.

Use a GitHub App: its pull requests are its own, so you can approve them.

`docs/setup.md` section 5 creates it with one command. By hand:

1. Register an app (GitHub > Settings > Developer settings > GitHub Apps): webhook off, only
   on your account; repository permissions Contents, Pull requests and Commit statuses read and
   write, Checks and Actions read. Never give it Workflows.
2. Install it on the tenant's repositories only.
3. Write the App ID to `deploy/secrets/github_app_id` and move the downloaded private key to
   `deploy/secrets/github_app_key`; keep both 0400 (Linux: owner 10001). If the empty files
   exist, `chmod 0600` them first.
4. `docker compose up -d --force-recreate nightshift`.

A fine-grained token in `deploy/secrets/github_token` (same permissions) also works, with two
limits: you cannot approve a pull request opened with your own token, and CI comes from Actions
runs and commit statuses, because such a token cannot read check runs.

The repository needs a CI check on pull requests: the conductor waits until one reports.

Tasks are picked up every 15 seconds. A task starts from `queued`; until Lead exists a person
puts it there with `nightshift task add <tenant> "<what to do>"` in the `nightshift` service.
Plan ahead with `--not-before 2026-11-26T22:00:00+01:00` (it does not start earlier) and
`--priority N` (-10 to 10; a higher one goes first): write the tasks by day, they run at night.

`repos` and `artifacts` volumes created by an earlier image keep their old owners. Fix them once:

```sh
docker compose run --rm --no-deps --user 0 --cap-add CHOWN --cap-add FOWNER --entrypoint sh nightshift \
  -c 'chown -R 10001 /repos'
docker compose run --rm --no-deps --user 0 --cap-add CHOWN --cap-add FOWNER --cap-add FSETID -v deploy_artifacts:/a \
  --entrypoint sh nightshift -c 'chown 0:10001 /a && chmod 2751 /a'
```

## Answer without Slack

A tenant with `chat: { adapter: inbox }` keeps Lead's cards in the book. Read and answer them with
`docker compose exec nightshift nightshift inbox`, `reply <task> "text"`, `approve <task>` and
`status`, or from your own Claude: `claude mcp add nightshift -- docker compose exec -T nightshift
nightshift mcp`. Every decision records its door (`cli`, `mcp`, `mcp-remote`) as the event's
actor. The remote door for a claude.ai project stays off until the tenant sets
`mcp: { remote: { listen: 127.0.0.1:8765 } }`; it checks the bearer token in `mcp_token` (rotate
it like any secret: overwrite the file, restart `nightshift`).

## Connect a project

`nightshift tenant init <name> --discover --repo-dir <checkout>` writes drafts into
`tenants/<name>/onboarding/`. After the `onboard-tenant` skill has filled them in, add
`--open-pr owner/repo` (reads the `github_token` secret from `--secrets`) to open a draft pull
request that adds `.nightshift/project.yaml`; the owner reviews and merges it. Then run
`nightshift tenant doctor <name> --repo-dir <checkout>`.

## Check the setup with doctor

Each container holds only its own secrets, so run the check inside the one you want to test:
`docker compose exec nightshift nightshift doctor` for the service and
`docker compose exec worker nightshift doctor --worker` for the worker.

- Service: `config`, `book`, `code host credential`, and for each tenant its builder provider,
  its chat and each of its repositories.
- Worker: `agent CLIs`, `model credentials` and `egress proxy`.

```text
ok    book: migrated, 0 task(s)
fail  example-solo chat: the owner's chat id or the channels are missing
      fix: docs/setup.md section 3: tokens, the owner's member id and channel ids
2 checks, 1 failed
```

Each line starts with `ok` or `fail`. A `fail` line is followed by a `fix:` line that names the
doc section to read. The last line is `N checks, M failed`. The exit code is non-zero when any
check fails. For a single project use `nightshift tenant doctor` (see Connect a project).

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

## Read a task's history

Every state change and decision is a row in `book.events`. The table is append-only, so the
history is complete. Columns (`internal/book/migrations/00001_book.sql`): `id` (global order),
`tenant`, `task_id` (references `book.tasks.id`; null for events without a task), `kind`
(for example `task.moved`), `actor` (`system`, a role, or a chat, GitHub or tracker id),
`payload` (jsonb), `trace_id` and `created_at`.

```sh
docker compose exec postgres psql -U postgres -d book -c \
  "SELECT id, created_at, kind, actor, payload FROM book.events
   WHERE task_id = 42 ORDER BY id"
```

Replace `42` with the task id (`SELECT id, title, state FROM book.tasks ORDER BY id DESC
LIMIT 10` lists recent tasks). Add `AND kind = 'task.moved'` to see only moves, or
`payload->>'key'` to pick one field out of a payload. For the stable instance use
`scripts/stable.sh compose exec postgres psql ...`. Do not try to edit or delete events: a
trigger rejects it.

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
`CODEX_VERSION=<version> docker compose build worker` (pin a version from `npm view @openai/codex version`;
for Claude set `CLAUDE_CODE_VERSION` from `npm view @anthropic-ai/claude-code version`; keep both in
`deploy/.env` so a later build does not drop a CLI) and add
the provider to the instance's `providers.yaml` and the role to the tenant's `providers`. Egress allows
`api.openai.com` and `api.deepseek.com` only; another endpoint needs its host in `deploy/egress-acl.yaml`.
Every worker holds every file in the directory, so keep only the keys you use there.

## The worker image (Debian, mise)

The worker is Debian slim with mise, git, make, build-essential, Node (for the agent CLIs) and no
language toolchain (ADR 0009). A project pins its toolchain in `mise.toml` or `.tool-versions`;
before each job the runner runs `mise install` (into the `agent-home` volume, so the second job
installs nothing) and then the project's `setup`. A first job on new versions downloads them through
the egress allowlist: if `mise install` fails with a network error, the host of that tool is not on
the allowlist (`docker compose logs worker`, the job's `job.error` event). `MISE_VERSION=<apt version>`
pins mise at build time; empty takes the newest. `/tmp` of the worker is executable, because Go, Node
and Gradle run what they build there.

## Go modules fail to download

The worker fetches Go modules from the `modcache` service (`GOPROXY=http://modcache:3000`), the only
container that reaches `proxy.golang.org` and `storage.googleapis.com` (ADR 0008). If a build says
it cannot download a module, check `docker compose logs modcache` and that the service is up; the cache
lives in the `modcache` volume and can be deleted safely, it refills on demand. Do not add
`storage.googleapis.com` to `deploy/egress-acl.yaml`.

## A project's tools fail to install

When a project pins its tools (`mise.toml`), the worker asks the `toolchain` service to run
`mise install` before the agent starts (ADR 0010). The service is the only container that reaches the
tool hosts, through its own proxy `egress-install` and the allowlist `deploy/egress-install-acl.yaml`.
If a job fails with "toolchain install failed", read the output in the job's error, then
`docker compose logs toolchain egress-install`. A denied host means the tool comes from a host that
is not in that allowlist (Java and Rust hosts are added here, per instance, with the first such
project). Add it to `egress-install-acl.yaml`, never to `egress-acl.yaml`, which belongs to the agent.
Installed tools live in the `agent-home` volume and survive restarts.

## Rotate secrets

Every 3 months and at once on suspicion. The general order: create the new secret,
update `deploy/secrets/`, restart the service that reads it, revoke the old one.
Keep each file 0400 and owned by the uid of its reader (see `deploy/init-secrets.sh`).

- **Remote MCP token.** Overwrite `mcp_token` with a new random value (`openssl rand -hex 32`),
  restart `nightshift`, and give the new token to the connector.
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

## Knowledge search

Full text search works out of the box. `postgres` runs the pgvector image, and a new data
directory gets the extension from `deploy/postgres/10-roles.sh`. To turn vector search on, apply
migrations (the embedding column appears when the extension exists) and set `search.embedder` in
`instance.yaml` to an embedding provider. Without it nothing is embedded and no extra traffic leaves.
A data directory made by the old alpine image cannot be reused (different base, uid and collations):
restore the book from a dump into a new volume. In a data directory that predates the extension run
`CREATE EXTENSION vector;` once as `postgres` in `book`.

## Stable instance

The factory that works on its own repository runs a tagged release of `main`, as a second
compose project (`nightshift-stable`) with its own volumes, images and config. Building or
breaking the development stack in the checkout does not touch it. Its tenant keeps
`review: { mode: all }`: nothing merges without the owner's Approve on GitHub.

```mermaid
flowchart LR
  M[main] --> L[live check on the dev stack] --> T[tag factory-stable-date] --> S[scripts/stable.sh up tag]
  S --> F[stable instance] --> P[pull requests to main]
```

State lives in `~/.nightshift/stable` (`NIGHTSHIFT_STABLE_HOME`):

| Path | What it is |
| --- | --- |
| `config/` | `instance.yaml`, `providers.yaml`, `tenants/<the factory's tenant>/` |
| `env` | `CLAUDE_CODE_VERSION`, `CODEX_VERSION`, `NIGHTSHIFT_OTLP`, `NIGHTSHIFT_TELEMETRY` |
| `releases/<tag>/` | the sources of each tag, written by the script |
| `current`, `previous` | the running tag and the one before it |
| `offsite/` | the off-site copy of the book's dumps |

The secrets are the files of `deploy/secrets/` in the checkout (`NIGHTSHIFT_STABLE_SECRETS`
names another directory); the release links to them, nothing is copied.

### First install

1. Set up and check one instance with [`setup.md`](setup.md), so the secrets, the GitHub App
   (installed on the factory's repository) and the chat app exist. It needs two tenants: one
   named `nightshift` for the factory's repository, and a sandbox one on the `inbox` adapter
   that stays on the development stack.
2. Stop the development stack: `(cd deploy && docker compose --profile obs stop)`.
3. Move the factory's tenant to the stable instance, so only one instance serves it:

   ```sh
   S=~/.nightshift/stable && mkdir -p $S/config/tenants
   cp deploy/config/instance.yaml deploy/config/providers.yaml $S/config/
   mv deploy/config/tenants/nightshift $S/config/tenants/
   cp deploy/.env $S/env
   ```

4. Tag a commit of `main` that passed the live check, and start it:

   ```sh
   git tag -a factory-stable-$(date +%F) <commit> -m "stable factory" && git push origin factory-stable-$(date +%F)
   scripts/stable.sh up factory-stable-$(date +%F)
   ```

5. On a new book, once: start task numbers away from the other instance, because a task's
   branch is `ns/<number>`.

   ```sh
   scripts/stable.sh compose exec -T postgres psql -U postgres -d book -c \
     "ALTER TABLE book.tasks ALTER COLUMN id RESTART WITH 1000"
   ```

### Day to day

`scripts/stable.sh compose <args>` is `docker compose` on the running release:

```sh
scripts/stable.sh current
scripts/stable.sh compose exec nightshift nightshift task add nightshift "docs: ..."
scripts/stable.sh compose logs --tail 100 nightshift worker
scripts/stable.sh compose stop        # start again with: scripts/stable.sh compose start
```

Never run plain `docker compose` inside a release directory: without the project name it
would replace the development stack.

### Both instances on one machine

- Only the stable config names the chat adapter and the factory's repository. The development
  config keeps sandbox tenants on the `inbox` adapter: two instances on one chat app would
  each receive part of the events.
- A subscription provider (`claude-sub`) allows one session at a time across both. Stop the
  stable worker (`scripts/stable.sh compose stop worker`) while the development stack runs a
  task, then start it again.
- Grafana of both uses port 3000: start the development stack without `--profile obs`.
- The secret files are shared, the books are not. After rotating a Postgres role password
  ([Rotate secrets](#rotate-secrets)), run the same `ALTER ROLE` with
  `scripts/stable.sh compose exec postgres ...`; after any rotation restart the stable
  services that read the secret.
- The default off-site copy is a local directory, one per instance. With a remote
  `restic_repository`, give the stable instance its own secrets directory and repository:
  in one repository the two books' snapshots prune each other.

### Upgrade

Move to a new tag only after a green live check of that commit.

1. Wait until the stable instance runs no task (`status` shows none in `building` or
   `checks`), then stop its worker: `scripts/stable.sh compose stop worker`.
2. Check the commit on the development stack: `docker compose up -d --build` in `deploy/` at
   that commit, both `doctor` commands, and one sandbox task that reaches `released`. Stop the
   stack afterwards (`docker compose stop`).
3. Tag the commit (a second tag on one day ends in `.2`), push the tag, run
   `scripts/stable.sh up <tag>`; it starts the stable worker again.
4. If the build fails, the running containers keep working: run `up <previous tag>` to restore
   the images. If doctor or the first task fails, go back the same way (the script prints the
   command). Book migrations only add, so an older release reads a newer book. If you stop
   before step 3, start the worker again: `scripts/stable.sh compose start worker`.

## Upgrade the factory

The factory's own instance is upgraded as in [Stable instance](#stable-instance). Phase 6 adds
`deploy/upgrade.sh <tag>`: stop the dispatcher, wait for jobs, switch
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

Grafana is on `http://127.0.0.1:3000` only. A task has one trace: the worker exports a span per
job, per agent turn and for the checks. Claude Code (2.1.332) sends its logs with the same trace
id, so Loki links to the task, but in `-p` mode its spans start their own traces. Codex sends no
telemetry yet.

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
