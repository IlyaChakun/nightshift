---
name: set-up-instance
description: Set up a Nightshift instance on a machine with the person, from secrets to a first approved pull request. Use when someone wants to run Nightshift, connect Slack or GitHub, or a first start fails.
---

# Set up an instance

Follow `docs/setup.md` in order; it is the only source of the steps. Your part:

1. **Check before doing.** For each section see what already exists (secret files by name only, config, volumes, running containers) and skip what is done.
2. **Hand over person steps** one at a time, with the filled-in link or command and what to send back (ids are fine, secrets never).
3. **Never read, print or commit a secret.** Do not open `.env` files or `deploy/secrets/` unless the person allows a named variable. Verify a credential by what it does (a health check, an API answer), not by its content.
4. **Ask before** creating a repository, deleting a volume or widening a permission, an egress host or a gate.
5. **Verify each section** with its check, and finish with section 8.
6. **Fix the guide** in the same session when a step was wrong or missing; run `make docs repo-check`.
