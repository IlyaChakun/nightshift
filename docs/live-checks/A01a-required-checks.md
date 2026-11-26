# Verify review holds and release scheduling

Status: pending

Source: [audit #332](https://github.com/IlyaChakun/nightshift/pull/332), [ADR 0021](../adr/0021-review-and-release-evidence.md).

On a disposable staging tenant, run a low-risk change without a Verifier. Check its task history for `verifier_unavailable` and its pull request for the pending human review status. Have an authorized reviewer approve the current head.

Check a release while its project configuration is unavailable. Confirm that the task waits in `merging`, restore the configuration and verify one release of the expected revision. Record task ID, revision, observed decision and deployed revision in the Mac live-check results.
