# Lead and Builder know that compose paths resolve from the repository root

State: proposed

Source: V4 on the Mac, 2026-11-28 (task #1008, by the Mac thread's account, not checked here): Lead and Builder wrote paths in a `compose-local` recipe as if they resolved from the compose file's folder, while the deployer resolves them from the repository root.

Next step: find where the deployer resolves the paths (`internal/control/deploy`), then say it in the stack pack or skill that Lead and Builder read for compose recipes, with a test that names the rule. A *should* item: it does not block the MVP.
