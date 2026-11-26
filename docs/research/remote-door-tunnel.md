# Remote door: which tunnel

Summary: the remote MCP door (design section 15) needs a public HTTPS address on port 443 that forwards to a loopback listener on the owner's machine. ngrok's free plan covers one person's use; Cloudflare Tunnel and Tailscale Funnel are the alternatives. Recommendation: ngrok, free plan, one dev domain, our bearer token as the only login. Read 2026-11-26; every fact below is from the linked page as read by a fetch agent, not by hand, and pages carry no date unless stated.

## What the connector needs

- Anthropic's cloud connects, not the browser, so the server must be reachable from the internet on port 443; `:8443` is refused ([custom connectors guide](https://support.claude.com/en/articles/11175166-get-started-with-custom-connectors-using-remote-mcp)).
- The connector form has a "Request headers" field for a fixed bearer token, offered on Free, Pro, Max, Team and Enterprise. The page does not call it a beta. Design section 25 says it is a beta for some organizations; that line is stale or unproven, so decision 7 checks it live (plan I8).
- The page lists no source IPs but links the current ranges ([IP addresses](https://platform.claude.com/docs/en/api/ip-addresses)).

## Options

| | ngrok free | Cloudflare Tunnel | Tailscale Funnel |
|---|---|---|---|
| Stable name | one free dev domain `*.ngrok-free.app` | named tunnel needs a domain on Cloudflare DNS (the ngrok comparison says so; Cloudflare's page does not confirm) | `machine.tailnet.ts.net`, name goes to public certificate logs |
| Port 443 | yes | yes | 443, 8443, 10000 |
| Limits | 1 GB out, 20,000 HTTP requests per month, 3 endpoints ([free plan limits](https://ngrok.com/docs/pricing-limits/free-plan-limits)) | none found on the pages read | "non-configurable bandwidth limits", no number ([third-party summary, 2026-11-23](https://www.ssdnodes.com/learn/tailscale-funnel-limits-and-ports)) |
| Edge access control | `restrict-ips`, OAuth, OIDC, JWT, basic auth as traffic policy actions ([examples](https://ngrok.com/docs/traffic-policy/examples/add-authentication)); plan per action not stated | Cloudflare Access, free up to 50 users (ngrok's claim) | none beyond the tailnet policy |
| Needs | ngrok account and authtoken | a domain on Cloudflare | a Tailscale account |

The browser interstitial of the free plan applies only to HTML GETs with a browser User-Agent, not to API clients like the connector. A cookie or the header `ngrok-skip-browser-warning` suppresses it; no plan change needed.

## Fit

- **Auth stays ours.** The door checks the bearer token (`internal/mcpdoor/remote.go`). Do not turn on ngrok OAuth in front of it: the connector cannot complete a browser login at the edge (inferred). `restrict-ips` with Anthropic's ranges is an optional second layer; it matches the TCP source address, never `X-Forwarded-For`.
- **Authorization header.** ngrok's pages read do not say it is forwarded unchanged. Inferred yes; the live check proves it.
- **Free quota.** 20,000 requests a month is about 27 an hour: enough for chat control, not for a client that polls.
- **Reachability gap (checked in `deploy/docker-compose.yml`).** `nightshift` publishes no port and the schema only allows a loopback `listen`, so a host process cannot reach the door as built. Plan I6 adds a `tunnel` profile whose ngrok agent shares the `nightshift` network namespace. Not run.
- **Egress.** The ngrok agent calls out to ngrok; that is a new host, so it is the owner's decision (decision 17).

## Codex

Codex reaches a remote MCP server over streamable HTTP with `url` plus `bearer_token_env_var` (the variable's name, not the token) or OAuth ([Codex MCP docs](https://learn.chatgpt.com/docs/extend/mcp?surface=cli)). Part of that page was read as a model summary, so field names are **unverified** until plan I8.

## Status and proposal

Status: partial. The door exists (plan I4); the tunnel and the read tools do not (I6, I7); nothing ran live. Proposal: decide 17, then build I6 to I8 after phase 6 starts; the runbook section "Remote door through ngrok" is the setup.
