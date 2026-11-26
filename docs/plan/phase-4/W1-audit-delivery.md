# Verify and finish audit fixes with fewer repeated checks

State: open

Run repository checks before the full Go suite. Use one PR helper to check access, wait on the verified head and, when authorized, merge and verify the landed commit. Keep live evidence in individual check files. The `audit` skill applies authorized findings and records before and after measurements.

Check with `make repo-check`, `make verify`, and a live PR completed with `scripts/pr-gate.sh`.
