# Secrets of an instance

Every secret is one file in `deploy/secrets/` on the machine that runs the factory. The
directory is not in git. Docker Compose mounts each file only into the containers that
list it, at `/run/secrets/<name>` (in the worker at `/run/worker-secrets/<name>`, a root-only
directory that the agent user cannot open); Go code reads it with `config.Secrets` and never from
environment variables. Agents get none of these files. Design: sections 3 and 16.

## First start

1. `sudo deploy/init-secrets.sh` creates every generated secret below with mode 0400 and
   the right owner. It keeps files that already exist, so it is safe to run again.
   On macOS with Docker Desktop run it without `sudo`, and skip every `sudo` and `chown`
   below: files stay yours and each container still reads the ones it lists.
2. Save `restic_password` in your password manager.
3. Create the Slack app and its two tokens ([below](#slack-app-and-tokens)), then paste them
   and fix their owner and mode:

   ```sh
   sudo sh -c 'printf %s "<xoxb-…>" > deploy/secrets/chat_bot_token'
   sudo sh -c 'printf %s "<xapp-…>" > deploy/secrets/chat_app_token'
   sudo chown 10001 deploy/secrets/chat_* && sudo chmod 0400 deploy/secrets/chat_*
   ```

4. Check: `sudo ls -ln deploy/secrets/` lists the files of the table, each `-r--------`.

Then continue with "Start and stop" in [`docs/runbook.md`](runbook.md).

## The files

On Linux the uid is the owner of the file: 999 is `postgres` (the pgvector image), 10001 is `nightshift`, 0 is root.

| File | What it unlocks | Who creates it | Read by | uid |
| --- | --- | --- | --- | --- |
| `pg_admin` | Postgres superuser | `init-secrets.sh` | postgres | 999 |
| `pg_owner_password` | Schema owner `book_owner`, for migrations | `init-secrets.sh` | postgres | 999 |
| `pg_nightshift_password` | Service role `nightshift` | `init-secrets.sh` | postgres | 999 |
| `pg_backup_password` | Read-only role `book_backup` | `init-secrets.sh` | postgres, backup | 999 |
| `pg_owner` | URL with the owner's password | `init-secrets.sh` | migrate | 10001 |
| `pg_nightshift` | URL with the service password | `init-secrets.sh` | nightshift | 10001 |
| `pg_grafana_password` | Dashboard role `grafana_ro` (reads only the `book.dash_*` views); the obs profile's one-shot `grafana-login` sets it as the role's login | `init-secrets.sh` | grafana-login, lgtm | 10001 |
| `chat_bot_token` | Chat bot token (`xoxb-`): read and write channels | You, from the chat app settings ([`deploy/slack-manifest.yaml`](../deploy/slack-manifest.yaml)) | nightshift | 10001 |
| `chat_app_token` | Chat app-level token (`xapp-`, `connections:write`): Socket Mode events | You, from the chat app settings | nightshift | 10001 |
| `worker_token` | Jobs API token of the worker | `init-secrets.sh` | worker | 0 |
| `worker_token_sha256` | Hash that `nightshift` checks the worker token against | `init-secrets.sh` | nightshift | 10001 |
| `github_token` | Token for GitHub (fine-grained, one repository set: contents and pull requests read and write, commit statuses write, checks and actions read). Empty keeps the conductor off | `init-secrets.sh` makes it empty; you paste the token ([runbook](runbook.md#the-conductor)) | nightshift | 10001 |
| `github_app_id` | Id of the factory's GitHub App (not secret by itself) | `init-secrets.sh` makes it empty; you write the number ([runbook](runbook.md#the-conductor)) | nightshift | 10001 |
| `github_app_key` | Private key of the App: it mints tokens for the repositories the App is installed on. Preferred over `github_token` | `init-secrets.sh` makes it empty; you put the downloaded `.pem` there | nightshift | 10001 |
| `mcp_token` | Bearer token of the remote MCP door; read only when a tenant sets `mcp.remote.listen` (off by default, loopback only). Anyone with it can add tasks, answer Lead and approve a spec, never merge, roll back or change settings | `init-secrets.sh` | nightshift | 10001 |
| `deployer_token` | Bearer token of the deploy service: `nightshift` sends it with every call; the worker never holds it | `init-secrets.sh` | nightshift, deployer | 10001 |
| `harness` | Directory with one file per model credential, named by the variable the harness reads (for example `CLAUDE_CODE_OAUTH_TOKEN`, a subscription token: personal instances only; a team or work instance uses `ANTHROPIC_API_KEY`) | You (the directory is made by `init-secrets.sh`, mode 0700) | worker, under `/run/worker-secrets`, which the agent user cannot open | 0 |
| `restic_password` | Encryption key of the off-site backup | `init-secrets.sh`; a copy in your password manager | offsite | 0 |
| `restic_repository` | Where the off-site copy lives (`/offsite/nightshift`) | `init-secrets.sh` | offsite | 0 |

`tenants/<name>/github_token` (optional) is a tenant's own GitHub token, for when its repositories
belong to another account than the instance's App or token. The tenant's repositories use it; every
other repository keeps the instance's credential. `init-secrets.sh` makes `tenants/` (mode 0700, owner
10001). Add a token for the tenant `example-team` like this (on macOS leave out `sudo` and `chown`):

```sh
sudo mkdir -p deploy/secrets/tenants/example-team
sudo sh -c 'umask 077; cat > deploy/secrets/tenants/example-team/github_token'   # paste, then Ctrl-D
sudo chown -R 10001 deploy/secrets/tenants/example-team
sudo chmod 0700 deploy/secrets/tenants/example-team
sudo chmod 0400 deploy/secrets/tenants/example-team/github_token
docker compose -f deploy/docker-compose.yml restart nightshift
```

A tenant whose file cannot be read (wrong owner or mode), or that has no token while the instance has
no App or token either, is left out of the conductor, its morning digest included; the other tenants keep running and the service
logs why. `nightshift doctor` names the tenants that have a token and fails a "code host credential"
check for each one left out. Later phases add the tracker tokens there too.

## Slack app and tokens

Three tokens appear in the setup. Only the last two stay in the running system.

| Token | Prefix | Why | Lifetime | Goes to |
| --- | --- | --- | --- | --- |
| Configuration token | `xoxe.xoxp-` | Creates the app from the manifest through `apps.manifest.create`. Optional: the manifest can be pasted in the Slack UI instead. | 12 hours | Nowhere. Use it once and revoke it. |
| Bot token | `xoxb-` | Posts and reads messages as the bot (`chat.postMessage`, history, files, reactions). | Until revoked | `deploy/secrets/chat_bot_token` |
| App-level token | `xapp-` with `connections:write` | Opens the Socket Mode connection, so no public URL is needed. | Until revoked | `deploy/secrets/chat_app_token` |

### Create the app

1. Create the app from `deploy/slack-manifest.yaml`, separately in each workspace so it stays
   an internal app. Either:
   - UI: <https://api.slack.com/apps> → Create New App → From a manifest → paste the file; or
   - API: generate a configuration token at <https://api.slack.com/apps> (bottom of the page,
     "Your App Configuration Tokens") and call `apps.manifest.create` with the manifest as JSON.
     Then revoke the configuration token on the same page.
2. Bot token: app settings → Install App → Install to Workspace → copy "Bot User OAuth Token"
   (`xoxb-`).
3. App-level token: Basic Information → App-Level Tokens → Generate Token and Scopes → add
   `connections:write` → copy the token (`xapp-`).
4. Invite the bot to the factory channel: `/invite @nightshift`. Note the channel id and your
   own user id (profile → three dots → Copy member ID) for the tenant's `owner.chat` and channel
   settings.

### Check the tokens

```sh
curl -s -X POST https://slack.com/api/auth.test -H "Authorization: Bearer $(sudo cat deploy/secrets/chat_bot_token)"
curl -s -X POST https://slack.com/api/apps.connections.open -H "Authorization: Bearer $(sudo cat deploy/secrets/chat_app_token)"
```

Both answer `"ok":true`. Do not paste the output with tokens into issues or chat.

### Rotate the tokens

Regenerate the bot token (Install App → Reinstall) and the app-level token (Basic Information →
App-Level Tokens), overwrite the two files, then `docker compose restart nightshift`. Rotate at
once if a token appeared in a log, a screenshot or a commit.

Rotation of the other secrets and what to do on a leak: "Rotate secrets" in `docs/runbook.md`.
