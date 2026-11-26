---
status: accepted
date: 2026-11-26
---

# Restore the previous factory release after a failed upgrade

## Context and Problem Statement

An owner upgrades the stable factory instance using a tagged commit.
Startup can succeed before a service exits or a release logs errors during the settling period.
[Audit #333](https://github.com/IlyaChakun/nightshift/pull/333) identified that this left the failed candidate serving.

## Considered Options

- Report the late failure and let the operator restore the previous tag.
- Restore the previous tag automatically and check the running services.

## Decision Outcome

Choose automatic recovery for failed startup, service state or release logs. Use the previous tag already recorded before activation. The stable launcher verifies the tag's cached commit and runs both doctors. The upgrade checks that the running core and worker use the Compose file from that tagged release.

### Consequences

The upgrade exits with a failure even when service is restored. An unconfirmed recovery requires the operator to run the recovery command. The candidate tag remains available for investigation.

### Confirmation

`TestUpgradeRestoresPreviousReleaseAfterLateFailure` covers service and log failures. `TestUpgradeReportsUnconfirmedRecovery` covers a failed restoration and a mismatch between the selected tag and the observed containers. The real stable exercise is pending in the [live check](../live-checks/A02a-upgrade-recovery.md).
