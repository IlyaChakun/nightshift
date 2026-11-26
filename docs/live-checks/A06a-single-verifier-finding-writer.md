# Check a Verifier finding after job completion and dispatch

Status: pending

Source: [A06a](../plan/phase-5/A06a-single-verifier-finding-writer.md) and [audit #337](https://github.com/IlyaChakun/nightshift/pull/337).

On an isolated staging tenant, run a Verifier review that returns one blocking finding with a rule, path, line range and fingerprint. Compare the finding rows for that job after completion and after dispatch. Expect one row with the same fields and one finding in the next Builder job.
