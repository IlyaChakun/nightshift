# Secrets

The core reads secrets only from files under `deploy/secrets/` (mounted at `/run/secrets`),
never from environment variables. `deploy/init-secrets.sh` creates the database and backup
passwords. This page covers the chat (Slack) tokens, which a human creates once.

## Slack app and tokens

Three tokens appear in the setup. Only the last two stay in the running system.

| Token | Prefix | Why | Lifetime | Goes to |
| --- | --- | --- | --- | --- |
| Configuration token | `xoxe.xoxp-` | Lets you create the app from the manifest through `apps.manifest.create`. Optional: the same can be done by pasting the manifest in the Slack UI. | 12 hours | Nowhere. Use it once and revoke it. |
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

### Store the tokens

- Production or local compose: write the two values to `deploy/secrets/chat_bot_token` and
  `deploy/secrets/chat_app_token`, then
  `sudo chown 10001 deploy/secrets/chat_* && sudo chmod 0400 deploy/secrets/chat_*`.
  Never commit them; gitleaks runs in `make verify`.
- Cloud sessions that run a live check: set the environment variables
  `NIGHTSHIFT_CHAT_BOT_TOKEN` and `NIGHTSHIFT_CHAT_APP_TOKEN`, and have the session write them
  to the files above. The core still reads files only.

### Check

```sh
curl -s -X POST https://slack.com/api/auth.test -H "Authorization: Bearer $(cat deploy/secrets/chat_bot_token)"
curl -s -X POST https://slack.com/api/apps.connections.open -H "Authorization: Bearer $(cat deploy/secrets/chat_app_token)"
```

Both answer `"ok":true`. Do not paste the output with tokens into issues or chat.

### Rotate

Regenerate the bot token (Install App → Reinstall) and the app-level token (Basic Information →
App-Level Tokens), overwrite the two files, then `docker compose restart nightshift`. Rotate at
once if a token appeared in a log, a screenshot or a commit.
