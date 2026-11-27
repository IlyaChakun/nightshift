# internal/experience/

Experience plane (ADR 0026, design file `docs/design/01-experience.md`): the doors through which people reach the factory. A door turns a request into a command; control decides what happens.

| Package | Owns |
|---|---|
| `mcpdoor` | the inbox doors as MCP tools, local and remote (bearer token) |

The chat doors (`adapters/chat/slackapp`, `adapters/chat/inbox`) are adapters and stay under `internal/adapters/`.

## Rules

- May import experience, control, observability, data, customization and `integration`; `internal/archtest` enforces it. Nothing imports a door but a composition root.
- Every door gives the same command the same meaning; approvals are checked by control, never by the door.
- A new door is a package here or an adapter with its twin, wired in `cmd/nightshift`.
