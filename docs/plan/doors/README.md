# Stream — other doors besides Slack

Owned by the session working on this stream; another session asks it and does not edit this file. Rules and limits: `docs/plan.md`.

Done when: a tenant without Slack, or one who works from their own Claude, drives the same task thread through the book (design section 15, ADR 0006).

## Status

- Last done: I1–I5 (inbox adapter, CLI, `nightshift mcp` over stdio, the remote door over HTTPS, `nightshift setup` picks the door).
- Next: I6 (tunnel profile), I7 (read tools), then the live check I8 on the Mac.
- Not verified live: the remote MCP door (decision 7).

## Items

Slack stays the default. A tenant without Slack, or one who works from their own Claude, drives the
same task thread through the book (design section 15, ADR 0006). They start after phase 3 items 18, 16
and 17, one item per PR, in this order. The tenant schema value `inbox` needs the owner's review.

- [x] I1–I3. inbox adapter, CLI `status`, `inbox`, `reply`, `approve`, and `nightshift mcp` over stdio
- [x] I4–I5. remote door over HTTPS (off by default, loopback, short-scope token; T6); `nightshift setup` picks the door
- [ ] I6. `feat(deploy): reach the remote door` — `nightshift` publishes no port, so a tunnel cannot reach its
  loopback listener (`deploy/docker-compose.yml`); add an opt-in `tunnel` profile with the ngrok agent
  (decision 17, `docs/research/remote-door-tunnel.md`); T6: no profile, no listener
- [ ] I7. `feat(mcp): read tools for results` — `task_timeline` (as `nightshift status --task`), `digest`
 , `counters` (24-hour task, token and cost counts behind Grafana); T1
- [ ] I8. Live check on the Mac (decision 7): a claude.ai connector with a static bearer reaches the door through
  the tunnel, Codex connects with `bearer_token_env_var`; then fix the beta line in design section 25
