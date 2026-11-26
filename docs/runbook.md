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
2. Follow [setup sections 1–3](setup.md) to prepare the mounted files and your chosen chat
   adapter. Slack uses two tokens. The terminal inbox works with empty chat files.
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
   write, Checks, Actions and Code scanning alerts read. Never give it Workflows.
2. Install it on the tenant's repositories only.
3. Write the App ID to `deploy/secrets/github_app_id` and move the downloaded private key to
   `deploy/secrets/github_app_key`; keep both 0400 (Linux: owner 10001). If the empty files
   exist, `chmod 0600` them first.
4. `docker compose up -d --force-recreate nightshift`.

A fine-grained token in `deploy/secrets/github_token` (same permissions) also works, with two
limits: you cannot approve a pull request opened with your own token, and CI comes from Actions
runs and commit statuses, because such a token cannot read check runs.

The repository needs a CI check on pull requests: the conductor waits until one reports.

The conductor checks for work every 15 seconds. With a Lead provider, a chat idea goes
through spec approval. To queue work directly for Builder, run
`nightshift task add <tenant> "<what to do>"` in the `nightshift` service.
A task nothing watches any more (a spec nobody wants, a stuck one) is ended with `nightshift cancel [--tenant name] <task>`;
a task released already cannot be canceled, only rolled back. A pull request that you or someone else merges or closes on
GitHub while its task has one (from building to ready) moves the task to `merged_by_team` or `canceled` on the next tick,
stops its jobs and tells you in the thread; a task that depends on it then starts.
Plan ahead with `--not-before 2026-11-26T22:00:00+01:00` (it does not start earlier) and
`--priority N` (-10 to 10; a higher one goes first): write the tasks by day, they run at night. For a Lead spec, give each scheduled task its earliest start with a time zone or its priority. Check the schedule on the spec card before approval.

Facts Lead should not ask twice live on the tenant's memory page (at most 40 one-line entries and 4 KB;
an entry expires after 90 days). `nightshift memory remember <tenant> "<fact>"` adds one and
`nightshift memory show <tenant>` lists the page; `nightshift search <tenant> "<words>"` searches the book (tasks, decisions, documents; add `--kind` or `--limit`); a full page is refused until you merge or drop an entry.
The page goes whole, as data, into the prompts of Lead and Builder (expired entries left out), and once a day the factory
drops expired and repeated entries without a model, recording each drop in the book (`memory.written` events).
A tenant whose `tracker` adapter is built (only the twin `fake` for now) gets one issue per task, updated at every state; a cancel or a P0 to P3 priority set in the tracker by the owner or an approver comes back, and nothing else does. A tenant with another adapter name runs without a projection and the log says so at start.

In Slack, the owner or an approver can use `/ns status` (the 24-hour summary), `/ns memory`, `/ns remember <fact>` and `/ns search <words>` (the best hits of the tenant's book); the answer is one message in the channel, and anyone else gets none.
Lead also proposes entries (`memory[]` in its reply); nightshift keeps one only when it quotes an owner message of that task or names a file of the repository, and records `memory.accepted` or `memory.refused` events.

`nightshift digest <tenant> [--since 24h] [--send]` prints the morning digest, at most 3000 characters whatever the window.
Blocks are separated by a rule and the head comes first: the window ("last 24 h (since ...)", in the tenant's time
zone), whether anything waits for you and the state of `main`, how many pull requests were merged and by whom, and how
the factory's own tasks went. Then "Needs you" (a task that a later task with the same title replaced is left out),
"What changed" (the merges by result: new, fixed, reverted and other, each with its three newest titles, and one count
for docs, tests and upkeep; a merge that a later revert in the window takes back is left out), "Plan" (up to
six open parts of the plan under `docs/plan/`, each a folder with its `README.md` or a file, by their headings, phases
in order, with their checklist items done of all; past six, the ones with the most open items), "Merged" (at most 8 pull requests, newest first: protected paths with a
link to all of them, code without a recorded independent review, the factory's own work, and on a day with lines to
spare the reviewed code and docs; the rest is a count with a link to GitHub), "Decisions" (at most 5), the health
detail when something is wrong, and a "Conclusion" (what merged without you and where it is listed; "Nothing to do
today" on a clean day; none when the head already says what waits or what is wrong). The head, "Needs you", "Merged"
and the conclusion, when there is one, always show. When the message is too long, "Merged" gives up lines, down to two,
as far as that lets the health detail, the counts of what changed and the plan show; the other blocks show while they fit, tried in
this order: the health detail, what changed as one line of counts, the plan, the titles of what changed, the
decisions. Each listed pull
request carries its URL, where the Revert button is. The command needs the GitHub App or token to list the merges; a
list that cannot be read is said so at the top, never shown as empty.
`/ns status` has a head of its own (task counts and merge numbers) and the same "Needs you" block. In Slack the rules are drawn as dividers.

With the `slack-app` chat adapter and `people.owner.chat` set, the same digest is sent to the owner once a local day after
`schedule.morning_summary` (default 09:00, in `schedule.timezone`). If the code host cannot list the merges it asks again
every 10 minutes and after two hours sends the digest marked as incomplete. Tenants on the `inbox` adapter read it with
`nightshift digest`. `--send` also queues the digest for the owner under a key of its own, so it goes out even when the daily one already did (it refuses while the merge list is incomplete); the running `nightshift serve` delivers it within seconds (without one it waits in the outbox), and the tenant needs the `slack-app` adapter and `people.owner.chat`.

Quiet hours: `schedule.quiet_hours: "22:00-09:00"` (in `schedule.timezone`; the window may cross midnight, nothing set means no quiet
hours). In the window the chat cards `decision_needed` and `ready_for_acceptance` wait and go out when it ends, and a merge that
is followed by a production release waits too, unless `schedule.releases_in_quiet_hours: true`. Answers to your own messages,
blockers and the digest go out at once. A tenant whose repository declares no release target merges at any hour. If the task stays in `merging`, check the conductor log for a capability or quiet-window read error. Check the project file on the base branch, the release adapter and its recipe, the `serve -deployer-url` setting and the tenant schedule. The next tick retries.

Review waits: `review.remind_after_hours: [4, 24]` sends the owner a reminder card after each wait (once each, and held until morning
in quiet hours). `review.sla_automerge_hours: 12` is off unless set and never works for `governance: team`: a pull request that
the blast-radius policy rates `low` and that nobody reviewed in that time is approved and goes to the merge train; the owner gets a
notice. It requires a passing Verifier review of the current head, an eligible release window and an unchanged blast radius. If the Verifier was unavailable, the task history shows `verifier_unavailable`; the task waits for an authorized GitHub review.

If the task history shows `review_required` after resuming an open pull request, an authorized GitHub reviewer can approve or request changes on the task's recorded revision. The task keeps its branch and pull request while it waits.
If recovery reports that the host head has no completed train rebase, keep the task paused and retain its PR and train logs for [head reconciliation](plan/phase-6/A01r-resume-evidence.md).

When a model provider hits its limit the task stays where it is. The factory waits for the reset
(a limit that gives no reset time waits 15 minutes, doubling to 6 hours) or, for a reset later than
`on_limit.max_wait` of `tenant.yaml` (default `2h`), uses a provider marked `tier: fallback` in
`providers.yaml`, but only for a task queued with `--allow-fallback` (`on_limit.fallback: always`
allows every task, `never` none). Lead never uses a fallback provider.

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

## Remote door through ngrok

Not built yet and not enabled by default (plan I6 to I8; decision 17 chose ngrok for a local instance). A deployed instance has its own URL and skips this section. The factory runs on your Mac; a claude.ai
project or Codex controls it through the remote door: status, new tasks, Lead's questions and spec approval.
Merge to prod, rollback and settings stay in Slack or the terminal. Why ngrok:
`docs/research/remote-door-tunnel.md`.

1. Create a free ngrok account, copy the authtoken and the free dev domain it shows (`<name>.ngrok-free.app`).
2. Turn the door on in `tenants/<name>/tenant.yaml`, then restart `nightshift`:

   ```yaml
   mcp: { remote: { listen: 127.0.0.1:8765 } }
   ```

3. Start the ngrok agent with the `tunnel` profile (plan I6) so it forwards `https://<name>.ngrok-free.app` to
   `127.0.0.1:8765`. Do not enable ngrok OAuth; the bearer token is the login.
4. In claude.ai, add a custom connector with the URL `https://<name>.ngrok-free.app/mcp` (port 443) and the
   request header `Authorization: Bearer <contents of deploy/secrets/mcp_token>`. For Codex set `url` and
   `bearer_token_env_var` in its MCP config.
5. Check that `status` answers. Every decision made this way records the actor `mcp-remote`.

Stop it by stopping the agent; rotate the token as in "Rotate secrets". The free plan allows 1 GB and 20,000
requests a month.

## Connect a project

`nightshift tenant init <name> --discover --repo-dir <checkout>` writes drafts into
`tenants/<name>/onboarding/`. After the `onboard-tenant` skill has filled them in, add
`--open-pr owner/repo` (reads the `github_token` secret from `--secrets`) to open a draft pull
request that adds `.nightshift/project.yaml`; the owner reviews and merges it. Then run
`nightshift tenant doctor <name> --repo-dir <checkout>`.

Before a restart or a PR, `nightshift config validate --config <instance dir> --repo-dir <checkout> --tenant <name>`
checks the instance, its tenants and the project's `.nightshift/` files against `schemas/`, and the
paths and stacks they name. It reads files only, so it needs no secrets and runs on a laptop; the
exit code is 3 when anything is an error.

`capacity.tasks` in `tenant.yaml` bounds how many of the tenant's tasks the factory starts: while that
many are in work, a new task stays `queued` (`nightshift status` shows it) until one is accepted or waits
for a person. A task sent back to work by a review, a rollback or a merge-train conflict continues at once. Without it there is no limit.
`budget.weight` (default 1) is the tenant's share of the workers: a free worker takes the oldest job of
the tenant with the fewest running jobs per weight, so a tenant with weight 2 gets about twice as many.
Each tenant is advanced by its own loop. A repository belongs to one tenant: the instance does not start
when two tenants list the same one in `repos`, and `nightshift config validate` reports it. To move a
repository to another tenant, first finish or cancel the old tenant's tasks on it.

To change a connected tenant later (stack, skills, instructions, blast radius, review mode, capacity),
give your agent the `configure-tenant` skill (`skills/own/configure-tenant/`): it edits the files,
runs `config validate` and `tenant doctor`, and opens a PR to the project. It never touches secrets,
egress or the live config: a `tenant.yaml` change comes to you as `tenant.yaml.proposed` beside the file
and a diff; put it in place and run `docker compose restart nightshift`.

To brief one role for a project, add `.nightshift/roles/<lead|builder|verifier>.yaml` with
`instructions: [docs/x.md]` (files of the repository) and `skills: [own/audit]` (ids from the shared set),
eight of each at most. It only adds; any other key, a bad path or a missing file skips the whole file
with a "role override skipped" warning in the log, and the role runs as the factory defines it.

## Previews and releases on this machine

A project with `compose-local` in its `capabilities` gets a preview per task and a release at the merge, brought up by the `deployer` service (design section 17). Nothing runs until you start it, and the first use needs these steps:

1. Run `deploy/init-secrets.sh` again: it adds `deployer_token` and keeps every other secret. Compose refuses to start `nightshift` without that file.
2. Pick a folder for the checkouts and export it, for example `export NIGHTSHIFT_DEPLOY_DIR=$HOME/nightshift-deploy` (default `/tmp/nightshift-deploy`). On macOS add it under Docker Desktop's file sharing; the service mounts it at the same path as on the host.
3. Start with `docker compose --profile deploy up -d --build`.

The deployer holds the Docker socket, which is root-equivalent on this machine, so only it has it: the worker and the agent cannot reach it. It reads the recipe from `main`, so a recipe change counts only after it is merged. The recipe is one file: `include` and `extends` of another file are refused (`extends` of a service in the same file works). Before `up` it also refuses privileged mode, host networking, host paths, added capabilities and devices; a preview that breaks a rule goes back to Builder with the reason. Without the `deploy` profile a tenant with `compose-local` keeps its tasks waiting at the preview step. A project's own smoke command is not run yet; the health checks of the recipe decide.

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

The restore test builds its scratch database from `template1`, which carries the `vector` extension
on instances created after 2026-11-25. On an older data directory add it once, as the superuser:
`docker compose exec postgres psql -U postgres -d template1 -c 'CREATE EXTENSION IF NOT EXISTS vector'`;
until then the weekly test fails with "permission denied to create extension".

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

## Drill: do the backups and the isolation really work

`make drill` (or `scripts/drill.sh`) needs Docker and about ten minutes the first time, when it builds
the worker image. It starts a throwaway compose project named `nightshift-drill` with its own volumes
and without the `nightshift` service, so it never touches your stack or connects to chat or GitHub.
It prints one line per check and ends with `N checks, M failed`:

- Phase 1: Postgres starts and migrates; a marker row is dumped by `backup`, the weekly restore test
  passes, the book is dropped and restored by the steps above, and the marker is still there; state
  survives a restart of Postgres.
- Phase 2: as the agent user in a worker container there is no GitHub, chat or tracker token, the
  worker's secrets are unreadable (and mounted, read as root), an allowlisted host answers through the
  proxy, and a host outside the allowlist, a private address, a direct connection and Postgres are all
  refused. Each "refused" check has a control that proves the tool works, so a broken container cannot pass.

Run it after changes to `deploy/` or the worker image, and note the date and result in
`docs/status.md`. `--keep` leaves the project running for a look; remove it with
`docker compose -p nightshift-drill -f deploy/docker-compose.yml down -v`.

## Read a task's history

`nightshift status --task 42` prints one task's timeline: every move, every job with its
provider, the model and effort it asked for, the model the harness reported it ran, how long it ran, its tokens and cost, and why it ended when it did not finish
(for example the supervisor's `stopped` reason or the harness error). Times are in the
tenant's `schedule.timezone`. Run it inside the service like the other terminal commands:

```sh
docker compose exec nightshift nightshift status --task 42
```

In Grafana (obs profile) the same lines are the Task timeline dashboard ([Observability](#observability-obs-profile)).

For the raw rows, query the book. Every state change and decision is a row in `book.events`. The table is append-only, so the
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

## Choose a role's model and effort

Config decides which model a role runs and how hard it thinks, never the agent (ADR 0020).
Lead and Verifier ship with `claude-opus-5-5` at effort `high`. Builder keeps the CLI default.

| Setting | Role (`roles/*.yaml`) | Tenant (`tenant.yaml` `roles.<role>`) | Provider (`providers.yaml`) |
|---|---|---|---|
| Model | `model: { claude: <id>, codex: <id> }` | `model: { claude: <id> }` | `model: <id>` wins over both |
| Effort | `effort: high` | `effort: max` | none |

Use full model ids. An alias such as `opus` moves with the CLI version, so the schemas reject it.
The levels are `low`, `medium`, `high`, `xhigh` and `max`. On `claude` a model with fewer levels
runs at the highest one below the level you set; `codex` gets the level as it is. A job's `model` and `effort` (in `book.jobs.payload`) show
what each run was asked to use:

```sql
SELECT id, role, payload->>'model' AS model, payload->>'effort' AS effort FROM book.jobs ORDER BY id DESC LIMIT 10;
```

## Stop a runaway agent run

The worker stops an agent run that hangs, loops or overspends (ADR 0019). A stopped job ends
with the exit `budget`, and its result says why, for example
`{"stopped":"loop: Bash called 6 times in a row with the same input"}`.

| Rule | Worker default (`nightshift work` flag) | Role (`roles/*.yaml` `limits`) | Tenant (`tenant.yaml` `budget.run`) |
|---|---|---|---|
| No output for a while | `--stall-after 30m` | `stall_minutes` | `stall_minutes` |
| The same tool call in a row | `--repeat-limit 6` | `repeat_limit` | `repeat_limit` |
| Tokens of one run | the role's `tokens` | `tokens` | `tokens` |
| Dollars of one run | none | `cost_usd` | `cost_usd` |

Each column overrides the one to its left; an absent key inherits, and `0` switches the
rule off. Dollars are priced from tokens with the provider's `price`, so the dollar cap needs
a `price` on the provider; a subscription provider has none.

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
- Codex on a ChatGPT plan (`codex-sub`, personal instance only): a Codex personal access token of your
  own ChatGPT account, which starts with `at-`. Where ChatGPT creates it is not verified yet (the first
  live run records it in `docs/plan/live-checks.md`). Check the token on the Mac first:
  `printf '%s' "$TOKEN" | CODEX_HOME=$(mktemp -d) codex login --with-access-token`, then
  `printf '%s' "$TOKEN" > deploy/secrets/harness/CODEX_ACCESS_TOKEN`. The adapter passes no API key and
  forces the ChatGPT login, so a run never bills the API; one session at a time (design section 12).
  Never use it for company code or for other people.

Then `sudo chown 10001 deploy/secrets/harness/* && sudo chmod 0400 deploy/secrets/harness/*`, build with
`CODEX_VERSION=<version> docker compose build worker` (pin a version from `npm view @openai/codex version`, at least 0.172.0 for `codex-sub`;
for Claude set `CLAUDE_CODE_VERSION` from `npm view @anthropic-ai/claude-code version`; keep both in
`deploy/.env` so a later build does not drop a CLI) and add
the provider to the instance's `providers.yaml` and the role to the tenant's `providers`. Egress allows
`api.openai.com`, `api.deepseek.com`, and `chatgpt.com` with `auth.openai.com` for `codex-sub`, only; another endpoint needs its host in `deploy/egress-acl.yaml`.
Every worker holds every file in the directory, so keep only the keys you use there.

`nightshift eval run --harness claude` reads the same directory: pass `--secrets deploy/secrets` (it looks
in `<dir>/harness/`) and run it as a user who can read the files. It stops with "eval did not run" when the
credential for `--auth` is missing, and also when the CLI is not logged in, hits a limit or crashes; such
a task or case is never graded and no result is saved. The credential goes to the CLI through its environment only, never
to a command line, a log or a result file.

## Merge from a Claude Code session without a bypass

Claude Code decides what a session may run; the factory's merge rule (`AGENTS.md`) decides what it
should. When `gh pr merge` is blocked although `.claude/settings.local.json` allows it, find the
cause before changing anything (rules and modes: [permissions](https://code.claude.com/docs/en/permissions),
[permission modes](https://code.claude.com/docs/en/permission-modes), read 2026-11-25):

1. `/permissions` lists every rule and the file it comes from. A `deny` or `ask` rule for the command in
   any file (user, project, managed) beats a local `allow`: the order is deny, ask, allow.
2. A compound command (`gh pr merge 205 && git pull`) needs an allow rule for each part.
3. `/status` shows the mode and the setting sources. A sandbox that does not allow `api.github.com` stops
   `gh`: add it under `sandbox.network.allowedDomains`.
4. A hook that exits with 2 blocks the call even when a rule allows it.
5. `claude --debug-file /tmp/claude.log` shows which of these fired.

Not verified in the docs: the exact denial text, whether an allow rule applies in plan mode, and which
checkout a worktree session reads `settings.local.json` from.

The proper setup keeps the control in two places the session cannot edit away: the allow rule is narrow
(`Bash(gh pr merge *)`, in the user's own settings, approved by the owner once), and a `PreToolUse` hook
on `Bash` refuses a `gh pr merge` unless the PR's checks are green, it touches no protected path
(`.github/`, `deploy/egress*`, the ruleset files) and it carries the independent-review label. A hook
that exits with 2 wins over the allow rule. Pull requests of a session on the Mac are merged by that
session, never by a cloud thread on its behalf: that would route around the control. The factory's own
tasks are merged by the factory. `--dangerously-skip-permissions` and `bypassPermissions` are never used
(`CLAUDE.md`).

## Several projects on one instance

One instance builds several projects at once, one tenant each (design section 17, ADR 0015).

1. **Add the tenant.** Make `tenants/<name>/tenant.yaml` in the instance config, for example with
   `nightshift tenant init` (see "Connect a project"). Each repository belongs to one tenant.
2. **Check before the restart:**
   `nightshift config validate --config <instance dir> --repo-dir <checkout> --tenant <name>`.
3. **Restart** with `docker compose restart nightshift`. Tenants are read at start; a running instance
   does not see a new directory. `nightshift doctor` then lists the tenants and their GitHub credential.
4. **Optional, its own GitHub account:** if its repositories belong to another account, give it
   `deploy/secrets/tenants/<name>/github_token` ([secrets](secrets.md)).
5. **Share the instance.**
   - `capacity.tasks` in `tenant.yaml` bounds how many of its tasks run at once.
   - `budget.weight` sets its share of free workers.
   - Each tenant has its own conductor loop, so a slow project does not hold the others back.

**More than one project at a time on the subscription.**

- `claude-sub` ships with `concurrency: 1`, so two projects' jobs on it run one after the other.
- To run them side by side, raise `concurrency` on `claude-sub` in the instance's `providers.yaml` and restart.
- The claim keeps that number across all tenants. Every job then draws from the one plan limit (see
  "Who uses the model limit").
- A numeric `pools.<name>.monthly_usd`, including zero, requires a guaranteed maximum charge reserved before a worker starts. The current harnesses report usage after spending. Jobs for those pools wait in the queue even when token prices are configured. The service logs the provider and pool on startup. A pool with `monthly_usd: null` has no factory monthly ceiling. A provider naming an undefined pool fails config validation.
- `weekly_share` in `providers.yaml` is not enforced yet. Before you raise `concurrency`, check what the
  tenants used last week:

```sql
SELECT tenant, count(*) AS jobs,
       sum((usage->>'input_tokens')::bigint + (usage->>'output_tokens')::bigint) AS tokens,
       count(*) FILTER (WHERE exit_reason = 'rate_limit') AS limit_hits
FROM book.jobs WHERE provider = 'claude-sub' AND finished_at > now() - interval '7 days'
GROUP BY tenant ORDER BY tokens DESC;
```

Limit hits in that week mean the plan is already full. Then either keep `concurrency` as it is, or list an
API provider ahead of `claude-sub` for the busier tenant's roles. An API provider costs money.

**Not yet built:**

- reloading tenants without a restart;
- a share of a provider per tenant;
- a HOME per tenant on the worker (plan M3, which also lists the other shared worker state).

## Who uses the model limit

`claude-sub` runs `claude -p` with a subscription token (`claude setup-token`), so the factory's Builder,
Lead and Verifier draw from the same plan limits as the owner. The help article says that, signed in
with a plan, "the Claude Agent SDK and `claude -p` still draw from your subscription limits", and that
the monthly API credits of Max, Ultra and Team plans apply only to runs with an API key from a linked
Console organization ([Agent SDK and your plan](https://support.claude.com/en/articles/15036540-use-the-claude-agent-sdk-with-your-plan),
read 2026-11-25). The article does not name Claude Code on the web; that those cloud sessions share
the plan's limit is inferred (three sessions stopped together on 2026-11-24), not documented. Both the
development and the stable instance use the one token, and `claude-sub` allows one session at a time.

To see who spent what, from the book (`docker compose exec postgres psql -U postgres -d book`):

```sql
SELECT date_trunc('day', finished_at) AS day, provider, role, count(*) AS jobs,
       sum((usage->>'input_tokens')::bigint) AS input_tokens,
       sum((usage->>'output_tokens')::bigint) AS output_tokens,
       count(*) FILTER (WHERE exit_reason = 'rate_limit') AS limit_hits
FROM book.jobs WHERE finished_at > now() - interval '7 days'
GROUP BY 1, 2, 3 ORDER BY 1, 2, 3;
```

Compare the `claude-sub` rows with the times your own sessions hit a limit. Which credential a role runs on
is the operator's choice per instance: the owner's instance runs on his subscription, and an operator who
would rather not share the limit lists an API provider first in `providers:` (docs/setup.md, step 2).

## The worker image (Debian, mise)

The worker is Debian slim with mise, git, make, build-essential, Node (for the agent CLIs) and no
language toolchain (ADR 0009). A project pins its toolchain in `mise.toml` or `.tool-versions`;
before each job the runner runs `mise install` (into the `agent-home` volume, so the second job
installs nothing) and then the project's `setup`. A first job on new versions downloads them through
the install allowlist (see [A project's tools fail to install](#a-projects-tools-fail-to-install)). `MISE_VERSION=<apt version>`
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
tool hosts, through its own proxy `egress-install` and the allowlist `deploy/egress-install-acl.yaml`
(Go tools built from source come through `modcache`).
If a job fails with "toolchain install failed", read the output in the job's error, then
`docker compose logs toolchain egress-install`. A denied host means the tool comes from a host that
is not in that allowlist (Java and Rust hosts are added here, per instance, with the first such
project). Add it to `egress-install-acl.yaml`, never to `egress-acl.yaml`, which belongs to the agent.
Installed tools live in the `agent-home` volume, one directory per tenant (`tenants/<tenant>`), and survive restarts. Two tenants that pin the same tool versions install them twice.

## Copies of repository mirrors

The `repos` volume may hold `<name>.git.broken-<n>` (a mirror damaged on disk) or
`<name>.git.unconverted-<n>` (a mirror that could not be reduced to branches and tags). No job reads
them. They use disk, and an unconverted one still holds the commits of fork pull requests. Delete them
once the log shows the new mirror works, unless you need a branch from them that was never pushed.

## Vendor an external skill

External skills are pinned by commit SHA in `skills/vendor.yaml` (design section 12). Adding or updating one needs the owner's Approve.

1. Add an entry: `id` (`<set>/<name>`), `repo` (https), `path`, `sha` (40 hex characters, never a branch or tag), `license`, `roles`, `applies_to`.
2. Run `nightshift skills sync --dry-run` and read the report. It refuses a missing license, a bad SHA, scripts, executable or binary files, links, hidden unicode and, when `gitleaks` is installed, secrets. Nothing is written while any entry is refused.
3. Run `nightshift skills sync` to write `skills/vendor/<id>/` (the skill, its license and `VENDORED.yaml`), and open a pull request with the report in its description.

A skill that is not in `skills/vendor/` is skipped by the worker with a warning.

## A language the factory has no pack for

Connect the project anyway: pin its tools in `mise.toml`, set `verify` (and `setup` for the dependency
install) in `.nightshift/project.yaml`. `tenant doctor` warns that no pack covers the stack. To add
structured checks, put a pack in the project at `.nightshift/stacks/<name>/pack.yaml` (the schema of
`stacks/*/pack.yaml`). A project pack with a factory pack's name replaces it for that project. The
folder is a protected path: the agent cannot change its own checks.

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
breaking the development stack in the checkout does not touch it. Its tenant holds every
change for the owner's Approve on GitHub except `low` risk (documents and test files within
the limits of `.nightshift/blast-radius.yaml`), which merges on green CI:

```yaml
review:
  mode: blast_radius
  actions: { low: auto, medium: review, high: review, critical: review_design_and_pr }
```

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

### Before you delete or re-clone the checkout

The checkout holds four paths that git does not track and the instance cannot start without:
`deploy/secrets/`, `deploy/config/`, `deploy/.env` and `.env`. The stable release links to
`deploy/secrets/` in the checkout, so removing the folder also stops the stable instance at its
next start. The backup covers the book only, so nothing else keeps a copy of them.

1. Rename the old folder, do not delete it: `mv nightshift nightshift-old`.
2. Clone again and copy the four paths back with their modes and owners (`cp -a`, with `sudo`
   for the files a container uid owns).
3. Start the stable instance, `scripts/stable.sh up "$(cat ~/.nightshift/stable/current)"`, and
   remove `nightshift-old` only after its doctor passes.

If the old folder is already gone, look in the Trash first; without it every secret is issued
again ([secrets](secrets.md), "Rotate secrets" above).

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

Move to a new tag only after a green live check of that commit. One command does both:

```bash
deploy/upgrade.sh --check-only     # check origin/main, change nothing
deploy/upgrade.sh                  # check origin/main, then tag it and upgrade
```

A commit of `main` can be named as the last argument. In order, it waits for `ci-ok` (and stops at once, naming the cause, when every `verify` run of the commit was cancelled by a newer push: name a newer commit), runs the
[drill](#drill-do-the-backups-and-the-isolation-really-work), waits until the stable instance has no job and stops its worker, builds the
development stack from the commit (with this checkout's `deploy/config`, `.env` and secrets),
runs both doctors, installs the pinned tools from an empty directory, downloads a module through
`modcache`, takes a sandbox task of the tenant `example-solo`
(`NIGHTSHIFT_CHECK_TENANT`) to `released`, runs the off-site copy and the restore test and reads
the logs. It stops the development stack and, only if everything passed, tags
`factory-stable-<date>[.N]`, pushes the tag and runs `scripts/stable.sh up`. If startup fails, a service fails during the settling period, the release logs contain errors, or the script cannot read the service state or logs,
the upgrade restarts the previous tag. It checks the running core and worker against that tag's
Compose file. The upgrade exits unsuccessfully even after a confirmed recovery. If recovery
cannot be confirmed or the upgrade is interrupted after activation begins, run the printed
`scripts/stable.sh up <previous-tag>` command. The candidate tag remains in Git.
After a successful upgrade, the script moves the CLI
state that older releases kept straight in `/home/agent` (`.claude`, `.claude.json`, `.codex`) into
`/home/agent/.pre-tenant-state/<time>`, which only root can read (`deploy/legacy-home.sh`, run as root
in a one-off worker container). Nothing is deleted: look at the copy and remove it when you no longer
need it. The development stack's volumes stay after `down`. Restart it with `docker compose up -d`.

It needs `docker`, `gh` signed in for this repository, and a development stack that serves a
sandbox tenant. The annotated tag carries your git committer email: with GitHub's email privacy
on, set `GIT_COMMITTER_EMAIL` to your `noreply` address. Book migrations only add, so an older
release reads a newer book.

## Upgrade the factory

The factory's own instance is upgraded with `deploy/upgrade.sh`, see
[Stable instance](#stable-instance). Phase 6 adds the fast polygon to its checks.

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

The Nightshift folder holds the factory dashboards; Grafana reads them from the book as
`grafana_ro`. **Task timeline**: pick the tenant, type a task id, and see every event and job of
the task, the same lines as `nightshift status --task <id>` without payload or job result text.
An instance set up before this dashboard has no `pg_grafana_password`: `scripts/stable.sh up` and
`deploy/upgrade.sh` run `deploy/init-secrets.sh` to create it; with plain `docker compose`, run
that script yourself (on Linux with `sudo`). Without the obs profile nothing needs it. Dashboards live in `deploy/grafana/dashboards/`; they are read-only in the UI.

## Idle pull requests

The workflow `pr-sweep` runs every 2 hours (and by hand: Actions, `pr-sweep`, Run workflow). It runs `scripts/pr-sweep.sh`, which uses no model and no secret, only `gh` with the workflow's own token (`pull-requests: write`). It flags an open pull request that conflicts with `main` (it then gets no CI), has no checks 15 minutes after it was opened, or has no commit for 2 hours (`IDLE_HOURS`). A flagged PR gets one comment per day, and the list goes to the job summary. The sweep changes nothing else: it neither merges nor closes. Whoever sees the comment merges the PR or closes it with a reason (skill `finish-task`, "Open PRs"). To change a rule, edit the script and `scripts/test-pr-sweep.sh`; the test runs in `make repo-check`.

## Health tasks

Once an hour the conductor reads each tenant's repositories (`summary.BuildHealth`, the same
reading as the digest's "Factory health"). For each open code-scanning alert it queues the task
"Fix code-scanning alert #N in owner/name", and for each non-draft pull request idle for 24 hours
that has no task of its own, "Unstick pull request #N in owner/name", at priority -5 and at most
3 new tasks per run. One task per alert or pull request, ever: the title is the key, so a
finished task whose alert is still open does not open another (read the alert then). A task that
has not started is canceled when its alert is fixed or its pull request moves on. A part that
could not be read changes nothing. The alerts need the App right in
[setup](setup.md#an-app-made-before-the-code-scanning-alerts-right). The `pr-sweep` workflow above
only comments; these tasks do the work. To stop one, cancel it (`/ns cancel`).

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
