# Security policy

Nightshift runs coding agents next to real repositories and credentials, so security reports are welcome and handled first.

## Report a vulnerability

Do not open a public issue. Report it privately: [**Security > Report a vulnerability**](https://github.com/IlyaChakun/nightshift/security/advisories/new). Only the maintainer and you see the report.

Include what you found, how to reproduce it, and what it lets an attacker do. Expect an answer within a few days.

## What counts

Anything that breaks a promise in [`docs/design.md`](docs/design.md) section 16, for example:

- the worker or an agent reaching GitHub, chat or tracker tokens, Postgres, or a host outside the egress allowlist;
- a Builder seeing hidden scenarios;
- a merge, release or message that did not go through the gates in `nightshift`;
- a secret in git history, logs or images;
- prompt injection from issue or web text that changes what the factory does.

## Not a secret

Never put a real token, key or customer data in a report, an issue or a PR. Rotation steps for the project's own secrets are in [`docs/runbook.md`](docs/runbook.md) and [`docs/secrets.md`](docs/secrets.md).
