# Security

> Part of the [Nightshift design](../design.md). Cross-cutting rules for secrets, egress, isolation of the worker and data classes. Every plane follows them.

## 16. Security

**What we protect:** private and corporate code, secrets (chat, tracker and GitHub App tokens, model credentials, kubeconfig), project production, client data, the identities of people in chat.

**Threats:** prompt injection from tickets, the web and previews; poisoned standards and skills through learn; destructive agent mistakes; malicious dependencies; tampered tools and actions (the March 2026 retag of `trivy-action`); credential theft from the worker; self-damage; forged commands; mixing personal and corporate data.

| Control | How |
|---|---|
| Least privilege for the agent | The agent and all project code run as a separate OS user with no GitHub, chat or tracker token. The worker reads code from the read-only `repos` mirror and returns a patch; `nightshift` applies it in its own bare copy (`core.hooksPath=/dev/null`, no project hooks or filters), names the branch `ns/<task_id>` and pushes with the GitHub App token. Merge, chat and tracker belong to `nightshift` only |
| Project commands | `preview`, `release`, `rollback` and `smoke` come from `main`, not the PR branch, and run in a `deployer` job with that tenant's credentials only (section 17); the worker never holds a Docker socket, kubeconfig or SSH key. With GitOps, release is a merge and a tag |
| Secrets | Compose `secrets:` files in `/run/secrets` (worker: `/run/worker-secrets`), never environment variables: Go code never reads the environment (forbidigo), and the runner passes the harness only the variable it needs. Tenant secrets live only in `nightshift`. The subscription uses `claude setup-token`. Known secret values (and URL passwords) are masked and gitleaks scans every output: artifacts, logs, OTel, chat, GitHub, tracker |
| Database and jobs API | The worker never connects to Postgres (section 3). A job gets only its role's data: Builder never sees hidden scenarios, no agent reads reviews or decisions or forges an Approve. A job token is bound to task and role and dies with the job |
| Gates in GitHub | `team`: the organization's branch protection and rulesets plus the required status `nightshift/human-review`; the team merges. `solo`: rulesets need a paid plan for private repositories, so `nightshift` holds the gates and only it merges. The GitHub App has no `workflows` permission; workflows use `permissions: contents: read` (code scanning adds `security-events: write`) and Actions has no secrets. With a paid plan or a public repository a ruleset is a second layer |
| Containers | Non-root (only the worker's runner is root, with just `SETUID` and `SETGID`), read-only rootfs, `no-new-privileges`, `cap_drop: ALL`, CPU, memory and process limits, no Docker socket, one job per container, ephemeral gh-runner |
| Outbound traffic | The worker's only way out is Smokescreen by allowlist: model APIs, read-only registries, vulnerability services; plain http goes to `egress-preview`, which reaches only live previews by name on the `preview` network; private ranges denied; gh-runner has its own list. Web research is a separate job without code. The outbox posts with unfurling off and strips links to hosts outside the allowlist. This cuts the "private data + foreign text + a way out" leak |
| Prompt injection | Foreign text is data; agents have no side-effect tools; results pass JSON Schema; irreversible actions only by button; Verifier only reads; learn's conclusions from foreign text go to the owner (section 9) |
| Dependencies | A new dependency is blast radius `critical` (owner in any mode, no SLA) and needs a research record (section 6). Lockfile mandatory, installs with `--ignore-scripts`, age, popularity and vulnerabilities checked by `nightshift deps check` |
| Supply chain | `mise.lock` checksums, actions and external skills by SHA, zizmor and actionlint, updates only through PRs |
| Self-damage | The constitution (section 10): protected paths; weakening gates, rules or role prompts needs the owner's Approve |
| Forged commands | Only events from the tenant's `owner` and `approvers` count; buttons carry the card version. Mirrors hold only the repository's own branches and tags, and CI counts no fork's Actions run; checks of other apps and commit statuses count whoever triggered them. The terminal and MCP doors act as the owner only with a token from the secrets directory; the remote MCP door is off by default, listens on `127.0.0.1` behind a tunnel the person starts, and cannot merge, roll back or change settings (section 15) |
| Instances and data classes | Personal and corporate code live in different instances that share no secrets, books, artifacts or logs. `data_classification` limits providers, attachments, exports and retention (section 14) |
| Package and tool hosts | The worker's allowlist opens no download hosts (`github.com` would be a leak path). `modcache` (Athens) serves Go modules (ADR 0008); `toolchain` reaches the tool hosts, the npm registry and the Sigstore trust root through its own allowlist, once per job before the agent starts, and refuses a second call (ADR 0010). For private registries `nightshift` runs a proxy mirror; the agent gets no registry tokens |
| Integrity and audit | `events` is append-only; the runner records artifact sha256 in the book |

**Residual risk:** harness credentials live in the worker and the agent can read them. Mitigation: the egress allowlist, a dedicated factory token, rotation every 3 months and on suspicion (`docs/runbook.md`), API credits without auto-reload.
