# Restore the previous stable release after a late failure

State: done

Source: [audit #333](https://github.com/IlyaChakun/nightshift/pull/333).

The upgrade restores the previous tag after startup, service or log checks fail and verifies the running core and worker. A failed restoration prints the owner command. See [ADR 0022](../../adr/0022-stable-upgrade-recovery.md).

Check: `go test ./deploy -run 'TestUpgradeRestoresPreviousReleaseAfterLateFailure|TestUpgradeReportsUnconfirmedRecovery|TestAReleaseThatDoesNotStartIsRolledBack'`. The real stable exercise is pending in the [live check](../../live-checks/A02a-upgrade-recovery.md).
