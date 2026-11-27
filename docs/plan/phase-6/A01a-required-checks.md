# Review evidence and release scheduling

State: done

Source: [audit #332](https://github.com/IlyaChakun/nightshift/pull/332). The conductor requires an authorized reviewer when verification is unavailable and waits for known release settings before merging. [ADR 0021](../../adr/0021-review-and-release-evidence.md) records the policy.

Check with `go test ./internal/control/dispatch -run 'TestRequiredReviewAwaitsEvidence|TestReleaseAwaitsKnownPolicyAndRetries|TestScenarios'`. [Live check pending](../../live-checks/A01a-required-checks.md).
