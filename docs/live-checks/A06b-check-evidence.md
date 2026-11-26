# Verify finding status after local checks and GitHub CI

Status: pending

Source: [A06b check evidence](../plan/phase-5/A06b-check-evidence.md) and [audit #337](https://github.com/IlyaChakun/nightshift/pull/337).

On an isolated staging tenant, trigger a local check failure, then fix it and rerun the same suite on the Builder patch. Expect `handed_off`, then `fixed`. With a credential that can read check runs, repeat with a GitHub Actions job that fails, then succeeds on the next pushed head. In a separate run, let the failing job be skipped while another job succeeds and expect its finding to remain `handed_off`. Keep the job IDs, check runs and finding rows as evidence.
