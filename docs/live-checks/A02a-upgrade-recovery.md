# Stable factory upgrade restores the previous release after a late failure

- What: [audit #333](https://github.com/IlyaChakun/nightshift/pull/333), [A02a](../plan/phase-6/A02a-upgrade-recovery.md).
- Run: during an owner-approved stable upgrade exercise, record the previous tag. After candidate startup and before the settle check, arrange a detectable service failure. Capture the upgrade exit status and operator output.
- Expect: the upgrade fails, the running core and worker belong to the previous tag's Compose file, and both doctors pass. Compare the running containers with the recorded previous release and retain the evidence.
- Status: pending
