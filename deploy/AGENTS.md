# deploy/

The compose stack of one instance, its egress allowlists, backups and the upgrade of the stable instance (design sections 3, 16 and 17).

## Map

| Path | What |
|---|---|
| `docker-compose.yml` | every service, network and secret; profiles `deploy` (the deployer) and `obs` (Grafana) |
| `egress-acl.yaml` | the worker's HTTPS allowlist (`egress`) |
| `egress-preview-acl.yaml` | the worker's plain http, previews only (`egress-preview`, ADR 0028) |
| `egress-install-acl.yaml`, `egress-ci-acl.yaml` | tool hosts for `toolchain`; hosts for the CI runner |
| `egress-config.yaml` | Smokescreen settings shared by every proxy |
| `init-secrets.sh` | creates missing secrets, keeps the rest |
| `upgrade.sh`, `legacy-home.sh` | check a commit on a development stack, then move the stable instance to it |
| `backup/`, `postgres/`, `grafana/` | dumps and restic, the book's roles, dashboards |

## Rules

- A new allowlist host, a wider range or a new network with a way out is an owner decision (`docs/decisions.md`).
- `upgrade.sh` and `scripts/stable.sh` run the same profiles, so the check covers what the stable instance starts.
- A service on `preview` or `internet` says why in its comment.

## Tests

`go test ./deploy/` covers the compose file, the scripts and the browser image; `make engine-test` runs the deployer's recipes on a real Docker engine.
