# Verifier acceptance on a real preview

The [phase 6 P5a item](../plan/phase-6/P5a-acceptance-leg.md) runs a read-only Verifier job in `acceptance` mode against the task's preview with its hidden scenarios.

On the stable instance with the `deploy` profile on, take one task of a test tenant with a `compose-local` preview and a spec that has hidden scenarios to `acceptance`. Expect: one Verifier job whose prompt names the preview URL; the job opens that URL in Chromium (screenshots in its job directory); `pass` moves the task to `human_review` or `ready`, and a failing scenario sends it to `building` with a finding that quotes no scenario step.

Known risk to check first: the preview listens on the host's 127.0.0.1, and the worker reaches the network only through its egress proxy, so the worker may not reach the preview at all.

Status: pending
