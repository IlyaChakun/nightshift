# Secrets of an instance

Every secret is one file in `deploy/secrets/` on the machine that runs the factory. The
directory is not in git. Docker Compose mounts each file only into the containers that
list it, at `/run/secrets/<name>`; Go code reads it with `config.Secrets` and never from
environment variables. Agents get none of these files. Design: sections 3 and 16.

## First start

1. `sudo deploy/init-secrets.sh` creates every generated secret below with mode 0400 and
   the right owner. It keeps files that already exist, so it is safe to run again.
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

The uid is the owner of the file: 70 is `postgres`, 10001 is `nightshift`, 0 is root.

| File | What it unlocks | Who creates it | Read by | uid |
| --- | --- | --- | --- | --- |
| `pg_admin` | Postgres superuser | `init-secrets.sh` | postgres | 70 |
| `pg_owner_password` | Schema owner `book_owner`, for migrations | `init-secrets.sh` | postgres | 70 |
| `pg_nightshift_password` | Service role `nightshift` | `init-secrets.sh` | postgres | 70 |
| `pg_backup_password` | Read-only role `book_backup` | `init-secrets.sh` | postgres, backup | 70 |
| `pg_owner` | URL with the owner's password | `init-secrets.sh` | migrate | 10001 |
| `pg_nightshift` | URL with the service password | `init-secrets.sh` | nightshift | 10001 |
| `chat_bot_token` | Chat bot token (`xoxb-`): read and write channels | You, from the chat app settings ([`deploy/slack-manifest.yaml`](../deploy/slack-manifest.yaml)) | nightshift | 10001 |
| `chat_app_token` | Chat app-level token (`xapp-`, `connections:write`): Socket Mode events | You, from the chat app settings | nightshift | 10001 |
| `worker_token` | Jobs API token of the worker | `init-secrets.sh` | worker | 0 |
| `worker_token_sha256` | Hash that `nightshift` checks the worker token against | `init-secrets.sh` | nightshift | 10001 |
| `github_token` | Token for GitHub (fine-grained, one repository set: contents and pull requests read and write, commit statuses write). Optional: without it the conductor stays off | You, from GitHub settings | nightshift | 10001 |
| `harness` | Directory with one file per model credential, named by the variable the harness reads (for example `CLAUDE_CODE_OAUTH_TOKEN`) | You (the directory is made by `init-secrets.sh`, mode 0700) | worker | 0 |
| `restic_password` | Encryption key of the off-site backup | `init-secrets.sh`; a copy in your password manager | offsite | 0 |
| `restic_repository` | Where the off-site copy lives (`/offsite/nightshift`) | `init-secrets.sh` | offsite | 0 |

Later phases add `tenants/<name>/` with the tracker and GitHub App tokens of each tenant, one
file per value.

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
